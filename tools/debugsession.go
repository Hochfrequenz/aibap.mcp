package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// replacedListenerStopTimeout bounds the best-effort StopListener on a session
// that is being replaced. A replaced session may be the wedged one (#562), so
// waiting for its full HTTP timeout would stall the call that replaces it.
const replacedListenerStopTimeout = 10 * time.Second

// debugSessionKey identifies what a DebugSession is bound to. adt.NewDebugSession
// fixes both at creation: the SAP user (upper-cased) and the system that was
// active then, because it copies that system's client (freshSession).
type debugSessionKey struct {
	user   string
	system string
}

// debugSessions owns the one DebugSession the debug tools share. Before #562
// it was created on the first call and kept for the life of the process, so a
// later user, a system switch or a wedged session could not be recovered from
// without a restart.
//
// Tool calls run concurrently (mcp-go's worker pool), so every access holds mu.
// HTTP calls on a session happen outside the lock.
type debugSessions struct {
	mu           sync.Mutex
	newSession   func(user string) *adt.DebugSession
	activeSystem func() string
	cur          *adt.DebugSession
	key          debugSessionKey
}

// newDebugSessions expects selector to be the same registry as client (as
// main.go passes it): the key records selector.ActiveName(), while
// adt.NewDebugSession binds the session to client's active system.
func newDebugSessions(client adt.Client, selector SystemSelector) *debugSessions {
	return &debugSessions{
		newSession: func(user string) *adt.DebugSession {
			return adt.NewDebugSession(client, user, "aibap.mcp")
		},
		activeSystem: func() string {
			if selector == nil {
				return ""
			}
			return selector.ActiveName()
		},
	}
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
