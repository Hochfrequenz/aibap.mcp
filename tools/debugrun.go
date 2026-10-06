package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// Run statuses (DebugRunState.Status).
const (
	runListening = "listening"
	runAttaching = "attaching"
	runAttached  = "attached"
	runEnded     = "ended"
	runTimeout   = "timeout"
	runStopping  = "stopping"
	runStopped   = "stopped"
)

// End reasons (DebugRunState.EndReason, status ended).
const (
	endDetached     = "detached"
	endCompleted    = "completed"
	endTerminated   = "terminated"
	endNoHit        = "no_hit"
	endAttachFailed = "attach_failed"
	endIdleDetached = "idle_detached"
)

// Trigger kinds (debug_run trigger.kind) and trigger states.
const (
	triggerUnitTests = "unit_tests"
	triggerManual    = "manual"
	triggerGUI       = "gui"

	triggerPending = "pending"
	triggerRunning = "running"
	triggerDone    = "done"
	triggerFailed  = "failed"
)

// debugTimings are the waits of a run (spec "Run state and concurrency").
// newDebugSessions copies the defaults, so a test can shorten them before it
// registers a server without racing a running goroutine.
type debugTimings struct {
	triggerDelay     time.Duration // listener start → trigger start
	initialWait      time.Duration // debug_run waits this long for a server-triggered run
	cleanupBudget    time.Duration // total budget of one cleanup
	listenerExitWait time.Duration // cleanup waits this long for the listener goroutine
	idleLimit        time.Duration // an idle attached debuggee is detached after this
	attachTimeout    time.Duration // deadline of the background attach (and of late detaches)
	triggerSlack     time.Duration // trigger deadline = listener budget + this
}

var (
	debugTimingsMu      sync.Mutex
	defaultDebugTimings = debugTimings{
		triggerDelay:     4 * time.Second,
		initialWait:      15 * time.Second,
		cleanupBudget:    20 * time.Second,
		listenerExitWait: 10 * time.Second,
		idleLimit:        10 * time.Minute,
		attachTimeout:    30 * time.Second,
		triggerSlack:     30 * time.Minute,
	}
)

func currentDebugTimings() debugTimings {
	debugTimingsMu.Lock()
	defer debugTimingsMu.Unlock()
	return defaultDebugTimings
}

// debugRun is one debugging run (#558): breakpoints, listening windows, the
// background attach and an optional trigger. Its state lives behind mu and
// changes only through transition, which wakes every waiter.
//
// Lock order: the run-start lock (*startMu), debugSessions.mu, then mu. No
// HTTP call is made while mu is held.
type debugRun struct {
	// Set once before the run is published, read-only afterwards.
	key         debugSessionKey
	user        string
	kind        string
	sess        *adt.DebugSession // the debug session: listener, attach, steps
	cleanupSess *adt.DebugSession // fresh session, same user/terminal/IDE ID: StopListener and external deletes
	source      func(ctx context.Context, uri string) (string, error)
	timings     debugTimings
	nextVersion func() int64
	startMu     *sync.Mutex
	budgetEnd   time.Time
	runCtx      context.Context
	runCancel   context.CancelFunc

	mu           sync.Mutex
	st           DebugRunState
	changed      chan struct{} // closed and replaced on every transition
	fatal        error         // listener failure; reported by debug_run/debug_wait
	listenerDone chan struct{} // closed when the current listener goroutine exits
	windowCancel context.CancelFunc
	inFlight     int         // in-attempt calls in flight; the idle timer pauses meanwhile
	idleGen      int         // invalidates idle timers that fired late
	idleTimer    *time.Timer // detaches an idle attached debuggee
	detaching    bool        // the idle detach is in flight
}

// runParams are what debug_run knows when it creates a run.
type runParams struct {
	key         debugSessionKey
	kind        string
	sess        *adt.DebugSession
	cleanupSess *adt.DebugSession
	breakpoints []DebugRunBreakpoint
	budget      time.Duration
	trigger     *DebugTriggerState
	hint        string
}

func newDebugRun(p runParams, timings debugTimings, nextVersion func() int64, startMu *sync.Mutex) *debugRun {
	ctx, cancel := context.WithCancel(context.Background())
	r := &debugRun{
		key: p.key, user: p.key.user, kind: p.kind,
		sess: p.sess, cleanupSess: p.cleanupSess,
		timings: timings, nextVersion: nextVersion, startMu: startMu,
		budgetEnd: time.Now().Add(p.budget),
		runCtx:    ctx, runCancel: cancel,
		changed: make(chan struct{}),
	}
	r.st = copyState(DebugRunState{
		Version:     nextVersion(),
		Status:      runListening,
		Breakpoints: p.breakpoints,
		Trigger:     p.trigger,
		Hint:        p.hint,
	})
	r.st.ListeningUntil = r.budgetEnd.UTC().Format(time.RFC3339)
	return r
}

// transitionLocked applies fn, increments the version and wakes every waiter.
// Caller holds r.mu.
func (r *debugRun) transitionLocked(fn func(st *DebugRunState)) {
	fn(&r.st)
	r.st.Version = r.nextVersion()
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *debugRun) transition(fn func(st *DebugRunState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitionLocked(fn)
}

// snapshot returns a deep copy of the state and the listener failure, if any.
func (r *debugRun) snapshot() (DebugRunState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyState(r.st), r.fatal
}

func (r *debugRun) status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st.Status
}

// active reports whether the run has not been cleaned up.
func (r *debugRun) active() bool {
	s := r.status()
	return s != runStopping && s != runStopped
}

// copyState deep-copies s and guarantees a non-nil Breakpoints slice.
func copyState(s DebugRunState) DebugRunState {
	out := s
	out.Breakpoints = append([]DebugRunBreakpoint{}, s.Breakpoints...)
	if s.Position != nil {
		p := *s.Position
		out.Position = &p
	}
	if s.Trigger != nil {
		t := *s.Trigger
		if s.Trigger.UnitTests != nil {
			t.UnitTests = copyTestResult(s.Trigger.UnitTests)
		}
		out.Trigger = &t
	}
	if s.Instructions != nil {
		out.Instructions = &DebugInstructions{
			Steps: append([]string{}, s.Instructions.Steps...),
			Notes: append([]string{}, s.Instructions.Notes...),
			User:  s.Instructions.User,
		}
	}
	return out
}

// copyTestResult deep-copies a unit-test result, test cases and messages included.
func copyTestResult(r *adt.TestResult) *adt.TestResult {
	out := *r
	out.TestCases = make([]adt.TestCase, len(r.TestCases))
	for i, tc := range r.TestCases {
		tc.Messages = append([]string(nil), tc.Messages...)
		out.TestCases[i] = tc
	}
	return &out
}

// waitUntil blocks until pred holds, timeout passes or ctx ends, and returns
// the state then. Nothing is consumed, so concurrent waiters each see every
// change; a cancelled wait does not affect the run.
func (r *debugRun) waitUntil(ctx context.Context, timeout time.Duration, pred func(DebugRunState) bool) (DebugRunState, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		r.mu.Lock()
		st, fatal, changed := copyState(r.st), r.fatal, r.changed
		r.mu.Unlock()
		if pred(st) {
			return st, fatal
		}
		select {
		case <-changed:
		case <-timer.C:
			return r.snapshot()
		case <-ctx.Done():
			return r.snapshot()
		}
	}
}

// stopIdleLocked cancels a pending idle detach. Caller holds r.mu.
func (r *debugRun) stopIdleLocked() {
	r.idleGen++
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
}

// timeoutHint explains a listening window that ended without a hit.
const timeoutHint = "No run hit a breakpoint while the listener was waiting. Usual causes: the run was made as another user, " +
	"the breakpoint is in a system program, the run started before or after the listening window, " +
	"or (gui) the SAP GUI session was not enabled for external debugging."

// excerptRadius is how many source lines position.source_excerpt shows above
// and below the current line.
const excerptRadius = 3

// adtExceptionSubtype is the ADT exception property that carries a subtype.
const adtExceptionSubtype = "com.sap.adt.communicationFramework.subType"

// startWindowLocked starts a listening window: one long poll with the run's
// remaining budget, at most maxRunTimeoutSeconds. Caller holds r.mu.
func (r *debugRun) startWindowLocked() {
	secs := int(math.Ceil(time.Until(r.budgetEnd).Seconds()))
	secs = max(1, min(secs, maxRunTimeoutSeconds))
	ctx, cancel := context.WithCancel(r.runCtx)
	done := make(chan struct{})
	r.windowCancel, r.listenerDone = cancel, done
	go r.listen(ctx, cancel, secs, done)
}

// listen is the listener goroutine of one window. On a hit, unless cleanup has
// started, it sets attaching and attaches.
func (r *debugRun) listen(ctx context.Context, cancel context.CancelFunc, secs int, done chan struct{}) {
	defer close(done)
	defer cancel()
	res, err := r.sess.StartListener(ctx, secs)
	if ctx.Err() != nil {
		return // cleanup, or the trigger ended the run without a hit
	}
	r.mu.Lock()
	if r.st.Status != runListening {
		r.mu.Unlock()
		return // cleanup started meanwhile; its StopListener released the poll
	}
	if err != nil {
		r.fatal = fmt.Errorf("the debug listener failed: %w", err)
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runTimeout
			st.Hint = "The listener failed: " + err.Error()
		})
		r.mu.Unlock()
		return
	}
	if res.Status != "attached" || res.DebuggeeID == "" {
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runTimeout
			st.Hint = timeoutHint
		})
		r.mu.Unlock()
		return
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runAttaching
		st.DebuggeeID = res.DebuggeeID
	})
	r.mu.Unlock()
	r.attach(res.DebuggeeID)
}

// attach attaches to a caught debuggee with one Attach call (adtler retries
// AdiFailed until adtler#196 lands) and its own deadline, not derived from the
// run context, so cleanup cannot abort an attach that SAP may already be
// completing.
func (r *debugRun) attach(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	err := r.sess.Attach(ctx, id)
	var pos *DebugPosition
	var perr error
	if err == nil && r.status() == runAttaching {
		pos, perr = r.readPosition(ctx)
	}
	r.mu.Lock()
	if r.st.Status != runAttaching {
		r.mu.Unlock()
		// Cleanup started meanwhile and skipped the detach (the run was not
		// attached at its step 1), so a completed attach is detached here, in
		// the listener goroutine, whose exit cleanup step 2b waits for.
		if err == nil {
			r.detachLate()
		}
		return
	}
	defer r.mu.Unlock()
	if err != nil {
		r.transitionLocked(func(st *DebugRunState) {
			st.Status = runEnded
			st.EndReason = endAttachFailed
			st.Hint = attachFailedHint(err)
		})
		return
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runAttached
		st.Position = pos
		st.Hint = ""
		if perr != nil {
			st.Hint = "Attached, but the position could not be read: " + perr.Error()
		}
	})
	r.resetIdleLocked()
}

// detachLate detaches an attach that completed after cleanup had started.
// Best effort, with its own deadline. It runs synchronously in the listener
// goroutine without r.mu: a new run on the same debug session starts only
// after cleanup has waited for that goroutine, so the detach cannot hit the
// new run's breakpoints or listener.
func (r *debugRun) detachLate() {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	_, _ = r.sess.Step(ctx, "detachDebugger")
}

func attachFailedHint(err error) string {
	h := "Attaching to the caught run failed: " + err.Error() + ". Start a new debug_run."
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) && adtErr.Properties[adtExceptionSubtype] == "invalidServer" {
		h += " SAP answered invalidServer: the run was caught on another application server, which this server cannot attach to yet (#513)."
	}
	return h
}

// readPosition reads where the debuggee stands: the active stack frame
// (adtler#203) and a few source lines around it.
func (r *debugRun) readPosition(ctx context.Context) (*DebugPosition, error) {
	frames, err := r.sess.GetStackFrames(ctx)
	if err != nil {
		return nil, err
	}
	f, ok := adt.ActiveFrame(frames)
	if !ok {
		return nil, errors.New("the debugger returned an empty stack")
	}
	pos := &DebugPosition{Program: f.Program, Include: f.Include, Line: f.Line, SourceURI: f.SourceURI, SourceLine: f.SourceLine}
	if pos.SourceURI != "" && pos.SourceLine > 0 && r.source != nil {
		if text, err := r.source(ctx, pos.SourceURI); err == nil {
			pos.SourceExcerpt = sourceExcerpt(text, pos.SourceLine, excerptRadius)
		}
	}
	return pos, nil
}

// sourceExcerpt returns lines line-radius … line+radius of text, numbered,
// with the current line marked ">".
func sourceExcerpt(text string, line, radius int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	from, to := max(1, line-radius), min(len(lines), line+radius)
	var b strings.Builder
	for n := from; n <= to; n++ {
		marker := "  "
		if n == line {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%d: %s\n", marker, n, lines[n-1])
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// noHitHint explains a unit-test run that finished without a hit.
const noHitHint = "The trigger finished without a hit: the breakpoint line was not executed, " +
	"the run was made as another user, or the breakpoint is in a system program."

// runTrigger is the trigger goroutine. It starts about triggerDelay after the
// listener, because the activation reaches the server asynchronously, and
// runs fn with a context the run's cleanup does not cancel: the listener
// budget plus triggerSlack. endsRun says that fn's return proves the run
// finished (unit tests), so a return without a hit ends the run with no_hit;
// a DebugTriggerer may return as soon as it has started the run.
func (r *debugRun) runTrigger(fn func(context.Context) (*adt.TestResult, error), fallback *DebugInstructions, endsRun bool) {
	delay := time.NewTimer(r.timings.triggerDelay)
	select {
	case <-delay.C:
	case <-r.runCtx.Done():
		delay.Stop()
		return // cleaned up before the trigger started
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Until(r.budgetEnd)+r.timings.triggerSlack)
	defer cancel()
	r.transition(func(st *DebugRunState) { st.Trigger.State = triggerRunning })

	res, err := fn(ctx)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("trigger request timed out: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	noHit := endsRun && err == nil && r.st.Status == runListening
	r.transitionLocked(func(st *DebugRunState) {
		if err != nil {
			st.Trigger.State = triggerFailed
			st.Trigger.Error = err.Error()
			if st.Status == runListening {
				st.Instructions = fallback
			}
		} else {
			st.Trigger.State = triggerDone
			st.Trigger.UnitTests = res
		}
		if noHit {
			st.Status = runEnded
			st.EndReason = endNoHit
			st.Hint = strings.TrimSpace(st.Hint + " " + noHitHint)
		}
	})
	if noHit {
		r.windowCancel()
		go r.stopListenerQuietly()
	}
}

// stopListenerQuietly deregisters the listener after a no_hit end; best
// effort, from the cleanup session. The breakpoints stay until debug_stop.
func (r *debugRun) stopListenerQuietly() {
	ctx, cancel := context.WithTimeout(context.Background(), r.timings.listenerExitWait)
	defer cancel()
	_ = r.cleanupSess.StopListener(ctx)
}

// errNoDebuggee is the answer of an in-attempt tool outside status attached.
var errNoDebuggee = errors.New("no debuggee attached; call debug_wait")

// beginCall admits an in-attempt call: only while attached and not being
// detached for idleness. The idle timer pauses until endCall.
func (r *debugRun) beginCall() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runAttached || r.detaching {
		return errNoDebuggee
	}
	r.inFlight++
	r.stopIdleLocked()
	return nil
}

// endCall ends an in-attempt call; the last one restarts the idle timer.
func (r *debugRun) endCall() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inFlight--
	if r.inFlight == 0 && r.st.Status == runAttached {
		r.resetIdleLocked()
	}
}

// resetIdleLocked (re)starts the idle timer. Caller holds r.mu.
func (r *debugRun) resetIdleLocked() {
	r.stopIdleLocked()
	gen := r.idleGen
	r.idleTimer = time.AfterFunc(r.timings.idleLimit, func() { r.onIdle(gen) })
}

// onIdle detaches a debuggee left attached for idleLimit without a debugger
// call, so it does not hold a SAP work process indefinitely. It holds the
// run-start lock across its HTTP call, so it and cleanup never detach twice.
func (r *debugRun) onIdle(gen int) {
	r.startMu.Lock()
	defer r.startMu.Unlock()
	r.mu.Lock()
	if gen != r.idleGen || r.st.Status != runAttached || r.inFlight > 0 {
		r.mu.Unlock()
		return
	}
	r.detaching = true
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), r.timings.attachTimeout)
	defer cancel()
	_, err := r.sess.Step(ctx, "detachDebugger")

	r.mu.Lock()
	defer r.mu.Unlock()
	r.detaching = false
	if r.st.Status != runAttached {
		return
	}
	hint := fmt.Sprintf("Detached after %s without a debugger call, so the halted program does not hold a SAP work process.", r.timings.idleLimit)
	if err != nil && !isNotAttachedErr(err) {
		hint += " The detach failed: " + err.Error()
	}
	r.transitionLocked(func(st *DebugRunState) {
		st.Status = runEnded
		st.EndReason = endIdleDetached
		st.Position = nil
		st.Hint = hint
	})
}

// stepOutcome is what a debug_step did: SAP's raw answer and the run state after it.
type stepOutcome struct {
	raw   []byte
	state DebugRunState
}

// step runs one step action and moves the run state (spec "Transitions caused
// by the in-attempt tools").
func (r *debugRun) step(ctx context.Context, action string) (stepOutcome, error) {
	if err := r.beginCall(); err != nil {
		return stepOutcome{}, err
	}
	defer r.endCall()
	data, err := r.sess.Step(ctx, action)
	switch _, ended := stepResultForError(err); {
	case err == nil && action == "detachDebugger":
		r.end(endDetached, "")
	case err == nil && action == "terminateDebuggee":
		r.end(endTerminated, "")
	case err == nil:
		pos, perr := r.readPosition(ctx)
		r.transitionIfAttached(func(st *DebugRunState) {
			st.Position = pos
			st.Hint = ""
			if perr != nil {
				st.Hint = "The position could not be read after the step: " + perr.Error()
			}
		})
	case ended:
		r.end(endCompleted, "")
	case action == "stepContinue" && isInvalidDataErr(err):
		if !r.debuggeeGone(ctx) {
			return stepOutcome{}, fmt.Errorf("debug_step: %w (a debuggee is still listed, so the attachment may be lost, #513; "+
				"end it with debug_step detachDebugger or debug_stop)", err)
		}
		r.end(endCompleted, "stepContinue answered 400 ExceptionInvalidData and no debuggee remains: the run completed "+
			"(SAP_BASIS 816 reports the end of a run this way, #513).")
	default:
		return stepOutcome{}, fmt.Errorf("debug_step: %w — the debuggee is still attached; if it does not respond, "+
			"end the session with debug_step detachDebugger or debug_stop", err)
	}
	st, _ := r.snapshot()
	return stepOutcome{raw: data, state: st}, nil
}

// end moves an attached run to ended with reason.
func (r *debugRun) end(reason, hint string) {
	r.transitionIfAttached(func(st *DebugRunState) {
		st.Status = runEnded
		st.EndReason = reason
		st.Position = nil
		st.Hint = hint
	})
}

// transitionIfAttached applies fn only while the run is attached; cleanup or
// the idle detach may have moved it on meanwhile.
func (r *debugRun) transitionIfAttached(fn func(st *DebugRunState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status == runAttached {
		r.transitionLocked(fn)
	}
}

// debuggeeGone reports whether SAP lists no debuggee session any more.
func (r *debugRun) debuggeeGone(ctx context.Context) bool {
	data, err := r.sess.GetDebuggeeSessions(ctx)
	return err == nil && len(bytes.TrimSpace(data)) == 0
}

// isInvalidDataErr reports SAP's 400 ExceptionInvalidData.
func isInvalidDataErr(err error) bool {
	var adtErr *adt.ADTError
	return errors.As(err, &adtErr) && adtErr.StatusCode == 400 && adtErr.Type == "ExceptionInvalidData"
}
