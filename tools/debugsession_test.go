package tools_test

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fakeDebugBackend answers the ADT debugger endpoints just well enough for
// debug_start / debug_stop to run, and records every request. The debug tools
// need a real adtler client (adt.NewDebugSession panics on anything else), so
// the fake sits at the transport level.
type fakeDebugBackend struct {
	mu   sync.Mutex
	reqs []recordedRequest
	// failStop makes the listener DELETE answer 500, as a wedged session would
	// fail it.
	failStop bool
}

type recordedRequest struct {
	host, method, path, query, body string
}

func (f *fakeDebugBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{req.URL.Host, req.Method, req.URL.Path, req.URL.RawQuery, body})
	failStop := f.failStop
	f.mu.Unlock()

	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: http.NoBody}
	switch {
	case req.Header.Get("X-CSRF-Token") == "Fetch":
		resp.Header.Set("X-CSRF-Token", "token")
	case failStop && req.Method == http.MethodDelete && req.URL.Path == listenersPath:
		resp.StatusCode = http.StatusInternalServerError
	case req.Method == http.MethodPost && req.URL.Path == "/sap/bc/adt/debugger/breakpoints":
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = io.NopCloser(strings.NewReader(
			`<?xml version="1.0" encoding="utf-8"?><dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger"><breakpoint kind="line" clientId="0" id="BP1"/></dbg:breakpoints>`))
	}
	// Everything else, including the listener POST, answers 200 with an empty
	// body, which adtler reads as a listener timeout.
	return resp, nil
}

// requests returns the recorded requests matching method and path.
func (f *fakeDebugBackend) requests(method, path string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recordedRequest
	for _, r := range f.reqs {
		if r.method == method && r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeDebugBackend) csrfFetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if r.method == http.MethodGet && r.path == "/sap/bc/adt/discovery" {
			n++
		}
	}
	return n
}

const (
	breakpointsPath = "/sap/bc/adt/debugger/breakpoints"
	listenersPath   = "/sap/bc/adt/debugger/listeners"
)

// newDebugServer registers the debug tools on a registry of two fake systems,
// sysA (host sap-a.test) and sysB (host sap-b.test), with sysA active.
func newDebugServer(t *testing.T) (*server.MCPServer, *adt.ClientRegistry, *fakeDebugBackend) {
	t.Helper()
	backend := &fakeDebugBackend{}
	clients := map[string]adt.Client{
		"sysA": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-a.test", User: "u", Password: "p"}, backend),
		"sysB": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-b.test", User: "u", Password: "p"}, backend),
	}
	reg, err := adt.NewClientRegistry(clients, "sysA")
	if err != nil {
		t.Fatal(err)
	}
	s := server.NewMCPServer("test", "0")
	tools.RegisterAllWithLockMap(s, reg, reg, adt.NewLockMap(), map[string]bool{"debug": true}, nil)
	return s, reg, backend
}

func debugStartArgs(user string) map[string]interface{} {
	return map[string]interface{}{
		"object_uri":      "/sap/bc/adt/programs/programs/zprog/source/main",
		"line":            3,
		"object_type":     "PROG/P",
		"object_name":     "ZPROG",
		"user":            user,
		"timeout_seconds": 1,
	}
}

// #562: the first debug_start fixed the user for the rest of the process.
// A later debug_start for another user must set its breakpoint and listener
// for that user, and stop the old user's listener.
func TestDebugStart_OtherUserReplacesSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("first debug_start: %v", res.Content)
	}
	if res := callTool(t, s, "debug_start", debugStartArgs("bob")); res.IsError {
		t.Fatalf("second debug_start: %v", res.Content)
	}

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || !strings.Contains(bps[1].body, `requestUser="BOB"`) {
		t.Errorf("second breakpoint must be set for BOB; breakpoint requests: %+v", bps)
	}
	listens := backend.requests(http.MethodPost, listenersPath)
	if len(listens) != 2 || !strings.Contains(listens[1].query, "requestUser=BOB") {
		t.Errorf("second listener must listen for BOB; listener requests: %+v", listens)
	}
	stops := backend.requests(http.MethodDelete, listenersPath)
	if len(stops) != 1 || !strings.Contains(stops[0].query, "requestUser=ALICE") {
		t.Errorf("replacing the session must stop ALICE's listener; stop requests: %+v", stops)
	}
}

// #562: the session copied the client of the system active at creation, so
// after select_system every debug call still went to the first system.
func TestDebugStart_SystemSwitchReplacesSession(t *testing.T) {
	s, reg, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start on sysA: %v", res.Content)
	}
	if _, err := reg.Select("sysB"); err != nil {
		t.Fatal(err)
	}
	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start on sysB: %v", res.Content)
	}

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].host != "sap-a.test" || bps[1].host != "sap-b.test" {
		t.Errorf("breakpoints must go to sysA, then sysB; got %+v", bps)
	}
	stops := backend.requests(http.MethodDelete, listenersPath)
	if len(stops) != 1 || stops[0].host != "sap-a.test" {
		t.Errorf("the replaced session's listener must be stopped on sysA; stop requests: %+v", stops)
	}
}

// Within a debugging attempt, a mismatching user must not silently replace
// the session: that would discard the debuggee the session is attached to.
func TestDebugInAttemptTool_OtherUserIsError(t *testing.T) {
	s, _, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start: %v", res.Content)
	}
	before := backend.count()

	res := callTool(t, s, "debug_get_stack", map[string]interface{}{"user": "bob"})
	if !res.IsError {
		t.Fatalf("debug_get_stack for another user must fail, got %v", res.Content)
	}
	if text := debugResultText(res); !strings.Contains(text, "debug_start") || !strings.Contains(text, "ALICE") {
		t.Errorf("error should name the session's user and the way out (debug_start), got %q", text)
	}
	if after := backend.count(); after != before {
		t.Errorf("a refused call must not reach SAP; %d new requests", after-before)
	}
}

// #562: a wedged session could only be recovered by restarting the server.
// debug_stop now drops the session, so the next debug_start starts on a
// fresh one (its own CSRF preflight is the visible sign of a new session).
func TestDebugStop_DropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start: %v", res.Content)
	}
	if res := callTool(t, s, "debug_stop", map[string]interface{}{"user": "alice"}); res.IsError {
		t.Fatalf("debug_stop: %v", res.Content)
	}
	if got := backend.csrfFetches(); got != 1 {
		t.Fatalf("expected 1 CSRF fetch before the restart, got %d", got)
	}
	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start after debug_stop: %v", res.Content)
	}
	if got := backend.csrfFetches(); got != 2 {
		t.Errorf("debug_start after debug_stop must run on a new session (new CSRF fetch); fetches = %d", got)
	}
}

func (f *fakeDebugBackend) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func debugResultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// debug_stop must drop the session even when stopping it fails: a wedged
// session is exactly the one whose StopListener fails, and keeping it would
// leave the restart as the only way out (#562).
func TestDebugStop_FailedStopStillDropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start: %v", res.Content)
	}
	backend.mu.Lock()
	backend.failStop = true
	backend.mu.Unlock()
	if res := callTool(t, s, "debug_stop", map[string]interface{}{"user": "alice"}); !res.IsError {
		t.Fatalf("debug_stop must report the failed stop, got %v", res.Content)
	}
	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start after the failed debug_stop: %v", res.Content)
	}
	if got := backend.csrfFetches(); got != 2 {
		t.Errorf("debug_start after a failed debug_stop must run on a new session; CSRF fetches = %d", got)
	}
}

// debug_stop for a user other than the current session's stops both: the
// current session, which is dropped, and any listener of the named user.
func TestDebugStop_OtherUserStopsBoth(t *testing.T) {
	s, _, backend := newDebugServer(t)

	if res := callTool(t, s, "debug_start", debugStartArgs("alice")); res.IsError {
		t.Fatalf("debug_start: %v", res.Content)
	}
	if res := callTool(t, s, "debug_stop", map[string]interface{}{"user": "bob"}); res.IsError {
		t.Fatalf("debug_stop: %v", res.Content)
	}
	stops := backend.requests(http.MethodDelete, listenersPath)
	var alice, bob bool
	for _, r := range stops {
		alice = alice || strings.Contains(r.query, "requestUser=ALICE")
		bob = bob || strings.Contains(r.query, "requestUser=BOB")
	}
	if !alice || !bob {
		t.Errorf("debug_stop for BOB must stop ALICE's current session and BOB's listener; stop requests: %+v", stops)
	}
}
