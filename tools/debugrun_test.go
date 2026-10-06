package tools_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
)

// manual: the server only listens and hands back instructions; a lower-case
// user addresses the same SAP user as the upper-case one.
func TestDebugRun_ManualListensWithInstructions(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("alice")))
	if st.Status != "listening" || st.Version < 1 {
		t.Fatalf("state: %+v", st)
	}
	if len(st.Breakpoints) != 1 || st.Breakpoints[0] != (tools.DebugRunBreakpoint{ObjectURI: progURI, Line: 3, ID: "BP1", Scope: "external"}) {
		t.Errorf("breakpoints: %+v", st.Breakpoints)
	}
	if st.Trigger != nil {
		t.Errorf("a manual run has no server trigger: %+v", st.Trigger)
	}
	if st.Instructions == nil || st.Instructions.User != "ALICE" || !strings.Contains(st.Instructions.Steps[0], "ALICE") ||
		!strings.Contains(st.Instructions.Steps[0], st.ListeningUntil) {
		t.Errorf("instructions must name user and deadline: %+v (until %s)", st.Instructions, st.ListeningUntil)
	}

	bp := backend.requests(http.MethodPost, breakpointsPath)[0]
	for _, want := range []string{`requestUser="ALICE"`, `scope="external"`, `adtcore:type="PROG/P"`, `adtcore:name="ZPROG"`, `ideId="` + tools.ProcessIDEID() + `"`} {
		if !strings.Contains(bp.body, want) {
			t.Errorf("breakpoint request lacks %s: %s", want, bp.body)
		}
	}
	listen := backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if !regexp.MustCompile(`ideId=[0-9A-F]{32}`).MatchString(listen.query) || !strings.Contains(listen.query, "timeout=60") {
		t.Errorf("listener query: %s", listen.query)
	}
}

func TestDebugRun_ManualHitIsAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	att := mustStatus(t, s, st.Version, runAttachedStatus)

	if att.DebuggeeID != testDebuggeeID {
		t.Errorf("debuggee_id = %q", att.DebuggeeID)
	}
	if att.Position == nil || att.Position.Program != "ZPROG" || att.Position.Line != 3 || att.Position.SourceURI != progURI ||
		att.Position.SourceLine != 3 || !strings.Contains(att.Position.SourceExcerpt, "> 3: line 3") {
		t.Errorf("position: %+v", att.Position)
	}
	a := backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=attach&debuggeeId=DBG1")
	if !a.stateful {
		t.Error("attach must be stateful")
	}
}

// Attaching at once closes the window in which a caught debuggee waits for an
// attach that a serialising client might never send.
func TestDebugRun_HitWithoutWaiterIsAttachedInBackground(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=getStack")
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
		if st.Status == runAttachedStatus {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no background attach; state %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDebugWait_ConcurrentWaitersBothSeeAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	var wg sync.WaitGroup
	got := make([]tools.DebugRunState, 2)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _ = waitForStatus(t, s, st.Version, runAttachedStatus)
		}(i)
	}
	backend.hit(t, testDebuggeeID)
	wg.Wait()
	for i, g := range got {
		if g.Status != runAttachedStatus {
			t.Errorf("waiter %d saw %+v", i, g)
		}
	}
}

func TestDebugWait_SinceVersionWaitsForAChange(t *testing.T) {
	s, _, _ := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	start := time.Now()
	now := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if now.Version != st.Version || time.Since(start) > time.Second {
		t.Errorf("without since_version debug_wait returns at once: %+v", now)
	}
	start = time.Now()
	same := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 1}))
	if same.Version != st.Version || time.Since(start) < time.Second {
		t.Errorf("with since_version and no change, debug_wait waits out its timeout: %+v after %v", same, time.Since(start))
	}
	old := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version - 1}))
	if old.Version != st.Version {
		t.Errorf("an older since_version returns at once: %+v", old)
	}
}

func TestDebugWait_RejectsNonIntegerSinceVersion(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	for _, v := range []interface{}{"7", 1.5} {
		res := callTool(t, s, "debug_wait", map[string]interface{}{"since_version": v})
		if !res.IsError || !strings.Contains(debugResultText(res), "since_version") {
			t.Errorf("since_version %v: got %s", v, debugResultText(res))
		}
	}
	res := callTool(t, s, "debug_wait", map[string]interface{}{"timeout_seconds": 0})
	if !res.IsError || !strings.Contains(debugResultText(res), "timeout_seconds") {
		t.Errorf("timeout_seconds 0: got %s", debugResultText(res))
	}
}

func TestDebugWait_CancelledCallDoesNotStopTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	callToolCtx(ctx, t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 30})
	if time.Since(start) > 5*time.Second {
		t.Fatal("a cancelled debug_wait must return")
	}
	backend.hit(t, testDebuggeeID)
	mustStatus(t, s, st.Version, runAttachedStatus)
}

func TestDebugWait_WithoutRunIsAnError(t *testing.T) {
	s, _, _ := newDebugServer(t)
	res := callTool(t, s, "debug_wait", map[string]interface{}{})
	if !res.IsError || !strings.Contains(debugResultText(res), "debug_run") {
		t.Errorf("got %s", debugResultText(res))
	}
}

func TestDebugWait_AfterStopReportsStopped(t *testing.T) {
	s, _, _ := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != runStoppedStatus {
		t.Errorf("status = %q, want stopped", st.Status)
	}
}

func TestDebugRun_RejectedBreakpointRemovesTheOthers(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.rejectBP = map[string][2]string{"zother": {"invalidPosition", "no executable statement"}}
	})

	res := callTool(t, s, "debug_run", manualRunArgs("", progURI, otherURI))
	text := debugResultText(res)
	if !res.IsError || !strings.Contains(text, "zother") || !strings.Contains(text, "invalidPosition") {
		t.Fatalf("debug_run must fail naming the rejected breakpoint, got %s", text)
	}
	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("the breakpoint that was set must be deleted again")
	}
	if n := len(backend.requests(http.MethodPost, listenersPath)); n != 0 {
		t.Errorf("no listener may start; %d listener requests", n)
	}
}

func TestDebugRun_ExistingBreakpointCountsAsSet(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.existingBP = map[string]bool{"zprog": true} })

	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if st.Status != "listening" || len(st.Breakpoints) != 1 || st.Breakpoints[0].ID != "BP1" {
		t.Errorf("errorKind existing must count as set: %+v", st)
	}
}

func TestDebugRun_GUIInstructionsFollowOKCodeAvailability(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		want, notWant string
	}{
		{"program exists", http.StatusOK, "/H_REACTIVATE_EXTD_DBG", "SADT_START_TCODE"},
		{"program missing", http.StatusNotFound, "SADT_START_TCODE", "/H_REACTIVATE_EXTD_DBG"},
		{"lookup fails", http.StatusInternalServerError, "SADT_START_TCODE", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, backend := newDebugServer(t)
			backend.set(func(f *fakeDebugBackend) { f.okCodeStatus = tc.status })
			args := manualRunArgs("")
			args["trigger"] = map[string]interface{}{"kind": "gui", "target": map[string]interface{}{"type": "report", "name": "ZPROG"}}

			st := runState(t, callTool(t, s, "debug_run", args))
			if st.Instructions == nil {
				t.Fatalf("gui without triggerer must return instructions: %+v", st)
			}
			step := st.Instructions.Steps[0]
			if !strings.Contains(step, tc.want) || (tc.notWant != "" && strings.Contains(step, tc.notWant)) {
				t.Errorf("enable step: %q", step)
			}
			if tc.status == http.StatusInternalServerError && strings.Index(step, "/H_REACTIVATE") > strings.Index(step, "SADT_START_TCODE") {
				t.Errorf("a failed lookup gives both, OK code first: %q", step)
			}
			if len(backend.requests(http.MethodGet, okCodeProgPath)) == 0 {
				t.Error("the OK-code program must be looked up through the ADT repository")
			}
		})
	}
}

func TestDebugRun_NoDefaultUserIsRejected(t *testing.T) {
	s, _, backend := newDebugServer(t, tools.WithSystemUser(func(string) string { return "" }))
	res := callTool(t, s, "debug_run", manualRunArgs(""))
	if !res.IsError || !strings.Contains(debugResultText(res), "no configured logon user") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 0 {
		t.Errorf("nothing may reach SAP; %d breakpoint requests", n)
	}
}

func TestDebugRun_ListenerTimeout(t *testing.T) {
	s, _, backend := newDebugServer(t)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "")
	got := mustStatus(t, s, st.Version, "timeout")
	if !strings.Contains(got.Hint, "system program") {
		t.Errorf("timeout hint: %q", got.Hint)
	}
}

// A listener conflict is not observed live yet; whatever SAP answers must
// surface as a tool error rather than a silent timeout.
func TestDebugRun_ListenerFailureIsReported(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.listenerErr = &fakeError{status: http.StatusConflict, typ: "ExceptionResourceAlreadyExists"}
	})
	res := callTool(t, s, "debug_run", manualRunArgs(""))
	if !res.IsError {
		st := runState(t, res)
		res = callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 5})
	}
	if !res.IsError || !strings.Contains(debugResultText(res), "listener failed") {
		t.Errorf("listener failure must be reported, got %s", debugResultText(res))
	}
}

// Cleanup sends StopListener and the external deletes from a fresh session:
// those endpoints are keyed by user and IDE ID, not by cookie.
func TestDebugStop_StopsListenerAndRemovesBreakpointsFromAFreshSession(t *testing.T) {
	s, _, backend := newDebugServer(t)

	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "requestUser=ALICE")
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	debugCookie := backend.cookieOf(http.MethodPost, breakpointsPath)
	stop := backend.requests(http.MethodDelete, listenersPath)
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(stop) != 1 || len(del) != 1 {
		t.Fatalf("want one listener stop and one breakpoint delete; got %+v / %+v", stop, del)
	}
	if !strings.Contains(stop[0].query, "requestUser=ALICE") || !strings.Contains(stop[0].query, "ideId="+tools.ProcessIDEID()) {
		t.Errorf("listener stop query: %s", stop[0].query)
	}
	if stop[0].cookie == debugCookie || del[0].cookie == debugCookie || stop[0].cookie != del[0].cookie {
		t.Errorf("cleanup must use one fresh session (debug %q, stop %q, delete %q)", debugCookie, stop[0].cookie, del[0].cookie)
	}
}

// A listener failure is reported until the run is cleaned up: after debug_stop
// the run is stopped, and debug_wait reports that rather than the old failure.
func TestDebugWait_AfterListenerFailureAndStopReportsStopped(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.listenerErr = &fakeError{status: http.StatusConflict, typ: "ExceptionResourceAlreadyExists"}
	})
	if res := callTool(t, s, "debug_run", manualRunArgs("")); !res.IsError {
		st := runState(t, res)
		res = callTool(t, s, "debug_wait", map[string]interface{}{"since_version": st.Version, "timeout_seconds": 5})
		if !res.IsError {
			t.Fatalf("the listener failure must be reported first, got %s", debugResultText(res))
		}
	}
	if res := callTool(t, s, "debug_stop", map[string]interface{}{}); res.IsError {
		t.Fatalf("debug_stop: %s", debugResultText(res))
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); st.Status != runStoppedStatus {
		t.Errorf("status = %q, want stopped", st.Status)
	}
}
