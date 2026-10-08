package tools_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// secondBreakpointID is the ID the fake gives the second breakpoint it sets.
const (
	firstBreakpointID  = "BP1"
	secondBreakpointID = "BP2"
)

func setBreakpointArgs(uri string, line int) map[string]interface{} {
	return map[string]interface{}{"object_uri": uri, "line": line}
}

func TestDebugSetBreakpoint_NeedsARun(t *testing.T) {
	s, _, _ := newDebugServer(t)
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "start a run with debug_run") {
		t.Errorf("got %s", debugResultText(res))
	}
}

func TestDebugSetBreakpoint_DuringAttachIsRefused(t *testing.T) {
	s, _, backend := newDebugServer(t)
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.attachGate = gate })
	defer close(gate)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "DBG1")
	mustStatus(t, s, st.Version, "attaching")
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "attach in progress; call debug_wait") {
		t.Errorf("got %s", debugResultText(res))
	}
}

// While attached, an external request would detach the debugger (SAP_BASIS
// 750): the breakpoint goes to the attached debugger, in the stateful session.
func TestDebugSetBreakpoint_WhileAttachedUsesDebuggerScope(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	st := runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	if len(st.Breakpoints) != 2 || st.Breakpoints[1].Scope != "debugger" || st.Breakpoints[1].ID != secondBreakpointID || st.Breakpoints[0].ID != "BP1" {
		t.Fatalf("breakpoints: %+v", st.Breakpoints)
	}
	posts := backend.requests(http.MethodPost, breakpointsPath)
	last := posts[len(posts)-1]
	if !strings.Contains(last.body, `scope="debugger"`) || !last.stateful || last.cookie != posts[0].cookie {
		t.Errorf("debugger-scope request: %+v", last)
	}
	if n := len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")); n != 0 {
		t.Error("the external breakpoint must stay while attached")
	}
}

// Outside an attachment all external breakpoints are deleted and the full
// list is set again, so the stored IDs stay right on both releases.
func TestDebugSetBreakpoint_OutsideAttachResetsTheExternalList(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	st := runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))

	if len(backend.requests(http.MethodDelete, breakpointsPath+"/BP1")) != 1 {
		t.Error("the old external breakpoint must be deleted first")
	}
	posts := backend.requests(http.MethodPost, breakpointsPath)
	if len(posts) != 2 || !strings.Contains(posts[1].body, "zprog") || !strings.Contains(posts[1].body, "zother") || !strings.Contains(posts[1].body, `scope="external"`) {
		t.Errorf("the full list must be set in one request: %+v", posts)
	}
	if len(st.Breakpoints) != 2 || st.Breakpoints[0].ID != secondBreakpointID || st.Breakpoints[1].ID != "BP3" || st.Breakpoints[1].Scope != "external" {
		t.Errorf("stored IDs: %+v", st.Breakpoints)
	}
}

func TestDebugSetBreakpoint_RestoresThePreviousListWhenTheResetFails(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.set(func(f *fakeDebugBackend) {
		f.bpPostErr = []*fakeError{{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}}
	})
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "previous breakpoints are set again") {
		t.Fatalf("got %s", debugResultText(res))
	}
	st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if len(st.Breakpoints) != 1 || st.Breakpoints[0].ObjectURI != progURI || st.Breakpoints[0].ID != secondBreakpointID {
		t.Errorf("restored list: %+v", st.Breakpoints)
	}
}

func TestDebugSetBreakpoint_NamesBreakpointsNoLongerSet(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	fail := &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	backend.set(func(f *fakeDebugBackend) { f.bpPostErr = []*fakeError{fail, fail} })
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	text := debugResultText(res)
	if !res.IsError || !strings.Contains(text, "no longer set") || !strings.Contains(text, progURI) {
		t.Fatalf("got %s", text)
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 0 {
		t.Errorf("the run must not claim breakpoints that are gone: %+v", st.Breakpoints)
	}
}

// Cleanup step 3: debugger-scope breakpoints are deleted in the debug session
// before the detach.
func TestDebugStop_RemovesDebuggerBreakpointsBeforeTheDetach(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	callTool(t, s, "debug_stop", map[string]interface{}{})

	del := backend.index(http.MethodDelete, breakpointsPath+"/BP2", "scope=debugger")
	detach := backend.index(http.MethodPost, debuggerPath, "method=detachDebugger")
	if del < 0 || detach < 0 || del > detach {
		t.Errorf("want the debugger-scope delete (%d) before the detach (%d)", del, detach)
	}
}

// A debugger-scope request that SAP answers with noSessionAttached means the
// debuggee is gone meanwhile: the call fails and stores nothing.
func TestDebugSetBreakpoint_DebuggerScopeWithoutAttachedDebugger(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.bpPostErr = []*fakeError{{
			status: http.StatusBadRequest, typ: "ExceptionInvalidData",
			props: map[string]string{"com.sap.adt.communicationFramework.subType": "noSessionAttached"},
		}}
	})
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "no debugger is attached any more; call debug_wait") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 1 {
		t.Errorf("only the run's breakpoint may be stored: %+v", st.Breakpoints)
	}
}

// A failed delete stops the reset: the breakpoints deleted before it are no
// longer set and the run stops claiming them; the rest stay as they were.
func TestDebugSetBreakpoint_FailedDeleteDropsTheDeletedOnes(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("", progURI, otherURI)))
	backend.set(func(f *fakeDebugBackend) {
		f.bpDeleteErr = &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
		f.bpDeleteID = secondBreakpointID
	})
	res := callTool(t, s, "debug_set_breakpoint", setBreakpointArgs("/sap/bc/adt/programs/programs/zthird/source/main", 7))
	text := debugResultText(res)
	if !res.IsError || !strings.Contains(text, secondBreakpointID) || !strings.Contains(text, "no longer set: "+progURI+" line 3") {
		t.Fatalf("got %s", text)
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 1 {
		t.Errorf("no breakpoint may be set after the failed delete (%d requests)", n)
	}
	st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if len(st.Breakpoints) != 1 || st.Breakpoints[0].ID != secondBreakpointID || st.Breakpoints[0].ObjectURI != otherURI {
		t.Errorf("stored breakpoints: %+v", st.Breakpoints)
	}
}

func TestDebugRemoveBreakpoint_External(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP1"})
	if res.IsError || !strings.Contains(debugResultText(res), `"removed":true`) {
		t.Fatalf("got %s", debugResultText(res))
	}
	del := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(del) != 1 || !strings.Contains(del[0].query, "scope=external") {
		t.Errorf("delete request: %+v", del)
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 0 {
		t.Errorf("the run must forget the breakpoint: %+v", st.Breakpoints)
	}
}

func TestDebugRemoveBreakpoint_DebuggerScopeUsesItsScope(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": secondBreakpointID}); res.IsError {
		t.Fatal(debugResultText(res))
	}
	del := backend.requests(http.MethodDelete, breakpointsPath+"/"+secondBreakpointID)
	if len(del) != 1 || !strings.Contains(del[0].query, "scope=debugger") || !del[0].stateful {
		t.Errorf("delete request: %+v", del)
	}
}

func TestDebugRemoveBreakpoint_Refusals(t *testing.T) {
	s, _, _ := newDebugServer(t)
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP1"}); !res.IsError || !strings.Contains(debugResultText(res), "debug_run") {
		t.Errorf("without a run: %s", debugResultText(res))
	}
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	if res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": "BP9"}); !res.IsError || !strings.Contains(debugResultText(res), "not part of the current run") {
		t.Errorf("unknown ID: %s", debugResultText(res))
	}
}

// A client that cancels debug_set_breakpoint after the old breakpoints were
// deleted and the new list failed must not lose them: the restore runs on a
// context that outlives the request.
func TestDebugSetBreakpoint_CancelledRequestStillRestoresThePreviousList(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend.set(func(f *fakeDebugBackend) {
		f.bpPostErr = []*fakeError{{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}}
		f.bpSetDone = cancel
	})
	res := callToolCtx(ctx, t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7))
	if !res.IsError || !strings.Contains(debugResultText(res), "previous breakpoints are set again") {
		t.Fatalf("got %s", debugResultText(res))
	}
	st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{}))
	if len(st.Breakpoints) != 1 || st.Breakpoints[0].ObjectURI != progURI {
		t.Errorf("the run must still hold its breakpoint: %+v", st.Breakpoints)
	}
}

// A debug_stop that wins the run-start lock between the run lookup and the
// DELETE of debug_remove_breakpoint must turn the removal into "no active run",
// not a request against the stopped run's session.
func TestDebugRemoveBreakpoint_RunStoppedWhileWaitingForTheLock(t *testing.T) {
	s, _, backend := newDebugServer(t)
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	gate := make(chan struct{})
	backend.set(func(f *fakeDebugBackend) { f.bpSetGate = gate })

	go callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)) // holds the lock at its gated POST
	deadline := time.Now().Add(5 * time.Second)
	for len(backend.requests(http.MethodPost, breakpointsPath)) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("debug_set_breakpoint never reached its breakpoint request")
		}
		time.Sleep(10 * time.Millisecond)
	}
	go callTool(t, s, "debug_stop", map[string]interface{}{})
	time.Sleep(200 * time.Millisecond) // the stop waits for the lock; the run is still active
	removed := make(chan string, 1)
	go func() {
		res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": firstBreakpointID})
		if !res.IsError {
			removed <- "removal succeeded: " + debugResultText(res)
			return
		}
		removed <- debugResultText(res)
	}()
	time.Sleep(200 * time.Millisecond) // the removal passed its run lookup and waits behind the stop
	close(gate)

	select {
	case text := <-removed:
		if !strings.Contains(text, "no active debug run") {
			t.Errorf("got %s", text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("debug_remove_breakpoint never returned")
	}
}

// SAP serialises requests per stateful session, so while the listener's long
// poll is open every external breakpoint request must go to the cleanup
// session, not the listener's (a request on it hangs until the client timeout).
func TestDebugBreakpoints_WhileListeningUseTheCleanupSession(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.serializeSessions = true })
	runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	listener := backend.waitForRequest(t, http.MethodPost, listenersPath, "").cookie

	st := runState(t, callTool(t, s, "debug_set_breakpoint", setBreakpointArgs(otherURI, 7)))
	if len(st.Breakpoints) != 2 {
		t.Fatalf("breakpoints: %+v", st.Breakpoints)
	}
	posts := backend.requests(http.MethodPost, breakpointsPath)
	if len(posts) != 2 || posts[1].cookie == "" || posts[1].cookie == listener {
		t.Errorf("the external set must not use the listener session %q: %+v", listener, posts)
	}
	dels := backend.requests(http.MethodDelete, breakpointsPath+"/BP1")
	if len(dels) != 1 || dels[0].cookie == listener {
		t.Errorf("the external delete must not use the listener session: %+v", dels)
	}

	res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": st.Breakpoints[1].ID})
	if res.IsError {
		t.Fatal(debugResultText(res))
	}
	dels = backend.requests(http.MethodDelete, breakpointsPath+"/"+st.Breakpoints[1].ID)
	if len(dels) != 1 || dels[0].cookie == listener {
		t.Errorf("the external remove must not use the listener session: %+v", dels)
	}
}

// On SAP_BASIS 750 an external request while attached detaches the debugger,
// so removing an external breakpoint is refused then, without any request.
func TestDebugRemoveBreakpoint_ExternalWhileAttachedIsRefused(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	res := callTool(t, s, "debug_remove_breakpoint", map[string]interface{}{"breakpoint_id": firstBreakpointID})
	if !res.IsError || !strings.Contains(debugResultText(res), "cannot be removed while the debuggee is attached") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if n := len(backend.requests(http.MethodDelete, breakpointsPath+"/"+firstBreakpointID)); n != 0 {
		t.Errorf("no request may be sent, got %d DELETEs", n)
	}
	if st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{})); len(st.Breakpoints) != 1 || st.Status != runAttachedStatus {
		t.Errorf("run state must be unchanged: %+v", st)
	}
}
