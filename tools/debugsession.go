package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// replacedListenerStopTimeout bounds the best-effort StopListener on a session
// that is being replaced. A replaced session may be the wedged one (#562), so
// waiting for its full HTTP timeout would stall the call that replaces it.
const replacedListenerStopTimeout = 10 * time.Second

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

	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	// systemUser returns the logon user configured for a system, "" when it
	// has none (OAuth2). It supplies the default `user` (#558).
	systemUser func(system string) string
	timings    debugTimings
	versions   atomic.Int64

	cur *adt.DebugSession
	key debugSessionKey
	run *debugRun
}

// newDebugSessions expects selector to be the same registry as client (as
// main.go passes it): the key records selector.ActiveName(), while
// adt.NewDebugSession binds the session to client's active system.
func newDebugSessions(client adt.Client, selector SystemSelector, systemUser func(string) string) *debugSessions {
	return &debugSessions{
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

// createLocked creates a session for user and returns it with the key it is
// bound to. adt.NewDebugSession reads the active system separately from
// keyFor, so a select_system in between would record the wrong system; the
// key is read again after creation and the session rebuilt if it moved.
// Creating a session sends no request. Caller holds mu.
func (m *debugSessions) createLocked(user string) (*adt.DebugSession, debugSessionKey) {
	for {
		key := m.keyFor(user)
		sess := m.newSession(user)
		if m.keyFor(user) == key {
			return sess, key
		}
	}
}

// open returns the session for user on the active system, for the tools that
// begin a debugging attempt (debug_start, debug_set_breakpoint). A session
// bound to another user or system is replaced; its listener is stopped on a
// best-effort basis so it does not keep catching runs nobody waits for. A
// debuggee the old listener caught just before that stop belongs to the old
// session and can no longer be attached from here.
func (m *debugSessions) open(ctx context.Context, user string) *adt.DebugSession {
	m.mu.Lock()
	old := m.cur
	if old != nil && m.key == m.keyFor(user) {
		m.mu.Unlock()
		return old
	}
	sess, key := m.createLocked(user)
	m.cur, m.key = sess, key
	m.mu.Unlock()

	if old != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), replacedListenerStopTimeout)
		_ = old.StopListener(stopCtx)
		cancel()
	}
	return sess
}

// use returns the current session for the tools that work inside an attempt
// (attach, step, variables, stack, watchpoints, debuggee list). Those calls
// only make sense on the session that caught the debuggee, so a mismatching
// user or system is an error rather than a silent replacement that would
// discard an attached debuggee. Without a session, one is created.
func (m *debugSessions) use(user string) (*adt.DebugSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		m.cur, m.key = m.createLocked(user)
		return m.cur, nil
	}
	if key := m.keyFor(user); m.key != key {
		return nil, fmt.Errorf(
			"the current debug session belongs to user %s on system %q, not user %s on system %q; "+
				"call debug_start (or debug_stop) to start over with the new user or system",
			m.key.user, m.key.system, key.user, key.system)
	}
	return m.cur, nil
}

// take removes the current session and returns every session debug_stop has
// to stop: the current one, and a fresh one for user on the active system
// unless the current one is exactly that. The fresh one removes a listener
// left behind for that user, e.g. by an earlier server process. The caller
// owns the returned sessions; the next debug call starts on a new one, which
// is the recovery path for a wedged session.
func (m *debugSessions) take(user string) []*adt.DebugSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*adt.DebugSession
	if m.cur != nil {
		out = append(out, m.cur)
	}
	if m.cur == nil || m.key != m.keyFor(user) {
		sess, _ := m.createLocked(user)
		out = append(out, sess)
	}
	m.cur = nil
	m.key = debugSessionKey{}
	return out
}

// call runs fn on the session for user (see use) and maps both ways it can
// fail, a mismatching session and a failed SAP call, to an MCP error result.
func (m *debugSessions) call(user string, fn func(*adt.DebugSession) ([]byte, error)) ([]byte, *mcp.CallToolResult) {
	sess, err := m.use(user)
	if err != nil {
		return nil, errorResult(err)
	}
	data, err := fn(sess)
	if err != nil {
		return nil, errorResult(err)
	}
	return data, nil
}

// debugUserOnlyHandler builds the handler of a debug tool whose only argument
// is user: read the session's data with fn and return it shaped by build.
func debugUserOnlyHandler[T any](
	sessions *debugSessions,
	toolName string,
	fn func(*adt.DebugSession, context.Context) ([]byte, error),
	build func([]byte) T,
) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		if err := requireDebuggerStringParam(toolName, "user", user); err != nil {
			return errorResult(err), nil
		}
		data, errRes := sessions.call(user, func(d *adt.DebugSession) ([]byte, error) {
			return fn(d, ctx)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(build(data))
	}
}
