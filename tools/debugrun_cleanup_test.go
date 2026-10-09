package tools_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// Run statuses this file compares against.
const (
	runListeningStatus = "listening"
	runStoppedStatus   = "stopped"
)

// Spec cleanup order: StopListener and the external deletes (step 2) before
// the detach of an attached debuggee in the debug session (step 3).
func TestDebugStop_AttachedRunIsDetachedAfterTheExternalCleanup(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	mustStatus(t, s, st.Version, runAttachedStatus)
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	detach := backend.index(http.MethodPost, debuggerPath, "method=detachDebugger")
	stop := backend.index(http.MethodDelete, listenersPath, "")
	del := backend.index(http.MethodDelete, breakpointsPath+"/BP1", "")
	if detach < 0 || stop < 0 || del < 0 || detach < stop || detach < del {
		t.Fatalf("want listener stop (%d) and delete (%d) before detach (%d)", stop, del, detach)
	}
	if d := backend.requests(http.MethodPost, debuggerPath); d[len(d)-1].cookie != backend.cookieOf(http.MethodPost, breakpointsPath) {
		t.Error("the detach must be sent in the debug session")
	}
}

// A hit that arrives during cleanup, with the attach completing after the
// listener-exit wait: the listener goroutine still sends a detach.
func TestDebugStop_LateAttachIsDetachedByTheListener(t *testing.T) {
	timings := fastDebugTimings
	timings.ListenerExitWait = 200 * time.Millisecond
	timings.AttachTimeout = 10 * time.Second
	s, _, backend := newDebugServerTimed(t, timings, nil)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.attachGate = gate })

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	mustStatus(t, s, st.Version, "attaching")
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, debuggerPath)); backend.index(http.MethodPost, debuggerPath, "method=detachDebugger") >= 0 {
		t.Fatalf("cleanup must not detach while the attach is in flight (%d debugger requests)", n)
	}
	close(gate)
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != runStoppedStatus {
		t.Errorf("a late attach must not resurrect the run: %+v", st)
	}
}

// A wedged debug session must not block the cleanup: StopListener and the
// external deletes go out from the fresh cleanup session.
func TestDebugStop_HangingDebugSessionStillStopsListenerAndBreakpoints(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)
	backend.set(func(f *fakeDebugBackend) { f.hangCookie = debugCookie })

	start := time.Now()
	callTool(t, s, "debug_stop", map[string]interface{}{})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("debug_stop took %v with a wedged debug session", elapsed)
	}
	stop := backend.requests(http.MethodDelete, listenersPath)
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(stop) != 1 || len(del) != 1 || stop[0].cookie == debugCookie || del[0].cookie == debugCookie {
		t.Errorf("listener stop and delete must come from the cleanup session: %+v / %+v", stop, del)
	}
}

// The whole cleanup stays inside its budget even when every cleanup call hangs.
func TestDebugStop_RespectsTheCleanupBudget(t *testing.T) {
	timings := fastDebugTimings
	timings.CleanupBudget = 500 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	backend.set(func(f *fakeDebugBackend) {
		f.hang = map[string]bool{http.MethodDelete + " " + listenersPath: true}
	})
	start := time.Now()
	res := callTool(t, s, "debug_stop", map[string]interface{}{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("cleanup exceeded its budget: %v", elapsed)
	}
	if res.IsError || !strings.Contains(debugResultText(res), `"listener_error"`) {
		t.Errorf("the hanging listener stop must be reported in listener_error: %s", debugResultText(res))
	}
	backend.set(func(f *fakeDebugBackend) { f.hang = nil })
}

// Two concurrent debug_run calls must not interleave: the run-start lock
// serialises them, so the second sets its breakpoints only after the first run
// is cleaned up, and the surviving run holds only the second run's state. The
// first call is held inside its breakpoint request while the second starts.
func TestDebugRun_ConcurrentCallsDoNotInterleave(t *testing.T) {
	s, _, backend := newDebugServer(t)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.bpSetGate = gate })

	var wg sync.WaitGroup
	run := func(uri string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res := callTool(t, s, "debug_run", manualRunArgs("", uri)); res.IsError {
				t.Errorf("debug_run %s: %s", uri, debugResultText(res))
			}
		}()
	}
	run(progURI)
	backend.waitForRequest(t, http.MethodPost, breakpointsPath, "")
	run(otherURI)
	// Without serialisation the second call reaches its breakpoint request
	// while the first is still held; with it, it never does. The wait only
	// bounds how long a broken lock gets to show itself.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) && len(backend.requests(http.MethodPost, breakpointsPath)) < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 1 {
		t.Errorf("the second debug_run set breakpoints while the first was still starting (%d breakpoint requests)", n)
	}
	close(gate)
	wg.Wait()

	backend.mu.Lock()
	var posts []int
	for i, r := range backend.reqs {
		if r.method == http.MethodPost && r.path == breakpointsPath {
			posts = append(posts, i)
		}
	}
	if len(posts) != 2 {
		backend.mu.Unlock()
		t.Fatalf("want 2 breakpoint requests, got %d", len(posts))
	}
	first, second := backend.reqs[posts[0]], backend.reqs[posts[1]]
	var stopBetween, deleteBetween bool
	for _, r := range backend.reqs[posts[0]:posts[1]] {
		stopBetween = stopBetween || (r.method == http.MethodDelete && r.path == listenersPath)
		deleteBetween = deleteBetween || (r.method == http.MethodDelete && strings.HasPrefix(r.path, breakpointsPath+"/"))
	}
	backend.mu.Unlock()
	if !stopBetween || !deleteBetween {
		t.Error("the first run must be cleaned up before the second sets its breakpoints")
	}
	if !strings.Contains(first.body, "zprog") || strings.Contains(first.body, "zother") ||
		!strings.Contains(second.body, "zother") || strings.Contains(second.body, "zprog") {
		t.Errorf("each breakpoint request must carry only its own run's breakpoint:\n%s\n%s", first.body, second.body)
	}

	st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if len(st.Breakpoints) != 1 || st.Breakpoints[0].ObjectURI != otherURI || st.Breakpoints[0].ID != secondBreakpointID || st.Status != runListeningStatus {
		t.Errorf("the surviving run must be the second one, with only its breakpoint: %+v", st)
	}
}

// An attached run whose debug session is wedged: the detach of step 3 hangs
// in the debug session, yet the listener stop and the external delete still
// go out from the cleanup session and the cleanup ends with its budget.
func TestDebugStop_WedgedAttachedSessionIsBoundedByTheBudget(t *testing.T) {
	timings := fastDebugTimings
	timings.CleanupBudget = time.Second
	s, _, backend := newDebugServerTimed(t, timings, nil)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	mustStatus(t, s, st.Version, runAttachedStatus)
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)
	backend.set(func(f *fakeDebugBackend) { f.hangCookie = debugCookie })

	start := time.Now()
	res := callTool(t, s, "debug_stop", map[string]interface{}{})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("debug_stop took %v with a wedged attached debug session", elapsed)
	}
	if res.IsError || !strings.Contains(debugResultText(res), `"detach_error"`) {
		t.Errorf("the hanging detach must be reported in detach_error: %s", debugResultText(res))
	}
	stop := backend.requests(http.MethodDelete, listenersPath)
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(stop) != 1 || len(del) != 1 || stop[0].cookie == debugCookie || del[0].cookie == debugCookie {
		t.Errorf("listener stop and delete must come from the cleanup session: %+v / %+v", stop, del)
	}
	backend.set(func(f *fakeDebugBackend) { f.hangCookie = "" })
}

// An attach that completes inside the listener-exit wait is detached by the
// listener goroutine before cleanup returns. A following debug_run for the
// same user reuses the debug session, so its breakpoints and listener must not
// race that detach.
func TestDebugRun_LateDetachFinishesBeforeTheNextRunStarts(t *testing.T) {
	timings := fastDebugTimings
	timings.ListenerExitWait = 5 * time.Second
	timings.AttachTimeout = 10 * time.Second
	s, _, backend := newDebugServerTimed(t, timings, nil)
	attachGate, detachGate := make(chan struct{}), make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.attachGate, f.detachGate = attachGate, detachGate })

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	mustStatus(t, s, st.Version, "attaching")
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if res := callTool(t, s, "debug_run", manualRunArgs("", otherURI)); res.IsError {
			t.Errorf("second debug_run: %s", debugResultText(res))
		}
	}()
	backend.waitForRequest(t, http.MethodDelete, listenersPath, "") // cleanup step 2 started
	close(attachGate)
	detach := backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
	if detach.cookie != debugCookie {
		t.Errorf("the late detach must be sent in the debug session (cookie %q, want %q)", detach.cookie, debugCookie)
	}
	// The detach is held by the fake. Cleanup must wait for it, so the second
	// run sets no breakpoints meanwhile; the window only bounds how long a
	// detach outside the listener goroutine gets to show itself.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) && len(backend.requests(http.MethodPost, breakpointsPath)) < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 1 {
		t.Errorf("the next run set breakpoints while the late detach was still in flight (%d breakpoint requests)", n)
	}
	close(detachGate)
	<-done

	posts := backend.requests(http.MethodPost, breakpointsPath)
	if len(posts) != 2 || posts[1].cookie != debugCookie {
		t.Fatalf("the next run must reuse the debug session: %+v", posts)
	}
	second := -1
	backend.mu.Lock()
	for i, r := range backend.reqs {
		if r.method == http.MethodPost && r.path == breakpointsPath {
			second = i
		}
	}
	for _, r := range backend.reqs[second:] {
		if r.method == http.MethodPost && r.path == debuggerPath && strings.Contains(r.query, "method=detachDebugger") {
			t.Error("a detach was sent after the next run set its breakpoints")
		}
	}
	backend.mu.Unlock()
}

func stopResult(t *testing.T, s *server.MCPServer) tools.DebugStopResult {
	t.Helper()
	res := callTool(t, s, "debug_stop", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("debug_stop: %s", debugResultText(res))
	}
	var out tools.DebugStopResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDebugStop_ReportsRemovedBreakpoints(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("", progURI, otherURI)))
	out := stopResult(t, s)
	if !out.Stopped || len(out.RemovedBreakpoints) != 2 || len(out.NotRemoved) != 0 || out.ListenerError != "" || out.DetachError != "" {
		t.Errorf("got %+v", out)
	}
}

func TestDebugStop_ReportsWhatItCouldNotRemove(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) {
		f.bpDeleteErr = &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	})
	out := stopResult(t, s)
	if len(out.NotRemoved) != 1 || out.NotRemoved[0].ID != firstBreakpointID || out.NotRemoved[0].Error == "" || len(out.RemovedBreakpoints) != 0 {
		t.Errorf("got %+v", out)
	}
	backend.set(func(f *fakeDebugBackend) { f.bpDeleteErr = nil })
}
