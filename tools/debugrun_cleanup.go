package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// bpFailure is a breakpoint cleanup could not remove.
type bpFailure struct {
	bp  DebugRunBreakpoint
	err error
}

// cleanupReport is what a cleanup removed and what it could not.
type cleanupReport struct {
	removed     []DebugRunBreakpoint
	notRemoved  []bpFailure
	listenerErr error
	detachErr   error
}

// err joins every failure of the cleanup, nil when it was complete.
func (rep cleanupReport) err() error {
	var errs []error
	if rep.listenerErr != nil {
		errs = append(errs, fmt.Errorf("stopping the listener: %w", rep.listenerErr))
	}
	for _, f := range rep.notRemoved {
		errs = append(errs, fmt.Errorf("removing breakpoint %q (%s line %d): %w", f.bp.ID, f.bp.ObjectURI, f.bp.Line, f.err))
	}
	if rep.detachErr != nil {
		errs = append(errs, fmt.Errorf("detaching the debugger: %w", rep.detachErr))
	}
	return errors.Join(errs...)
}

// cleanupRun stops r, best effort, inside one budget (spec "Cleanup"). It runs
// on debug_stop, a new debug_run, a session replacement and shutdown. Caller
// holds startMu and neither mu nor r.mu.
func (m *debugSessions) cleanupRun(r *debugRun) cleanupReport {
	// Step 1: stopping. From now on the listener goroutine starts no attach.
	r.mu.Lock()
	if r.st.Status == runStopping || r.st.Status == runStopped {
		r.mu.Unlock()
		return cleanupReport{}
	}
	attached := r.st.Status == runAttached
	bps := append([]DebugRunBreakpoint(nil), r.st.Breakpoints...)
	done := r.listenerDone
	r.stopIdleLocked()
	r.transitionLocked(func(st *DebugRunState) { st.Status = runStopping })
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.timings.cleanupBudget)
	defer cancel()

	// Step 2, in parallel: a) listener and external breakpoints from the
	// cleanup session; b) end the listener goroutine.
	var rep cleanupReport
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.stopExternal(ctx, bps, &rep)
	}()
	go func() {
		defer wg.Done()
		r.awaitListenerExit(ctx, done)
	}()
	wg.Wait()

	// Step 3: an attached debuggee would stay suspended in SAP, holding a work
	// process, and the trigger request would stay open. An attach completing
	// after step 1 is detached by the listener goroutine itself (detachLate).
	if attached {
		r.detachAttached(ctx, bps, &rep)
	}

	// Step 4: a trigger keeps running; its result goes into the stopped run.
	// Step 5: stopped. A listener failure belonged to the run that is now
	// cleaned up; debug_wait reports the stopped run, not the old failure.
	r.mu.Lock()
	r.fatal = nil
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runStopped
		st.Position = nil
	})
	r.mu.Unlock()
	return rep
}

// stopExternal stops the listener and deletes the run's external breakpoints
// from the cleanup session. An empty syncMode="full" request is not used: it
// removes nothing on SAP_BASIS 750.
func (r *debugRun) stopExternal(ctx context.Context, bps []DebugRunBreakpoint, rep *cleanupReport) {
	if err := r.cleanupSess.StopListener(ctx); err != nil {
		rep.listenerErr = err
	}
	for _, bp := range bps {
		if bp.Scope != string(adt.BreakpointScopeExternal) {
			continue
		}
		if bp.ID == "" {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, errors.New("SAP returned no ID for this breakpoint")})
			continue
		}
		if err := r.cleanupSess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, bp.ID); err != nil {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, err})
			continue
		}
		rep.removed = append(rep.removed, bp)
	}
}

// awaitListenerExit cancels the run context and waits for the listener
// goroutine, including an attach in flight, for at most listenerExitWait.
func (r *debugRun) awaitListenerExit(ctx context.Context, done chan struct{}) {
	r.runCancel()
	if done == nil {
		return
	}
	t := time.NewTimer(r.timings.listenerExitWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	case <-ctx.Done():
	}
}

// detachAttached deletes the debugger-scope breakpoints (they need the
// attachment) and detaches, both in the debug session. A "not attached"
// answer counts as success: the external deletes of step 2 may already have
// ended the attachment.
func (r *debugRun) detachAttached(ctx context.Context, bps []DebugRunBreakpoint, rep *cleanupReport) {
	for _, bp := range bps {
		if bp.Scope != string(adt.BreakpointScopeDebugger) {
			continue
		}
		if err := r.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeDebugger, bp.ID); err != nil && !isNotAttachedErr(err) {
			rep.notRemoved = append(rep.notRemoved, bpFailure{bp, err})
			continue
		}
		rep.removed = append(rep.removed, bp)
	}
	if err := r.detachWithin(ctx); err != nil && !isNotAttachedErr(err) {
		rep.detachErr = err
	}
}

// detachWithin sends detachDebugger in the debug session and returns when ctx
// ends even if the call has not. adt.DebugSession.Step follows a timed-out
// request with a check of its own (up to 10 s, not bound to ctx), which would
// otherwise overrun the cleanup budget; the abandoned call holds no lock and
// ends on its own.
func (r *debugRun) detachWithin(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() {
		_, err := r.sess.Step(ctx, "detachDebugger")
		errc <- err
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isNotAttachedErr reports whether err says that no debugger is attached (any
// more), or that the debuggee already ended.
func isNotAttachedErr(err error) bool {
	if errors.Is(err, adt.ErrNoSessionAttached) {
		return true
	}
	var ended *adt.DebuggeeEndedError
	if errors.As(err, &ended) {
		return true
	}
	var adtErr *adt.ADTError
	return errors.As(err, &adtErr) && adtErr.Properties[adtExceptionSubtype] == "noSessionAttached"
}
