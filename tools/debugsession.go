package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// processIDEID identifies this server process to SAP's debugger. Breakpoints
// and listeners are keyed by the logged-on user, requestUser and ideId, so two
// aibap.mcp processes of the same user no longer overwrite (SAP_BASIS 816) or
// mix (SAP_BASIS 750) each other's breakpoints (#558). The price: a new
// process cannot delete breakpoints a crashed one left behind.
var processIDEID = newIDEID(rand.Reader)

// newIDEID returns 32 upper-case hex characters read from r. If r fails, which
// crypto/rand does not do on supported platforms, it falls back to the start
// time and process ID so the server still starts with a per-process value.
func newIDEID(r io.Reader) string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err == nil {
		return strings.ToUpper(hex.EncodeToString(b))
	}
	return fmt.Sprintf("%016X%016X", uint64(time.Now().UnixNano()), uint64(os.Getpid()))
}

// debugSessionKey identifies what a DebugSession is bound to. adt.NewDebugSession
// fixes both at creation: the SAP user (upper-cased) and the system that was
// active then, because it copies that system's client (freshSession).
type debugSessionKey struct {
	user   string
	system string
}

// debugSessions owns the one DebugSession the debug tools share (#562) and
// the debugging run on it (#558).
//
// Locks, always taken in this order and never the reverse: startMu (the
// run-start lock: serialises debug_run, debug_stop, session replacement, the
// idle detach and the shutdown hook, and is held across their HTTP calls),
// mu, then a run's mu. No HTTP call is made while mu or a run's mu is held.
type debugSessions struct {
	startMu sync.Mutex
	mu      sync.Mutex

	client       adt.Client
	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	// systemUser returns the logon user configured for a system, "" when it
	// has none (OAuth2). It supplies the default `user` (#558).
	systemUser func(system string) string
	timings    debugTimings
	versions   atomic.Int64
	okCodes    okCodeCache
	// triggerer starts gui runs when the build provides one (#558).
	triggerer DebugTriggerer

	cur *adt.DebugSession
	// curCleanup is a second fresh session next to cur, bound to the same
	// user, system and IDE ID. Cleanup sends StopListener and the external
	// deletes from it: those endpoints are keyed by user and IDE ID, not by
	// cookie, so a wedged debug session cannot block them.
	curCleanup *adt.DebugSession
	key        debugSessionKey
	run        *debugRun
}

// newDebugSessions expects selector to be the same registry as client (as
// main.go passes it): the key records selector.ActiveName(), while
// adt.NewDebugSession binds the session to client's active system.
func newDebugSessions(client adt.Client, selector SystemSelector, systemUser func(string) string) *debugSessions {
	return &debugSessions{
		client: client,
		newSession: func(user string) *adt.DebugSession {
			return adt.NewDebugSession(client, user, processIDEID)
		},
		activeSystem: func() string {
			if selector == nil {
				return ""
			}
			return selector.ActiveName()
		},
		systemUser: systemUser,
		timings:    currentDebugTimings(),
	}
}

// nextVersion returns the next run-state version of this process.
func (m *debugSessions) nextVersion() int64 { return m.versions.Add(1) }

// newRun creates a run bound to m's version counter, timings and run-start lock.
func (m *debugSessions) newRun(p runParams) *debugRun {
	return newDebugRun(p, m.timings, m.nextVersion, &m.startMu)
}

// logonUser returns the logon user configured for system, "" when unknown.
func (m *debugSessions) logonUser(system string) string {
	if m.systemUser == nil {
		return ""
	}
	return strings.TrimSpace(m.systemUser(system))
}

// resolveUser returns the SAP user a debug tool acts for, upper-cased: the
// given one, or the logon user configured for the active system (#558).
func (m *debugSessions) resolveUser(given string) (string, error) {
	if u := strings.TrimSpace(given); u != "" {
		return strings.ToUpper(u), nil
	}
	system := m.activeSystem()
	if u := m.logonUser(system); u != "" {
		return strings.ToUpper(u), nil
	}
	return "", fmt.Errorf("no user given, and system %q has no configured logon user (OAuth2): pass user", system)
}

func (m *debugSessions) keyFor(user string) debugSessionKey {
	return debugSessionKey{user: strings.ToUpper(user), system: m.activeSystem()}
}

// createLocked creates the debug session and its cleanup session for user and
// returns them with the key they are bound to. adt.NewDebugSession reads the
// active system separately from keyFor, so a select_system in between would
// record the wrong system; the key is read again after creation and both
// rebuilt if it moved. Creating a session sends no request. Caller holds mu.
func (m *debugSessions) createLocked(user string) (*adt.DebugSession, *adt.DebugSession, debugSessionKey) {
	for {
		key := m.keyFor(user)
		sess, cleanup := m.newSession(user), m.newSession(user)
		if m.keyFor(user) == key {
			return sess, cleanup, key
		}
	}
}

// open returns the debug session and its cleanup session for user on the
// active system. A session bound to another user or system is replaced; its
// run is cleaned up first (#558). Caller holds startMu.
func (m *debugSessions) open(user string) (*adt.DebugSession, *adt.DebugSession, debugSessionKey) {
	m.mu.Lock()
	if m.cur != nil && m.key == m.keyFor(user) {
		defer m.mu.Unlock()
		return m.cur, m.curCleanup, m.key
	}
	run := m.run
	m.mu.Unlock()
	if run != nil {
		m.cleanupRun(run)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cur, m.curCleanup, m.key = m.createLocked(user)
	return m.cur, m.curCleanup, m.key
}

// use returns the current session for user (already resolved). A mismatching
// user or system is an error rather than a silent replacement that would
// discard an attached debuggee. Without a session, one is created.
func (m *debugSessions) use(user string) (*adt.DebugSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		m.cur, m.curCleanup, m.key = m.createLocked(user)
		return m.cur, nil
	}
	if key := m.keyFor(user); m.key != key {
		return nil, fmt.Errorf(
			"the current debug session belongs to user %s on system %q, not user %s on system %q; "+
				"call debug_run (or debug_stop) to start over with the new user or system",
			m.key.user, m.key.system, key.user, key.system)
	}
	return m.cur, nil
}

// callSession resolves user, runs fn on the session for that user (see use)
// and maps both ways it can fail to an MCP error result. debug_get_sessions
// uses it: it works in any state.
func (m *debugSessions) callSession(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	u, err := m.resolveUser(user)
	if err != nil {
		return nil, errorResult(err)
	}
	sess, err := m.use(u)
	if err != nil {
		return nil, errorResult(err)
	}
	data, err := fn(sess)
	if err != nil {
		return nil, errorResult(err)
	}
	return data, nil
}

// call runs fn for an in-attempt tool: only while a debuggee is attached, on
// the run's debug session. The idle timer pauses meanwhile.
func (m *debugSessions) call(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	run, err := m.attachedRun(user)
	if err != nil {
		return nil, errorResult(err)
	}
	if err := run.beginCall(); err != nil {
		return nil, errorResult(err)
	}
	defer run.endCall()
	data, err := fn(run.sess)
	if err != nil {
		return nil, errorResult(err)
	}
	return data, nil
}

// attachedRun returns the run for an in-attempt tool. A given user must be the
// run's user and the active system the run's system: a mismatch is an error
// rather than a silent replacement that would discard an attached debuggee.
func (m *debugSessions) attachedRun(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil {
		return nil, errNoDebuggee
	}
	if err := m.checkRunKeyLocked(user); err != nil {
		return nil, err
	}
	return m.run, nil
}

// checkRunKeyLocked compares user (when given) and the active system with the
// current run's. Caller holds mu; m.run is not nil.
func (m *debugSessions) checkRunKeyLocked(user string) error {
	u := strings.ToUpper(strings.TrimSpace(user))
	sys := m.activeSystem()
	if (u == "" || u == m.run.key.user) && sys == m.run.key.system {
		return nil
	}
	if u == "" {
		u = m.run.key.user
	}
	return fmt.Errorf("the current debug run belongs to user %s on system %q, not user %s on system %q; "+
		"call debug_run (or debug_stop) to start over with the new user or system",
		m.run.key.user, m.run.key.system, u, sys)
}

// step implements debug_step on the attached run.
func (m *debugSessions) step(ctx context.Context, user, action string) (stepOutcome, error) {
	run, err := m.attachedRun(user)
	if err != nil {
		return stepOutcome{}, err
	}
	return run.step(ctx, action)
}

// debugUserOnlyHandler builds the handler of a debug tool whose only argument
// is user: read the session's data with fn through via and shape it by build.
func debugUserOnlyHandler[T any](
	via func(string, func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult),
	fn func(*adt.DebugSession, context.Context) ([]byte, error),
	build func([]byte) T,
) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, errRes := via(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return fn(d, ctx)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(build(data))
	}
}

// startRun implements debug_run: it stops the previous run, sets every
// breakpoint in one request, starts listening in the background and, when the
// server starts the run itself, waits up to initialWait for it (#558).
func (m *debugSessions) startRun(ctx context.Context, a debugRunArgs) (DebugRunState, error) {
	user, hint, err := m.runUser(a)
	if err != nil {
		return DebugRunState{}, err
	}
	run, err := m.launchRun(ctx, a, user, hint)
	if err != nil {
		return DebugRunState{}, err
	}
	if !m.serverTriggers(a.kind) {
		return run.snapshot()
	}
	// The run-start lock is released: debug_stop must not wait for this.
	return run.waitUntil(ctx, m.timings.initialWait, settledAfterTrigger)
}

// settledAfterTrigger reports whether a server-triggered run has something to
// report: it was caught (or ended), or its trigger finished.
func settledAfterTrigger(s DebugRunState) bool {
	if s.Status != runListening && s.Status != runAttaching {
		return true
	}
	return s.Trigger != nil && (s.Trigger.State == triggerDone || s.Trigger.State == triggerFailed)
}

// runUser resolves debug_run's user. For unit_tests it must be the logon user,
// because the tests run as that user; the comparison ignores case. With an
// unknown logon user (OAuth2) the given user is accepted with a hint.
func (m *debugSessions) runUser(a debugRunArgs) (user, hint string, err error) {
	if a.kind != triggerUnitTests {
		u, err := m.resolveUser(a.user)
		return u, "", err
	}
	logon := strings.ToUpper(m.logonUser(m.activeSystem()))
	given := strings.ToUpper(strings.TrimSpace(a.user))
	switch {
	case given == "" && logon == "":
		_, err := m.resolveUser("")
		return "", "", err
	case given == "":
		return logon, "", nil
	case logon == "":
		return given, "The unit tests run as the logon user, which this server does not know (OAuth2); if that is not " +
			given + ", no breakpoint will be hit.", nil
	case given != logon:
		return "", "", fmt.Errorf("debug_run: trigger kind unit_tests runs the tests as the logon user %s, so user must be %s, not %s", logon, logon, given)
	}
	return given, "", nil
}

// serverTriggers reports whether the server starts runs of kind itself.
func (m *debugSessions) serverTriggers(kind string) bool {
	return kind == triggerUnitTests || (kind == triggerGUI && m.triggerer != nil)
}

// startTrigger starts the trigger goroutine of a server-run trigger. Its
// fallback instructions replace the trigger if it fails.
func (m *debugSessions) startTrigger(run *debugRun, a debugRunArgs, user string, guiAvail okCodeAvailability) {
	switch {
	case a.kind == triggerUnitTests:
		uri := a.unitObjectURI
		go run.runTrigger(func(ctx context.Context) (*adt.TestResult, error) {
			// RunUnitTests runs on a new isolated session of the run's system:
			// not the debug session, not the main client (adtler#204).
			secs := int((time.Until(run.budgetEnd) + run.timings.triggerSlack) / time.Second)
			return run.sess.RunUnitTests(ctx, uri, secs)
		}, manualInstructions(user, run.budgetEnd), true)
	case a.kind == triggerGUI && m.triggerer != nil:
		trig, system, target := m.triggerer, run.key.system, *a.target
		go run.runTrigger(func(ctx context.Context) (*adt.TestResult, error) {
			return nil, trig.TriggerDebugRun(ctx, system, user, target)
		}, guiInstructions(user, target, guiAvail), false)
	}
}

// launchRun is the part of debug_run that holds the run-start lock.
func (m *debugSessions) launchRun(ctx context.Context, a debugRunArgs, user, hint string) (*debugRun, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	m.mu.Lock()
	old := m.run
	m.mu.Unlock()
	if old != nil {
		m.cleanupRun(old)
	}

	sess, cleanupSess, key := m.open(user)
	guiAvail := okCodeUnknown
	if a.kind == triggerGUI {
		guiAvail = m.okCodes.lookup(ctx, key.system, m.objectInfoFor(key.system))
	}

	results, err := sess.SetBreakpoints(ctx, adt.BreakpointScopeExternal, a.breakpoints)
	if err != nil {
		return nil, fmt.Errorf("debug_run: setting the breakpoints failed: %w", err)
	}
	set, err := checkBreakpointResults(a.breakpoints, results)
	if err != nil {
		removeSetBreakpoints(ctx, sess, set)
		return nil, fmt.Errorf("debug_run: %w; the breakpoints that were set are removed again", err)
	}

	params := runParams{key: key, kind: a.kind, sess: sess, cleanupSess: cleanupSess, breakpoints: set, budget: a.timeout, hint: hint}
	if m.serverTriggers(a.kind) {
		params.trigger = &DebugTriggerState{Kind: a.kind, State: triggerPending}
	}
	run := m.newRun(params)
	run.source = m.sourceFor(key.system)
	switch {
	case a.kind == triggerManual:
		run.st.Instructions = manualInstructions(user, run.budgetEnd)
	case a.kind == triggerGUI && m.triggerer == nil:
		run.st.Instructions = guiInstructions(user, *a.target, guiAvail)
	}
	m.mu.Lock()
	m.run = run
	m.mu.Unlock()
	run.mu.Lock()
	run.startWindowLocked()
	run.mu.Unlock()
	m.startTrigger(run, a, user, guiAvail)
	return run, nil
}

// stop implements debug_stop: it cleans up the run and drops the session, so
// the next debug call starts on a new one (#562). The stopped run stays
// readable by debug_wait.
func (m *debugSessions) stop() cleanupReport {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	var rep cleanupReport
	if run != nil {
		rep = m.cleanupRun(run)
	}
	m.mu.Lock()
	m.cur, m.curCleanup, m.key = nil, nil, debugSessionKey{}
	m.mu.Unlock()
	return rep
}

// currentRun returns the run debug_wait reports on. A given user must be the
// run's user; a missing one is fine.
func (m *debugSessions) currentRun(user string) (*debugRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run == nil {
		return nil, errors.New("no debug run: start one with debug_run")
	}
	if u := strings.ToUpper(strings.TrimSpace(user)); u != "" && u != m.run.user {
		return nil, fmt.Errorf("the current debug run belongs to user %s, not %s", m.run.user, u)
	}
	return m.run, nil
}

var errOtherSystemActive = errors.New("another system is active than the one the run is bound to")

// sourceFor reads source text for the position excerpt. The main client is the
// registry, so it reads only while the run's system is the active one.
func (m *debugSessions) sourceFor(system string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, uri string) (string, error) {
		if m.client == nil || m.activeSystem() != system {
			return "", errOtherSystemActive
		}
		return readSourceText(ctx, m.client, uri)
	}
}

// readSourceText reads a source URI as a stack frame returns it: …/source/main
// (GetSource appends /source/main itself) or …/includes/<name>.
func readSourceText(ctx context.Context, client adt.Client, uri string) (string, error) {
	if base, ok := strings.CutSuffix(uri, "/source/main"); ok {
		res, err := client.GetSource(ctx, base)
		if err != nil {
			return "", err
		}
		return res.Source, nil
	}
	if i := strings.Index(uri, "/includes/"); i > 0 {
		res, err := client.GetIncludeSource(ctx, uri[:i], uri[i+len("/includes/"):])
		if err != nil {
			return "", err
		}
		return res.Source, nil
	}
	return "", fmt.Errorf("no source reader for %s", uri)
}

// objectInfoFor is the OK-code lookup: an ADT repository lookup, not a table
// read (scope guardrail). Like sourceFor it only asks the run's system.
func (m *debugSessions) objectInfoFor(system string) func(context.Context, string) error {
	return func(ctx context.Context, uri string) error {
		if m.client == nil || m.activeSystem() != system {
			return errOtherSystemActive
		}
		_, err := m.client.GetObjectInfo(ctx, uri)
		return err
	}
}

// removeSetBreakpoints deletes external breakpoints that were set before a
// later step failed; best effort.
func removeSetBreakpoints(ctx context.Context, sess *adt.DebugSession, set []DebugRunBreakpoint) {
	for _, bp := range set {
		if bp.ID != "" {
			_ = sess.RemoveBreakpoint(ctx, adt.BreakpointScopeExternal, bp.ID)
		}
	}
}

// setBreakpointWithoutRun is debug_set_breakpoint while no run is active.
// During a run it is refused: one external request replaces the run's
// breakpoints on SAP_BASIS 816 and would leave the stored IDs wrong.
func (m *debugSessions) setBreakpointWithoutRun(ctx context.Context, user string, bp adt.LineBreakpoint) (*adt.BreakpointResult, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	run := m.run
	m.mu.Unlock()
	if run != nil && run.active() {
		return nil, errors.New("debug_set_breakpoint: a debug_run is active; pass every breakpoint to debug_run instead")
	}
	sess, _, _ := m.open(user)
	return sess.SetBreakpoint(ctx, bp.ObjectURI, bp.Line, bp.ObjectType, bp.ObjectName)
}

// rearm starts the next listening window of run inside its remaining budget
// (debug_wait rearm). Only for manual and gui runs, after a timeout or after
// the debuggee has ended; a new unit-test run needs a new debug_run.
func (m *debugSessions) rearm(run *debugRun) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if run.kind == triggerUnitTests {
		return errors.New("debug_wait rearm: a unit-test run cannot listen again; start a new debug_run")
	}
	// The listener goroutine publishes timeout/ended just before it returns,
	// so wait (without run.mu) for it to finish before starting the next one.
	run.mu.Lock()
	status, done := run.st.Status, run.listenerDone
	run.mu.Unlock()
	if status != runTimeout && status != runEnded {
		return fmt.Errorf("debug_wait rearm: the run is %s; rearm needs status timeout or ended", status)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(run.timings.listenerExitWait):
			return errors.New("debug_wait rearm: the previous listener is still running; call debug_wait again")
		}
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if s := run.st.Status; s != runTimeout && s != runEnded {
		return fmt.Errorf("debug_wait rearm: the run is %s; rearm needs status timeout or ended", s)
	}
	if time.Until(run.budgetEnd) < time.Second {
		return errors.New("debug_wait rearm: the run's listener budget is used up; start a new debug_run")
	}
	run.fatal = nil
	run.transitionLocked(func(st *DebugRunState) {
		st.Status = runListening
		st.EndReason = ""
		st.DebuggeeID = ""
		st.Position = nil
		st.Hint = ""
	})
	run.startWindowLocked()
	return nil
}

// shutdown is the process-exit hook: debug_stop's cleanup, given up when ctx
// ends (the cleanup itself keeps its own 20 s budget).
func (m *debugSessions) shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.stop()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
