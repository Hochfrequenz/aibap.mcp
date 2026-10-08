package tools_test

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	breakpointsPath = "/sap/bc/adt/debugger/breakpoints"
	listenersPath   = "/sap/bc/adt/debugger/listeners"
	debuggerPath    = "/sap/bc/adt/debugger"
	unitTestsPath   = "/sap/bc/adt/abapunit/testruns"
	okCodeProgPath  = "/sap/bc/adt/programs/programs/rs_adtdbg_activate_by_okcode"
	progURI         = "/sap/bc/adt/programs/programs/zprog/source/main"
	otherURI        = "/sap/bc/adt/programs/programs/zother/source/main"
)

// fakeError is an ADT error answer. endsRun releases a halted debuggee (the
// unit-test request returns) the way a step that ends the run does.
type fakeError struct {
	status  int
	typ     string
	props   map[string]string
	endsRun bool
}

// fakeDebugBackend answers the ADT debugger endpoints at the transport level
// and records every request. The debug tools need a real adtler client
// (adt.NewDebugSession panics on anything else), so the fake sits below it.
// Every CSRF fetch opens a "session": its answer sets the cookie sid=s<n>, so
// a recorded request's cookie tells which adtler session sent it.
type fakeDebugBackend struct {
	// hits feeds a waiting listener POST: "" answers like a listener timeout,
	// anything else is the caught debuggee's ID. Set once, never replaced.
	hits chan string

	mu          sync.Mutex
	reqs        []recordedRequest
	sessions    int
	listenStop  chan struct{}
	bpCounter   int
	released    chan struct{}
	releaseOnce sync.Once

	failStop     bool
	listenerErr  *fakeError
	attachGate   chan struct{}
	attachErr    *fakeError
	stackXML     string
	stackGate    chan struct{}
	sessionsBody string
	sessionsErr  *fakeError // getDebuggeeSessions answers this error (nil: sessionsBody)
	stepErr      map[string]fakeError
	unitHit      string
	unitErr      *fakeError
	rejectBP     map[string][2]string // URI substring → errorKind, errorMessage
	existingBP   map[string]bool      // URI substring → answered with errorKind "existing"
	bpDeleteErr  *fakeError
	bpDeleteID   string               // when set, only the DELETE of this breakpoint ID fails with bpDeleteErr
	okCodeStatus int                  // status of the OK-code program lookup; 0 means 200
	hangCookie   string               // requests with this session cookie hang until their context ends
	hang         map[string]bool      // "METHOD path" → hangs until the request's context ends
	vars         map[string]fakeVar   // variable ID -> metadata and value
	children     map[string][]fakeVar // parent ID -> its children
	bpPostErr    []*fakeError         // answers of the next breakpoint POSTs, in order; nil = normal
	bpSetGate    chan struct{}        // a breakpoint POST waits for this gate (nil: no wait)
	detachGate   chan struct{}        // a detachDebugger request waits for this gate (nil: no wait)
	bpSetDone    func()               // called once a breakpoint POST's answer body was read to its end (nil: none)
}

// onEOFReader calls fn once, when the wrapped body is read to its end.
type onEOFReader struct {
	io.Reader
	fn   func()
	once sync.Once
}

func (r *onEOFReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.once.Do(r.fn)
	}
	return n, err
}

// stepDetach is the debugger step that detaches the debuggee.
const stepDetach = "detachDebugger"

// Values the debug run tests compare against (runAttachedStatus is in
// debugrun_trigger_test.go).
const (
	testDebuggeeID   = "DBG1"
	runEndedStatus   = "ended"
	endDetachedStr   = "detached"
	triggerDoneState = "done"
)

type recordedRequest struct {
	host, method, path, query, body, cookie string
	stateful                                bool
}

const defaultStackXML = `<?xml version="1.0" encoding="utf-8"?><dbg:stack xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core">` +
	`<stackEntry stackPosition="1" programName="ZPROG" includeName="ZPROG" line="3" eventType="EVENT" eventName="START-OF-SELECTION" systemProgram="false" isActive="true" adtcore:uri="/sap/bc/adt/programs/programs/zprog/source/main#start=3,0"/>` +
	`</dbg:stack>`

func newFakeDebugBackend() *fakeDebugBackend {
	return &fakeDebugBackend{hits: make(chan string), released: make(chan struct{}), stackXML: defaultStackXML}
}

func textBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func sourceLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	return strings.Join(lines, "\n")
}

func answerError(resp *http.Response, e fakeError) *http.Response {
	keys := make([]string, 0, len(e.props))
	for k := range e.props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var props strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&props, `<entry key="%s">%s</entry>`, k, e.props[k])
	}
	resp.StatusCode = e.status
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework">` +
		`<namespace id="com.sap.adt"/><type id="` + e.typ + `"/><message lang="EN">fake ` + e.typ + `</message>` +
		`<properties>` + props.String() + `</properties></exc:exception>`)
	return resp
}

func (f *fakeDebugBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	// Like http.Transport: a request whose context is already done never reaches SAP.
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return f.answer(req)
}

func (f *fakeDebugBackend) answer(req *http.Request) (*http.Response, error) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	cookie := ""
	if c, err := req.Cookie("sid"); err == nil {
		cookie = c.Value
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedRequest{
		host: req.URL.Host, method: req.Method, path: req.URL.Path, query: req.URL.RawQuery, body: body, cookie: cookie,
		stateful: req.Header.Get("X-sap-adt-sessiontype") == "stateful",
	})
	hang := (f.hangCookie != "" && cookie == f.hangCookie) || f.hang[req.Method+" "+req.URL.Path]
	f.mu.Unlock()
	if hang {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}

	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req, Body: http.NoBody}
	p := req.URL.Path
	switch {
	case req.Header.Get("X-CSRF-Token") == "Fetch":
		f.mu.Lock()
		f.sessions++
		n := f.sessions
		f.mu.Unlock()
		resp.Header.Set("X-CSRF-Token", "token")
		resp.Header.Add("Set-Cookie", fmt.Sprintf("sid=s%d; Path=/", n))
	case p == listenersPath && req.Method == http.MethodPost:
		return f.listen(req, resp)
	case p == listenersPath && req.Method == http.MethodDelete:
		f.mu.Lock()
		fail := f.failStop
		if f.listenStop != nil {
			close(f.listenStop)
			f.listenStop = nil
		}
		f.mu.Unlock()
		if fail {
			return answerError(resp, fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}), nil
		}
	case p == breakpointsPath && req.Method == http.MethodPost:
		f.mu.Lock()
		gate := f.bpSetGate
		f.mu.Unlock()
		if gate != nil {
			select {
			case <-gate:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return f.setBreakpoints(resp, body), nil
	case strings.HasPrefix(p, breakpointsPath+"/") && req.Method == http.MethodDelete:
		f.mu.Lock()
		e := f.bpDeleteErr
		if f.bpDeleteID != "" && !strings.HasSuffix(p, "/"+f.bpDeleteID) {
			e = nil
		}
		f.mu.Unlock()
		if e != nil {
			return answerError(resp, *e), nil
		}
	case p == debuggerPath:
		return f.debugger(req, resp, body)
	case p == unitTestsPath:
		return f.unitTests(req, resp)
	case p == okCodeProgPath:
		f.mu.Lock()
		st := f.okCodeStatus
		f.mu.Unlock()
		if st != 0 && st != http.StatusOK {
			return answerError(resp, fakeError{status: st}), nil
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<program:abapProgram xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="RS_ADTDBG_ACTIVATE_BY_OKCODE" adtcore:type="PROG/P"/>`)
	case req.Method == http.MethodGet && strings.HasSuffix(p, "/source/main"):
		resp.Header.Set("Content-Type", "text/plain")
		resp.Body = textBody(sourceLines(20))
	}
	return resp, nil
}

func (f *fakeDebugBackend) listen(req *http.Request, resp *http.Response) (*http.Response, error) {
	f.mu.Lock()
	if f.listenerErr != nil {
		e := *f.listenerErr
		f.mu.Unlock()
		return answerError(resp, e), nil
	}
	if f.listenStop == nil {
		f.listenStop = make(chan struct{})
	}
	stop := f.listenStop
	f.mu.Unlock()
	select {
	case id := <-f.hits:
		if id != "" {
			resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
			resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA><DEBUGGEE_ID>` +
				id + `</DEBUGGEE_ID></DATA></asx:values></asx:abap>`)
		}
		return resp, nil
	case <-stop:
		return resp, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

var bpEntry = regexp.MustCompile(`<breakpoint kind="line" clientId="(\d+)" adtcore:uri="([^"#]+)#start=(\d+)`)

func (f *fakeDebugBackend) setBreakpoints(resp *http.Response, body string) *http.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bpPostErr) > 0 {
		e := f.bpPostErr[0]
		f.bpPostErr = f.bpPostErr[1:]
		if e != nil {
			resp = answerError(resp, *e)
			if f.bpSetDone != nil {
				resp.Body = io.NopCloser(&onEOFReader{Reader: resp.Body, fn: f.bpSetDone})
			}
			return resp
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">`)
	for _, m := range bpEntry.FindAllStringSubmatch(body, -1) {
		clientID, uri := m[1], m[2]
		if rej, ok := matchURI(f.rejectBP, uri); ok {
			fmt.Fprintf(&b, `<breakpoint kind="line" clientId="%s" errorKind="%s" errorMessage="%s"/>`, clientID, rej[0], rej[1])
			continue
		}
		f.bpCounter++
		extra := ""
		for k := range f.existingBP {
			if strings.Contains(uri, k) {
				extra = ` errorKind="existing"`
			}
		}
		fmt.Fprintf(&b, `<breakpoint kind="line" clientId="%s" id="BP%d"%s/>`, clientID, f.bpCounter, extra)
	}
	b.WriteString(`</dbg:breakpoints>`)
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(b.String())
	if f.bpSetDone != nil {
		resp.Body = io.NopCloser(&onEOFReader{Reader: resp.Body, fn: f.bpSetDone})
	}
	return resp
}

func matchURI(m map[string][2]string, uri string) ([2]string, bool) {
	for k, v := range m {
		if strings.Contains(uri, k) {
			return v, true
		}
	}
	return [2]string{}, false
}

func (f *fakeDebugBackend) debugger(req *http.Request, resp *http.Response, body string) (*http.Response, error) {
	method := req.URL.Query().Get("method")
	f.mu.Lock()
	attachGate, attachErr, stack, stackGate, sessions := f.attachGate, f.attachErr, f.stackXML, f.stackGate, f.sessionsBody
	sessionsErr := f.sessionsErr
	detachGate := f.detachGate
	stepErr, hasStepErr := f.stepErr[method]
	f.mu.Unlock()
	wait := func(gate chan struct{}) error {
		if gate == nil {
			return nil
		}
		select {
		case <-gate:
			return nil
		case <-req.Context().Done():
			return req.Context().Err()
		}
	}
	switch method {
	case "attach":
		if err := wait(attachGate); err != nil {
			return nil, err
		}
		if attachErr != nil {
			return answerError(resp, *attachErr), nil
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<dbg:attach xmlns:dbg="http://www.sap.com/adt/debugger"/>`)
	case "getStack":
		if err := wait(stackGate); err != nil {
			return nil, err
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(stack)
	case "getDebuggeeSessions":
		if sessionsErr != nil {
			return answerError(resp, *sessionsErr), nil
		}
		resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
		resp.Body = textBody(sessions)
	case "stepInto", "stepOver", "stepReturn", "stepContinue", "terminateDebuggee", stepDetach:
		if method != stepDetach {
			detachGate = nil
		}
		if err := wait(detachGate); err != nil {
			return nil, err
		}
		if hasStepErr {
			if stepErr.endsRun {
				f.release()
			}
			return answerError(resp, stepErr), nil
		}
		if method == stepDetach || method == "terminateDebuggee" {
			f.release()
		}
		resp.Header.Set("Content-Type", "application/xml")
		resp.Body = textBody(`<dbg:step xmlns:dbg="http://www.sap.com/adt/debugger"/>`)
	case "getVariableValue":
		f.mu.Lock()
		v := f.vars[req.URL.Query().Get("variableName")]
		f.mu.Unlock()
		resp.Header.Set("Content-Type", "text/plain")
		resp.Body = textBody(v.value)
	case "getVariables", "getChildVariables", "getVariableData":
		return f.variables(resp, method, body), nil
	}
	return resp, nil
}

func (f *fakeDebugBackend) unitTests(req *http.Request, resp *http.Response) (*http.Response, error) {
	f.mu.Lock()
	hit, uerr, released := f.unitHit, f.unitErr, f.released
	f.mu.Unlock()
	if uerr != nil {
		return answerError(resp, *uerr), nil
	}
	if hit != "" {
		select {
		case f.hits <- hit:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		select {
		case <-released:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	resp.Header.Set("Content-Type", "application/xml")
	resp.Body = textBody(`<?xml version="1.0" encoding="utf-8"?><aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit"><program><testClasses>` +
		`<testClass name="LTC_TEST"><testMethods><testMethod name="TEST_HELLO" executionTime="0.01"/></testMethods></testClass>` +
		`</testClasses></program></aunit:runResult>`)
	return resp, nil
}

func (f *fakeDebugBackend) release() { f.releaseOnce.Do(func() { close(f.released) }) }

// set changes the fake's configuration under its lock.
func (f *fakeDebugBackend) set(fn func(f *fakeDebugBackend)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// hit answers the waiting listener with debuggee id ("" = listener timeout).
func (f *fakeDebugBackend) hit(t *testing.T, id string) {
	t.Helper()
	select {
	case f.hits <- id:
	case <-time.After(5 * time.Second):
		t.Fatalf("no listener was waiting for hit %q", id)
	}
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

// index returns the position of the first request matching method, path and
// a query substring, or -1.
func (f *fakeDebugBackend) index(method, path, query string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.reqs {
		if r.method == method && r.path == path && strings.Contains(r.query, query) {
			return i
		}
	}
	return -1
}

// waitForRequest polls until a matching request was recorded.
func (f *fakeDebugBackend) waitForRequest(t *testing.T, method, path, query string) recordedRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, r := range f.requests(method, path) {
			if strings.Contains(r.query, query) {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s %s?%s request was sent", method, path, query)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cookieOf returns the session cookie of the first request matching method and path.
func (f *fakeDebugBackend) cookieOf(method, path string) string {
	if rs := f.requests(method, path); len(rs) > 0 {
		return rs[0].cookie
	}
	return ""
}

func (f *fakeDebugBackend) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

var fastDebugTimings = tools.DebugTimingsForTest{
	TriggerDelay:     10 * time.Millisecond,
	InitialWait:      3 * time.Second,
	CleanupBudget:    3 * time.Second,
	ListenerExitWait: time.Second,
	IdleLimit:        time.Hour,
	AttachTimeout:    3 * time.Second,
}

// newDebugServer registers the debug tools on a registry of two fake systems,
// sysA (host sap-a.test) and sysB (host sap-b.test), with sysA active and
// "alice" as the configured logon user of both.
func newDebugServer(t *testing.T, opts ...tools.RegisterOption) (*server.MCPServer, *adt.ClientRegistry, *fakeDebugBackend) {
	t.Helper()
	return newDebugServerTimed(t, fastDebugTimings, nil, opts...)
}

func newDebugServerTimed(t *testing.T, timings tools.DebugTimingsForTest, fallback tools.BlackMagicClient, opts ...tools.RegisterOption) (*server.MCPServer, *adt.ClientRegistry, *fakeDebugBackend) {
	t.Helper()
	tools.SetDebugTimingsForTest(t, timings)
	backend := newFakeDebugBackend()
	clients := map[string]adt.Client{
		"sysA": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-a.test", User: "u", Password: "p"}, backend),
		"sysB": adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-b.test", User: "u", Password: "p"}, backend),
	}
	reg, err := adt.NewClientRegistry(clients, "sysA")
	if err != nil {
		t.Fatal(err)
	}
	s := server.NewMCPServer("test", "0")
	all := append([]tools.RegisterOption{tools.WithSystemUser(func(string) string { return "alice" })}, opts...)
	tools.RegisterAllWithLockMap(s, reg, reg, adt.NewLockMap(), map[string]bool{"debug": true}, fallback, all...)
	// Stop whatever run the test left, so no listener goroutine outlives it.
	t.Cleanup(func() { callTool(t, s, "debug_stop", map[string]interface{}{}) })
	return s, reg, backend
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

// runState decodes a successful debug_run/debug_wait result.
func runState(t *testing.T, res *mcp.CallToolResult) tools.DebugRunState {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %s", debugResultText(res))
	}
	var st tools.DebugRunState
	if err := json.Unmarshal([]byte(debugResultText(res)), &st); err != nil {
		t.Fatalf("decode run state: %v\n%s", err, debugResultText(res))
	}
	return st
}

// waitForStatus calls debug_wait until the run reaches status. It reports
// failures with t.Errorf, so it may run in a goroutine.
func waitForStatus(t *testing.T, s *server.MCPServer, since int64, status string) (tools.DebugRunState, bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res := callTool(t, s, "debug_wait", map[string]interface{}{"since_version": since, "timeout_seconds": 2})
		var st tools.DebugRunState
		if res.IsError || json.Unmarshal([]byte(debugResultText(res)), &st) != nil {
			t.Errorf("debug_wait failed: %s", debugResultText(res))
			return st, false
		}
		if st.Status == status {
			return st, true
		}
		since = st.Version
	}
	t.Errorf("the run did not reach status %q", status)
	return tools.DebugRunState{}, false
}

func mustStatus(t *testing.T, s *server.MCPServer, since int64, status string) tools.DebugRunState {
	t.Helper()
	st, ok := waitForStatus(t, s, since, status)
	if !ok {
		t.FailNow()
	}
	return st
}

// manualRunArgs are debug_run arguments for a manual run with one breakpoint
// at line 3 of each URI (default: progURI).
func manualRunArgs(user string, uris ...string) map[string]interface{} {
	if len(uris) == 0 {
		uris = []string{progURI}
	}
	bps := make([]interface{}, 0, len(uris))
	for _, u := range uris {
		bps = append(bps, map[string]interface{}{"object_uri": u, "line": 3})
	}
	args := map[string]interface{}{
		"breakpoints":     bps,
		"trigger":         map[string]interface{}{"kind": "manual"},
		"timeout_seconds": 60,
	}
	if user != "" {
		args["user"] = user
	}
	return args
}

// #562: the first debug run fixed the user for the rest of the process. A
// later debug_run for another user must set its breakpoints and listener for
// that user, after cleaning up the old user's run.
func TestDebugRun_OtherUserReplacesSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	runState(t, callTool(t, s, "debug_run", manualRunArgs("bob")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=BOB")

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || !strings.Contains(bps[1].body, `requestUser="BOB"`) {
		t.Errorf("second breakpoint request must be for BOB; got %+v", bps)
	}
	if backend.index(http.MethodDelete, listenersPath, "requestUser=ALICE") < 0 {
		t.Error("replacing the session must stop ALICE's listener")
	}
	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("replacing the session must delete ALICE's breakpoint BP1")
	}
}

// #562: after select_system every debug call still went to the first system.
func TestDebugRun_SystemSwitchReplacesSession(t *testing.T) {
	s, reg, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if _, err := reg.Select("sysB"); err != nil {
		t.Fatal(err)
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].host != "sap-a.test" || bps[1].host != "sap-b.test" {
		t.Errorf("breakpoints must go to sysA, then sysB; got %+v", bps)
	}
	stops := backend.requests(http.MethodDelete, listenersPath)
	if len(stops) == 0 || stops[0].host != "sap-a.test" {
		t.Errorf("the replaced run's listener must be stopped on sysA; stop requests: %+v", stops)
	}
}

// Within a debugging attempt, a mismatching user must not silently replace
// the session: that would discard the debuggee the session is attached to.
func TestDebugInAttemptTool_OtherUserIsError(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	before := backend.count()

	res := callTool(t, s, "debug_get_stack", map[string]interface{}{"user": "bob"})
	if !res.IsError {
		t.Fatalf("debug_get_stack for another user must fail, got %v", res.Content)
	}
	if text := debugResultText(res); !strings.Contains(text, "debug_run") || !strings.Contains(text, "ALICE") {
		t.Errorf("error should name the run's user and the way out (debug_run), got %q", text)
	}
	if after := backend.count(); after != before {
		t.Errorf("a refused call must not reach SAP; %d new requests", after-before)
	}
}

// #562: a wedged session could only be recovered by restarting the server.
// debug_stop drops the session, so the next debug_run starts on a new one.
func TestDebugStop_DropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatalf("debug_stop: %s", debugResultText(res))
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].cookie == "" || bps[0].cookie == bps[1].cookie {
		t.Errorf("debug_run after debug_stop must run on a new session; breakpoint cookies: %+v", bps)
	}
}

// debug_stop must drop the session even when stopping fails: a wedged session
// is exactly the one whose stop fails (#562).
func TestDebugStop_FailedStopStillDropsSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) { f.failStop = true })
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError || !strings.Contains(debugResultText(res), `"listener_error"`) {
		t.Fatalf("debug_stop must report the failed stop in listener_error, got %s", debugResultText(res))
	}
	backend.set(func(f *fakeDebugBackend) { f.failStop = false })
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))

	bps := backend.requests(http.MethodPost, breakpointsPath)
	if len(bps) != 2 || bps[0].cookie == bps[1].cookie {
		t.Errorf("debug_run after a failed debug_stop must run on a new session; breakpoint cookies: %+v", bps)
	}
}

// fakeVar is one debuggee variable of the fake.
type fakeVar struct {
	id, name, meta, value string
	lines                 int
}

var (
	asxIDs     = regexp.MustCompile(`<ID>([^<]*)</ID>`)
	asxParents = regexp.MustCompile(`<PARENT_ID>([^<]*)</PARENT_ID>`)
	dataTable  = regexp.MustCompile(`<table name="([^"]*)" offset="(\d+)" length="(\d+)"`)
)

const (
	asxHead = `<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>`
	asxTail = `</DATA></asx:values></asx:abap>`
)

func asxVar(v fakeVar) string {
	return fmt.Sprintf(`<STPDA_ADT_VARIABLE><ID>%s</ID><NAME>%s</NAME><META_TYPE>%s</META_TYPE><VALUE>%s</VALUE><TABLE_LINES>%d</TABLE_LINES></STPDA_ADT_VARIABLE>`,
		html.EscapeString(v.id), html.EscapeString(v.name), v.meta, html.EscapeString(v.value), v.lines)
}

// variables answers getVariables, getChildVariables and getVariableData. The
// request bodies are XML, so IDs such as REF->* arrive escaped.
func (f *fakeDebugBackend) variables(resp *http.Response, method, body string) *http.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	switch method {
	case "getVariables":
		b.WriteString(asxHead)
		for _, m := range asxIDs.FindAllStringSubmatch(body, -1) {
			if v, ok := f.vars[html.UnescapeString(m[1])]; ok {
				b.WriteString(asxVar(v))
			}
		}
		b.WriteString(asxTail)
	case "getChildVariables":
		var links strings.Builder
		b.WriteString(asxHead + "<VARIABLES>")
		for _, m := range asxParents.FindAllStringSubmatch(body, -1) {
			parent := html.UnescapeString(m[1])
			for _, c := range f.children[parent] {
				b.WriteString(asxVar(c))
				fmt.Fprintf(&links, `<STPDA_ADT_VARIABLE_HIERARCHY><PARENT_ID>%s</PARENT_ID><CHILD_ID>%s</CHILD_ID><CHILD_NAME>%s</CHILD_NAME></STPDA_ADT_VARIABLE_HIERARCHY>`,
					html.EscapeString(parent), html.EscapeString(c.id), html.EscapeString(c.name))
			}
		}
		b.WriteString("</VARIABLES><HIERARCHIES>" + links.String() + "</HIERARCHIES>" + asxTail)
	case "getVariableData":
		m := dataTable.FindStringSubmatch(body)
		if m == nil {
			break
		}
		offset, _ := strconv.Atoi(m[2])
		length, _ := strconv.Atoi(m[3])
		fmt.Fprintf(&b, `<dbg:data xmlns:dbg="http://www.sap.com/adt/debugger"><table name="%s" totalLines="%d">`, m[1], f.vars[m[1]].lines)
		for i := offset; i < offset+length; i++ {
			fmt.Fprintf(&b, `<tableLine index="%d"><field path="TEXT"><value>row %d</value></field></tableLine>`, i, i)
		}
		b.WriteString(`</table></dbg:data>`)
	}
	resp.Header.Set("Content-Type", "application/vnd.sap.as+xml")
	resp.Body = textBody(b.String())
	return resp
}
