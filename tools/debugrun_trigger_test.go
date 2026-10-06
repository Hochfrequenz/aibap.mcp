package tools_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// Values this file compares against.
const (
	runAttachedStatus = "attached"
	sysAHost          = "sap-a.test"
)

func unitTestRunArgs(user string) map[string]interface{} {
	args := manualRunArgs(user)
	args["trigger"] = map[string]interface{}{"kind": "unit_tests"}
	return args
}

// waitForState calls debug_wait until pred holds.
func waitForState(t *testing.T, s *server.MCPServer, since int64, pred func(tools.DebugRunState) bool) tools.DebugRunState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"since_version": since, "timeout_seconds": 2}))
		if pred(st) {
			return st
		}
		since = st.Version
	}
	t.Fatal("the run never reached the expected state")
	return tools.DebugRunState{}
}

// unit_tests: the server runs the tests, the run is attached within debug_run.
func TestDebugRun_UnitTestsAttachWithinTheCall(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) { f.unitHit = "DBG1" })

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != runAttachedStatus || st.Trigger == nil || st.Trigger.Kind != "unit_tests" || st.Trigger.State != "running" {
		t.Fatalf("want attached with a running unit-test trigger, got %+v / %+v", st, st.Trigger)
	}
	if st.Instructions != nil {
		t.Errorf("a server-run trigger needs no instructions: %+v", st.Instructions)
	}
	ut := backend.requests(http.MethodPost, unitTestsPath)
	if len(ut) != 1 || !strings.Contains(ut[0].body, "/sap/bc/adt/programs/programs/zprog") || ut[0].host != sysAHost {
		t.Fatalf("unit-test request: %+v", ut)
	}
	if ut[0].cookie == backend.cookieOf(http.MethodPost, breakpointsPath) {
		t.Error("the unit tests must run on their own isolated session, not the debug session")
	}

	// Releasing the debuggee lets the tests finish; their result lands in the run.
	backend.release()
	done := waitForState(t, s, st.Version, func(s tools.DebugRunState) bool { return s.Trigger != nil && s.Trigger.State == "done" })
	if done.Trigger.UnitTests == nil || done.Trigger.UnitTests.Passed != 1 {
		t.Errorf("trigger result: %+v", done.Trigger)
	}
}

// Tests that finish without a hit end the run: the breakpoint line was not executed.
func TestDebugRun_UnitTestsWithoutHitEndNoHit(t *testing.T) {
	s, _, backend := newDebugServer(t)

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Status != "ended" || st.EndReason != "no_hit" || st.Trigger.State != "done" || st.Trigger.UnitTests == nil {
		t.Fatalf("want ended/no_hit with the test result, got %+v / %+v", st, st.Trigger)
	}
	backend.waitForRequest(t, http.MethodDelete, listenersPath, "")
}

func TestDebugRun_UnitTestTriggerFailureFallsBackToInstructions(t *testing.T) {
	s, _, backend := newDebugServer(t)
	backend.set(func(f *fakeDebugBackend) {
		f.unitErr = &fakeError{status: http.StatusInternalServerError, typ: "ExceptionResourceFailure"}
	})

	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
	if st.Trigger == nil || st.Trigger.State != "failed" || st.Trigger.Error == "" {
		t.Fatalf("trigger must be failed with its error: %+v", st.Trigger)
	}
	if st.Status != runListeningStatus || st.Instructions == nil || st.Instructions.User != "ALICE" {
		t.Errorf("a failed trigger keeps listening and hands back instructions: %+v", st)
	}
}

func TestDebugRun_UnitTestUserMustBeTheLogonUser(t *testing.T) {
	s, _, backend := newDebugServer(t)
	res := callTool(t, s, "debug_run", unitTestRunArgs("bob"))
	if !res.IsError || !strings.Contains(debugResultText(res), "ALICE") {
		t.Fatalf("got %s", debugResultText(res))
	}
	if n := len(backend.requests(http.MethodPost, breakpointsPath)); n != 0 {
		t.Errorf("nothing may reach SAP; %d breakpoint requests", n)
	}
}

func TestDebugRun_UnitTestUserIsCaseInsensitive(t *testing.T) {
	s, _, _ := newDebugServer(t)
	if res := callTool(t, s, "debug_run", unitTestRunArgs("Alice")); res.IsError {
		t.Fatalf("the user comparison ignores case: %s", debugResultText(res))
	}
}

func TestDebugRun_UnitTestUserUnknownLogonUserGivesHint(t *testing.T) {
	s, _, _ := newDebugServer(t, tools.WithSystemUser(func(string) string { return "" }))
	st := runState(t, callTool(t, s, "debug_run", unitTestRunArgs("bob")))
	if !strings.Contains(st.Hint, "OAuth2") {
		t.Errorf("an unknown logon user is accepted with a hint, got %q", st.Hint)
	}
}

// fakeTriggerer is a BlackMagicClient that can also start gui runs.
type fakeTriggerer struct {
	backend *fakeDebugBackend
	err     error
	hit     string

	mu              sync.Mutex
	calls           []string
	listenersAtCall int
}

func (f *fakeTriggerer) TriggerDebugRun(ctx context.Context, system, user string, target tools.DebugTarget) error {
	f.mu.Lock()
	f.calls = append(f.calls, system+" "+user+" "+target.Type+" "+target.Name)
	f.listenersAtCall = len(f.backend.requests(http.MethodPost, listenersPath))
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.hit != "" {
		select {
		case f.backend.hits <- f.hit:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f *fakeTriggerer) ReleaseTransportFallback(context.Context, string) error {
	return errors.New("not used")
}

func (f *fakeTriggerer) CreateTransportFallback(context.Context, string, string, string, string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeTriggerer) UpdateCustomizing(context.Context, string, []tools.CustomizingEntry, string) error {
	return errors.New("not used")
}

func (f *fakeTriggerer) CreateObjectFallback(context.Context, string, string, string, string, string) error {
	return errors.New("not used")
}

func guiRunArgs() map[string]interface{} {
	args := manualRunArgs("")
	args["trigger"] = map[string]interface{}{"kind": "gui", "target": map[string]interface{}{"type": "report", "name": "ZPROG"}}
	return args
}

func newTriggererServer(t *testing.T, trig *fakeTriggerer) (*server.MCPServer, *fakeDebugBackend) {
	t.Helper()
	timings := fastDebugTimings
	timings.TriggerDelay = 200 * time.Millisecond
	s, _, backend := newDebugServerTimed(t, timings, trig)
	trig.backend = backend
	return s, backend
}

func TestDebugRun_GUITriggererIsCalledAfterTheListenerStarted(t *testing.T) {
	trig := &fakeTriggerer{hit: "DBG1"}
	s, _ := newTriggererServer(t, trig)

	st := runState(t, callTool(t, s, "debug_run", guiRunArgs()))
	if st.Status != runAttachedStatus {
		// The triggerer may return before the background attach finished.
		st = mustStatus(t, s, st.Version, runAttachedStatus)
	}
	if st.Trigger == nil || st.Trigger.Kind != "gui" {
		t.Fatalf("want attached with a gui trigger, got %+v / %+v", st, st.Trigger)
	}
	if st.Instructions != nil {
		t.Errorf("a triggerer needs no instructions: %+v", st.Instructions)
	}
	trig.mu.Lock()
	defer trig.mu.Unlock()
	if len(trig.calls) != 1 || trig.calls[0] != "sysA ALICE report ZPROG" {
		t.Errorf("triggerer calls: %v", trig.calls)
	}
	if trig.listenersAtCall < 1 {
		t.Error("the triggerer must be called only after the listener has started")
	}
}

func TestDebugRun_GUITriggererErrorBecomesFailedWithInstructions(t *testing.T) {
	trig := &fakeTriggerer{err: errors.New("no GUI session")}
	s, _ := newTriggererServer(t, trig)

	st := runState(t, callTool(t, s, "debug_run", guiRunArgs()))
	if st.Trigger == nil || st.Trigger.State != "failed" || !strings.Contains(st.Trigger.Error, "no GUI session") {
		t.Fatalf("trigger: %+v", st.Trigger)
	}
	if st.Status != runListeningStatus || st.Instructions == nil || !strings.Contains(strings.Join(st.Instructions.Steps, " "), "SE38") {
		t.Errorf("a failed triggerer falls back to the gui instructions: %+v", st)
	}
}
