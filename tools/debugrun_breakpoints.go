package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

var errNoRun = errors.New("no active debug run: start a run with debug_run")

var errExternalRemoveWhileAttached = errors.New("debug_remove_breakpoint: an external breakpoint cannot be removed while the debuggee is attached " +
	"(on SAP_BASIS 750 the request detaches the debugger); remove it after detachDebugger, or debug_stop removes it")

// runFor returns the active run for a breakpoint tool; user (when given) and
// the active system must match it.
func (m *debugSessions) runFor(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil || !m.run.active() {
		return nil, errNoRun
	}
	if err := m.checkRunKeyLocked(user); err != nil {
		return nil, err
	}
	return m.run, nil
}

// addBreakpoint implements debug_set_breakpoint (#558). While attached the
// breakpoint goes to the attached debugger (scope debugger, stateful): an
// external request then detaches the debugger on SAP_BASIS 750. Otherwise the
// run's external breakpoints are set again together with the new one.
func (m *debugSessions) addBreakpoint(ctx context.Context, user string, bp adt.LineBreakpoint) (DebugRunState, error) {
	run, err := m.runFor(user)
	if err != nil {
		return DebugRunState{}, err
	}
	switch run.status() {
	case runAttaching:
		return DebugRunState{}, errors.New("debug_set_breakpoint: attach in progress; call debug_wait")
	case runAttached:
		return run.addDebuggerBreakpoint(ctx, bp)
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if !run.active() {
		return DebugRunState{}, errNoRun
	}
	return run.resetExternalBreakpoints(ctx, bp)
}

// addDebuggerBreakpoint adds bp to the attached debugger. It is an in-attempt
// call: admitted only while attached, with the idle timer paused.
func (r *debugRun) addDebuggerBreakpoint(ctx context.Context, bp adt.LineBreakpoint) (DebugRunState, error) {
	if err := r.beginCall(); err != nil {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w", err)
	}
	defer r.endCall()
	res, err := r.sess.SetBreakpoints(ctx, adt.BreakpointScopeDebugger, []adt.LineBreakpoint{bp})
	if errors.Is(err, adt.ErrNoSessionAttached) {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: no debugger is attached any more; call debug_wait: %w", err)
	}
	if err != nil {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w", err)
	}
	set, err := checkBreakpointResults([]adt.LineBreakpoint{bp}, res)
	if err != nil {
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w", err)
	}
	set[0].Scope = string(adt.BreakpointScopeDebugger)
	r.transition(func(st *DebugRunState) {
		st.Breakpoints = append(st.Breakpoints, set[0])
	})
	st, _ := r.snapshot()
	return st, nil
}

// resetExternalBreakpoints deletes every external breakpoint of the run and
// sets the full list, including bp, in one request, so the stored IDs stay
// right on both releases (816 replaces, 750 adds). If that fails after the
// delete, the previous list is set again; if that fails too, the error names
// the breakpoints that are no longer set. A failed delete stops the reset; the
// ones deleted before it are no longer set. Caller holds the run-start lock.
//
// Every request here goes through the cleanup session, never r.sess: while the
// run is listening, r.sess is busy with the listener's long poll and SAP
// serialises requests per stateful session, so a DELETE or POST on it hangs
// until the client timeout (SAP_BASIS 816). Breakpoints are bound to the user
// and terminal/IDE ID, not to the session, so the listener still catches. The
// cleanup session is only used under the run-start lock (this, removeBreakpoint
// and cleanupRun all hold it), so its requests never overlap.
func (r *debugRun) resetExternalBreakpoints(ctx context.Context, bp adt.LineBreakpoint) (DebugRunState, error) {
	r.mu.Lock()
	if s := r.st.Status; s == runAttaching || s == runAttached {
		r.mu.Unlock()
		return DebugRunState{}, errors.New("debug_set_breakpoint: the run was caught meanwhile; call debug_wait, then set the breakpoint again")
	}
	var ext []DebugRunBreakpoint
	for _, b := range r.st.Breakpoints {
		if b.Scope == string(adt.BreakpointScopeExternal) {
			ext = append(ext, b)
		}
	}
	r.mu.Unlock()

	for i, b := range ext {
		if b.ID == "" {
			continue
		}
		if err := r.cleanupSess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, b.ID); err != nil {
			var gone []DebugRunBreakpoint
			for _, d := range ext[:i] {
				if d.ID != "" {
					gone = append(gone, d)
				}
			}
			if len(gone) == 0 {
				return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: removing breakpoint %s before setting the new list failed: %w", b.ID, err)
			}
			for _, d := range gone {
				r.dropBreakpoint(d.ID)
			}
			return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: removing breakpoint %s before setting the new list failed: %w; these were removed before and are no longer set: %s",
				b.ID, err, describeBreakpoints(gone))
		}
	}
	prev := lineBreakpoints(ext)
	set, err := setExternal(ctx, r.cleanupSess, append(append([]adt.LineBreakpoint{}, prev...), bp))
	if err != nil {
		// The restore must outlive a cancelled request, or the run would lose
		// its breakpoints silently.
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), currentDebugTimings().cleanupBudget)
		restored, rerr := setExternal(rctx, r.cleanupSess, prev)
		rcancel()
		if rerr != nil {
			r.replaceExternal(nil)
			return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w; setting the previous breakpoints again failed too (%v), so these are no longer set: %s",
				err, rerr, describeBreakpoints(ext))
		}
		r.replaceExternal(restored)
		return DebugRunState{}, fmt.Errorf("debug_set_breakpoint: %w; the previous breakpoints are set again", err)
	}
	r.replaceExternal(set)
	st, _ := r.snapshot()
	return st, nil
}

// setExternal sets bps in one external request. If any is rejected, the set
// ones are removed again and the error names the rejected ones. launchRun and
// resetExternalBreakpoints share it.
func setExternal(ctx context.Context, sess *adt.DebugSession, bps []adt.LineBreakpoint) ([]DebugRunBreakpoint, error) {
	if len(bps) == 0 {
		return nil, nil
	}
	res, err := sess.SetBreakpoints(ctx, adt.BreakpointScopeExternal, bps)
	if err != nil {
		return nil, fmt.Errorf("setting the breakpoints failed: %w", err)
	}
	set, err := checkBreakpointResults(bps, res)
	if err != nil {
		removeSetBreakpoints(ctx, sess, set)
		return nil, fmt.Errorf("%w; the breakpoints that were set are removed again", err)
	}
	return set, nil
}

// lineBreakpoints turns stored breakpoints back into a request list.
func lineBreakpoints(bps []DebugRunBreakpoint) []adt.LineBreakpoint {
	out := make([]adt.LineBreakpoint, 0, len(bps))
	for _, b := range bps {
		typ, name := deriveBreakpointObject(b.ObjectURI)
		out = append(out, adt.LineBreakpoint{ObjectURI: b.ObjectURI, Line: b.Line, ObjectType: typ, ObjectName: name})
	}
	return out
}

func describeBreakpoints(bps []DebugRunBreakpoint) string {
	parts := make([]string, 0, len(bps))
	for _, b := range bps {
		parts = append(parts, fmt.Sprintf("%s line %d", b.ObjectURI, b.Line))
	}
	return strings.Join(parts, "; ")
}

// replaceExternal stores set as the run's external breakpoints, followed by
// its debugger-scope ones as they are now.
func (r *debugRun) replaceExternal(set []DebugRunBreakpoint) {
	r.transition(func(st *DebugRunState) {
		out := append([]DebugRunBreakpoint{}, set...)
		for _, b := range st.Breakpoints {
			if b.Scope != string(adt.BreakpointScopeExternal) {
				out = append(out, b)
			}
		}
		st.Breakpoints = out
	})
}

// dropBreakpoint forgets the stored breakpoint with id.
func (r *debugRun) dropBreakpoint(id string) {
	r.transition(func(st *DebugRunState) {
		kept := st.Breakpoints[:0:0]
		for _, b := range st.Breakpoints {
			if b.ID != id {
				kept = append(kept, b)
			}
		}
		st.Breakpoints = kept
	})
}

// breakpoint returns the stored breakpoint with id.
func (r *debugRun) breakpoint(id string) (DebugRunBreakpoint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.st.Breakpoints {
		if b.ID == id {
			return b, true
		}
	}
	return DebugRunBreakpoint{}, false
}

// removeBreakpoint implements debug_remove_breakpoint: DELETE with the scope
// stored for the breakpoint. A debugger-scope breakpoint needs the attachment.
// An external one is deleted on the cleanup session (r.sess may be busy with
// the listener's long poll) and is refused while attaching or attached: on
// SAP_BASIS 750 an external request then detaches the debugger.
func (m *debugSessions) removeBreakpoint(ctx context.Context, user, id string) (DebugRunBreakpoint, error) {
	run, err := m.runFor(user)
	if err != nil {
		return DebugRunBreakpoint{}, err
	}
	bp, ok := run.breakpoint(id)
	if !ok {
		return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: breakpoint %q is not part of the current run", id)
	}
	if bp.Scope == string(adt.BreakpointScopeDebugger) {
		if err := run.beginCall(); err != nil {
			return DebugRunBreakpoint{}, err
		}
		defer run.endCall()
		if err := run.sess.RemoveBreakpoint(ctx, adt.BreakpointScopeDebugger, id); err != nil && !isNotAttachedErr(err) {
			return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: %w", err)
		}
	} else {
		m.startMu.Lock()
		defer m.startMu.Unlock()
		if !run.active() { // a debug_stop or a new debug_run won the lock meanwhile
			return DebugRunBreakpoint{}, errNoRun
		}
		if s := run.status(); s == runAttaching || s == runAttached {
			return DebugRunBreakpoint{}, errExternalRemoveWhileAttached
		}
		if err := run.cleanupSess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, id); err != nil {
			return DebugRunBreakpoint{}, fmt.Errorf("debug_remove_breakpoint: %w", err)
		}
	}
	run.dropBreakpoint(id)
	return bp, nil
}
