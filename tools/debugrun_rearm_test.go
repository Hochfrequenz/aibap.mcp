package tools_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// A window that ended early leaves budget: rearm listens again within it.
func TestDebugWait_RearmListensAgainAfterTimeout(t *testing.T) {
	s, _, backend := newDebugServer(t)
	st := runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
	backend.hit(t, "")
	to := mustStatus(t, s, st.Version, "timeout")

	re := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true}))
	if re.Status != runListeningStatus || re.Version <= to.Version || re.Hint != "" {
		t.Fatalf("after rearm: %+v", re)
	}
	backend.hit(t, "DBG1")
	mustStatus(t, s, re.Version, "attached")
	if n := len(backend.requests(http.MethodPost, listenersPath)); n != 2 {
		t.Errorf("want 2 listening windows, got %d", n)
	}
}

func TestDebugWait_RearmAfterADetachedRun(t *testing.T) {
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	stepState(t, callTool(t, s, "debug_step", map[string]interface{}{"action": "detachDebugger"}))
	re := runState(t, callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true}))
	if re.Status != runListeningStatus || re.DebuggeeID != "" || re.EndReason != "" {
		t.Errorf("after rearm: %+v", re)
	}
}

func TestDebugWait_RearmRefused(t *testing.T) {
	t.Run("unit tests", func(t *testing.T) {
		s, _, _ := newDebugServer(t)
		runState(t, callTool(t, s, "debug_run", unitTestRunArgs("")))
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "new debug_run") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
	t.Run("while listening", func(t *testing.T) {
		s, _, _ := newDebugServer(t)
		runState(t, callTool(t, s, "debug_run", manualRunArgs("")))
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "timeout or ended") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
	t.Run("budget used up", func(t *testing.T) {
		s, _, backend := newDebugServer(t)
		args := manualRunArgs("")
		args["timeout_seconds"] = 1
		st := runState(t, callTool(t, s, "debug_run", args))
		backend.hit(t, "")
		mustStatus(t, s, st.Version, "timeout")
		time.Sleep(1100 * time.Millisecond)
		res := callTool(t, s, "debug_wait", map[string]interface{}{"rearm": true})
		if !res.IsError || !strings.Contains(debugResultText(res), "budget") {
			t.Errorf("got %s", debugResultText(res))
		}
	})
}
