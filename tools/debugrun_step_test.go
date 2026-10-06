package tools_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// attachRun starts a manual run, catches DBG1 and waits until it is attached.
func attachRun(t *testing.T, s *server.MCPServer, backend *fakeDebugBackend) tools.DebugRunState {
	t.Helper()
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, testDebuggeeID)
	return mustStatus(t, s, st.Version, runAttachedStatus)
}

func stepState(t *testing.T, res *mcp.CallToolResult) tools.DebugStepResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("debug_step failed: %s", debugResultText(res))
	}
	var out tools.DebugStepResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func currentState(t *testing.T, s *server.MCPServer) tools.DebugRunState {
	t.Helper()
	return runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
}

func TestDebugInAttemptTools_NeedAnAttachedDebuggee(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.waitForRequest(t, http.MethodPost, listenersPath, "")
	before := backend.count()
	for tool, args := range map[string]map[string]interface{}{
		"debug_get_stack":      {},
		"debug_get_variable":   {"variable_name": "LV_X"},
		"debug_set_watchpoint": {"variable_name": "LV_X"},
		"debug_step":           {"action": "stepOver"},
	} {
		res := callTool(t, s, tool, args)
		if !res.IsError || !strings.Contains(debugResultText(res), "no debuggee attached; call debug_wait") {
			t.Errorf("%s while listening: %s", tool, debugResultText(res))
		}
	}
	if after := backend.count(); after != before {
		t.Errorf("refused calls must not reach SAP; %d new requests", after-before)
	}
}

func TestDebugStep_StaysAttachedWithNewPosition(t *testing.T) {
	s, _, backend := newDebugServer(t)
	att := attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) { f.stackXML = strings.ReplaceAll(defaultStackXML, "3", "5") })

	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepOver"}))
	st := currentState(t, s)
	if st.Status != runAttachedStatus || st.Version <= att.Version || st.Position == nil || st.Position.Line != 5 {
		t.Errorf("after a step the run stays attached with the new position: %+v / %+v", st, st.Position)
	}
}

func TestDebugStep_DetachEndsTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	if st := currentState(t, s); st.Status != runEndedStatus || st.EndReason != "detached" || st.Position != nil {
		t.Errorf("got %+v", st)
	}
}

func TestDebugStep_TerminateEndsTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "terminateDebuggee"}))
	if st := currentState(t, s); st.Status != runEndedStatus || st.EndReason != "terminated" {
		t.Errorf("got %+v", st)
	}
}

// SAP_BASIS 750 signals the end of a run with AdiFailed / CX_TPDAPI_DEBUGGEE_ENDED.
func TestDebugStep_DebuggeeEndedSignalCompletesTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {
			status: http.StatusInternalServerError, typ: "AdiFailed", endsRun: true,
			props: map[string]string{"previous1ExceptionClassName": "CX_TPDAPI_DEBUGGEE_ENDED"},
		}}
	})
	if out := stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"})); !out.DebuggeeEnded {
		t.Errorf("step result: %+v", out)
	}
	if st := currentState(t, s); st.Status != runEndedStatus || st.EndReason != "completed" {
		t.Errorf("got %+v", st)
	}
}

// SAP_BASIS 816 answers 400 ExceptionInvalidData at the end of a run (#513).
func TestDebugStep_InvalidDataWithoutDebuggeeCompletesTheRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {status: http.StatusBadRequest, typ: "ExceptionInvalidData", endsRun: true}}
		f.sessionsBody = ""
	})
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"}))
	if st := currentState(t, s); st.Status != runEndedStatus || st.EndReason != "completed" || !strings.Contains(st.Hint, "816") {
		t.Errorf("got %+v", st)
	}
}

// A lost attachment also answers 400: with a debuggee still listed the error
// is reported and the run stays attached.
func TestDebugStep_InvalidDataWithDebuggeeListedStaysAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepContinue": {status: http.StatusBadRequest, typ: "ExceptionInvalidData"}}
		f.sessionsBody = `<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><DEBUGGEE_ID>DBG1</DEBUGGEE_ID></DATA></asx:values></asx:abap>`
	})
	res := callTool(t, s, "debug_step", map[string]interface{}{"action": "stepContinue"})
	if !res.IsError || !strings.Contains(debugResultText(res), "#513") {
		t.Errorf("got %s", debugResultText(res))
	}
	if st := currentState(t, s); st.Status != runAttachedStatus {
		t.Errorf("got %+v", st)
	}
}

func TestDebugStep_OtherFailureStaysAttached(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.stepErr = map[string]fakeError{"stepOver": {status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}}
	})
	res := callTool(t, s, "debug_step", map[string]interface{}{"action": "stepOver"})
	if !res.IsError || !strings.Contains(debugResultText(res), "detachDebugger") {
		t.Errorf("a failed step must point to detachDebugger: %s", debugResultText(res))
	}
	if st := currentState(t, s); st.Status != runAttachedStatus {
		t.Errorf("got %+v", st)
	}
}

// unit_tests end to end: attached within debug_run, then ended with the trigger result.
func TestDebugRun_UnitTestsEndWithTheTriggerResult(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.unitHit = testDebuggeeID })

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != runAttachedStatus {
		t.Fatalf("got %+v", st)
	}
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	end := waitForState(t, s, st.Version, func(s tools.DebugRunState) bool { return s.Trigger.State == triggerDoneState })
	if end.Status != runEndedStatus || end.EndReason != "detached" || end.Trigger.UnitTests == nil || end.Trigger.UnitTests.Passed != 1 {
		t.Errorf("got %+v / %+v", end, end.Trigger)
	}
}

func TestDebugRun_IdleDebuggeeIsDetached(t *testing.T) {
	timings := fastDebugTimings
	timings.IdleLimit = 300 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	att := attachRun(t, s, backend)
	st := mustStatus(t, s, att.Version, runEndedStatus)
	if st.EndReason != "idle_detached" {
		t.Errorf("got %+v", st)
	}
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
}

// The idle timer pauses while an in-attempt call is in flight.
func TestDebugRun_IdleTimerPausesDuringACall(t *testing.T) {
	timings := fastDebugTimings
	timings.IdleLimit = 500 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, nil)

	attachRun(t, s, backend)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.stackGate = gate })
	done := make(chan struct{})
	go func() {
		defer close(done)
		callTool(t, s, "debug_get_stack", map[string]interface{}{})
	}()
	time.Sleep(1500 * time.Millisecond)
	if backend.index(http.MethodPost, debuggerPath, "method=detachDebugger") >= 0 {
		t.Fatal("the idle limit must not detach during an in-flight call")
	}
	close(gate)
	<-done
	backend.waitForRequest(t, http.MethodPost, debuggerPath, "method=detachDebugger")
}
