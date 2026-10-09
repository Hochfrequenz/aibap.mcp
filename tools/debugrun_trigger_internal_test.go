package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// The unit-test result is shared by every snapshot only if copyState copies
// it; a caller that edits its copy must not change the run.
func TestRunSnapshotCopiesTheUnitTestResult(t *testing.T) {
	r, _ := newStateTestRun(t)
	r.transition(func(st *DebugRunState) {
		st.Trigger = &DebugTriggerState{Kind: triggerUnitTests, State: triggerDone, UnitTests: &adt.TestResult{
			Passed:    1,
			TestCases: []adt.TestCase{{Name: "TEST_HELLO", Passed: true, Messages: []string{"ok"}}},
		}}
	})
	const changed = "CHANGED"
	st, _ := r.snapshot()
	st.Trigger.UnitTests.Passed = 99
	st.Trigger.UnitTests.TestCases[0].Name = changed
	st.Trigger.UnitTests.TestCases[0].Messages[0] = changed

	again, _ := r.snapshot()
	ut := again.Trigger.UnitTests
	if ut.Passed != 1 || ut.TestCases[0].Name != "TEST_HELLO" || ut.TestCases[0].Messages[0] != "ok" {
		t.Errorf("snapshot shares the unit-test result with the run: %+v", ut)
	}
}

// The trigger context is not derived from the run context: cleanup does not
// cancel it, and its deadline is the listener budget plus triggerSlack.
func TestRunTriggerContextOutlivesTheRunAndEndsAfterTheSlack(t *testing.T) {
	timings := currentDebugTimings()
	timings.triggerDelay = time.Millisecond
	timings.triggerSlack = 50 * time.Millisecond
	m := &debugSessions{timings: timings}
	r := m.newRun(runParams{
		kind:    triggerGUI,
		budget:  0,
		trigger: &DebugTriggerState{Kind: triggerGUI, State: triggerPending},
	})
	fallback := &DebugInstructions{Steps: []string{"start it yourself"}, User: "ALICE"}

	var deadline time.Time
	var errAfterRunCancel error
	r.runTrigger(func(ctx context.Context) (*adt.TestResult, error) {
		deadline, _ = ctx.Deadline()
		r.runCancel() // what cleanup does
		errAfterRunCancel = ctx.Err()
		<-ctx.Done()
		return nil, errors.New("SAP did not answer")
	}, fallback, false)

	if errAfterRunCancel != nil {
		t.Errorf("cancelling the run cancelled the trigger: %v", errAfterRunCancel)
	}
	if want := r.budgetEnd.Add(timings.triggerSlack); deadline.Before(want.Add(-10*time.Millisecond)) || deadline.After(want.Add(10*time.Millisecond)) {
		t.Errorf("trigger deadline %v, want about %v", deadline, want)
	}
	st, _ := r.snapshot()
	if st.Trigger.State != triggerFailed || !strings.Contains(st.Trigger.Error, "timed out") {
		t.Errorf("trigger: %+v", st.Trigger)
	}
	if st.Instructions == nil || st.Instructions.Steps[0] != "start it yourself" {
		t.Errorf("a failed trigger hands back its fallback instructions: %+v", st.Instructions)
	}
}
