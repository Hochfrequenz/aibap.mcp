package tools

import (
	"testing"
	"time"
)

// DebugTimingsForTest lets the external tests shorten the waits of a debugging
// run. A zero field keeps the default.
type DebugTimingsForTest struct {
	TriggerDelay     time.Duration
	InitialWait      time.Duration
	CleanupBudget    time.Duration
	ListenerExitWait time.Duration
	IdleLimit        time.Duration
	AttachTimeout    time.Duration
}

// SetDebugTimingsForTest replaces the default run timings until the test ends.
// Servers registered afterwards copy them; running goroutines are unaffected.
func SetDebugTimingsForTest(t testing.TB, d DebugTimingsForTest) {
	t.Helper()
	debugTimingsMu.Lock()
	old := defaultDebugTimings
	next := old
	set := func(dst *time.Duration, v time.Duration) {
		if v > 0 {
			*dst = v
		}
	}
	set(&next.triggerDelay, d.TriggerDelay)
	set(&next.initialWait, d.InitialWait)
	set(&next.cleanupBudget, d.CleanupBudget)
	set(&next.listenerExitWait, d.ListenerExitWait)
	set(&next.idleLimit, d.IdleLimit)
	set(&next.attachTimeout, d.AttachTimeout)
	defaultDebugTimings = next
	debugTimingsMu.Unlock()
	t.Cleanup(func() {
		debugTimingsMu.Lock()
		defaultDebugTimings = old
		debugTimingsMu.Unlock()
	})
}

// ProcessIDEID returns this process's debugger IDE ID.
func ProcessIDEID() string { return processIDEID }
