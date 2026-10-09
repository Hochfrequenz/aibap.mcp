package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func newStateTestRun(t *testing.T) (*debugRun, *debugSessions) {
	t.Helper()
	m := &debugSessions{timings: currentDebugTimings()}
	r := m.newRun(runParams{
		kind:   triggerManual,
		budget: time.Minute,
		breakpoints: []DebugRunBreakpoint{{
			ObjectURI: "/sap/bc/adt/programs/programs/zprog/source/main", Line: 3, ID: "BP1", Scope: "external",
		}},
	})
	return r, m
}

func TestRunStartsListening(t *testing.T) {
	r, _ := newStateTestRun(t)
	st, err := r.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != runListening || st.Version != 1 || len(st.Breakpoints) != 1 {
		t.Fatalf("initial state: %+v", st)
	}
	until, err := time.Parse(time.RFC3339, st.ListeningUntil)
	if err != nil || time.Until(until) < 50*time.Second || time.Until(until) > 61*time.Second {
		t.Errorf("listening_until = %q (%v), want about one minute ahead", st.ListeningUntil, err)
	}
}

// Every waiter must see every transition: nothing is consumed (#558).
func TestRunTransitionWakesEveryWaiter(t *testing.T) {
	r, _ := newStateTestRun(t)
	v0 := r.st.Version
	got := make(chan DebugRunState, 2)
	for i := 0; i < 2; i++ {
		go func() {
			st, _ := r.waitUntil(context.Background(), 5*time.Second, func(s DebugRunState) bool { return s.Version > v0 })
			got <- st
		}()
	}
	r.transition(func(st *DebugRunState) { st.Status = runAttached })
	for i := 0; i < 2; i++ {
		select {
		case st := <-got:
			if st.Status != runAttached || st.Version != v0+1 {
				t.Errorf("waiter %d saw %+v", i, st)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a waiter was not woken")
		}
	}
}

func TestRunWaitUntilTimesOutWithCurrentState(t *testing.T) {
	r, _ := newStateTestRun(t)
	v0 := r.st.Version
	start := time.Now()
	st, _ := r.waitUntil(context.Background(), 50*time.Millisecond, func(s DebugRunState) bool { return s.Version > v0 })
	if st.Version != v0 || st.Status != runListening {
		t.Errorf("timeout must return the unchanged state, got %+v", st)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Error("waitUntil returned before its timeout")
	}
}

func TestRunWaitUntilReturnsOnCancelledContext(t *testing.T) {
	r, _ := newStateTestRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st, _ := r.waitUntil(ctx, time.Hour, func(DebugRunState) bool { return false })
	if st.Status != runListening {
		t.Errorf("got %+v", st)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.Status != runListening {
		t.Error("a cancelled wait must not change the run")
	}
}

// One counter per process: a since_version from an earlier run never blocks.
func TestRunVersionsIncreaseAcrossRuns(t *testing.T) {
	r1, m := newStateTestRun(t)
	r1.transition(func(st *DebugRunState) { st.Status = runStopped })
	last := r1.st.Version
	r2 := m.newRun(runParams{kind: triggerManual, budget: time.Minute})
	if r2.st.Version <= last {
		t.Errorf("second run starts at version %d, first ended at %d", r2.st.Version, last)
	}
}

func TestRunSnapshotIsACopy(t *testing.T) {
	r, _ := newStateTestRun(t)
	st, _ := r.snapshot()
	st.Breakpoints[0].ID = "CHANGED"
	again, _ := r.snapshot()
	if again.Breakpoints[0].ID != "BP1" {
		t.Error("snapshot shares the breakpoint slice with the run")
	}
}

func TestDebugRunStateIsAnObjectWithBreakpointArray(t *testing.T) {
	res, err := mcp.NewToolResultJSON(copyState(DebugRunState{Status: runListening}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "{") || !strings.Contains(string(out), `"breakpoints":[]`) {
		t.Errorf("structuredContent must be an object with an empty breakpoints array: %s", out)
	}
}

func TestSourceExcerpt(t *testing.T) {
	text := "a\r\nb\r\nc\r\nd\r\ne"
	if got, want := sourceExcerpt(text, 2, 1), "  1: a\n> 2: b\n  3: c"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := sourceExcerpt(text, 1, 3); !strings.HasPrefix(got, "> 1: a") || !strings.HasSuffix(got, "  4: d") {
		t.Errorf("clipped at the start: %q", got)
	}
	if got := sourceExcerpt(text, 9, 1); got != "" {
		t.Errorf("a line past the end gives nothing: %q", got)
	}
}
