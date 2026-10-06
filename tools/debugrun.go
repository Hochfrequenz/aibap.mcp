package tools

import (
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
	defer r.mu.Unlock()
	if r.st.Status != runAttaching {
		return // cleanup started meanwhile
	}
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
