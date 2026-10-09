package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestManualInstructions(t *testing.T) {
	until := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	in := manualInstructions("ALICE", until)
	if in.User != "ALICE" || len(in.Steps) != 1 || len(in.Notes) != 3 {
		t.Fatalf("got %+v", in)
	}
	if !strings.Contains(in.Steps[0], "ALICE") || !strings.Contains(in.Steps[0], "2026-10-06T12:00:00Z") {
		t.Errorf("step must name user and deadline: %q", in.Steps[0])
	}
	if !strings.Contains(strings.Join(in.Notes, " "), "system programs") {
		t.Errorf("notes: %v", in.Notes)
	}
}

func TestGUIInstructionsFollowOKCodeAvailability(t *testing.T) {
	report := DebugTarget{Type: "report", Name: "zprog", Inputs: map[string]string{"P_B": "2", "P_A": "1"}}
	ok := guiInstructions("ALICE", report, okCodeAvailable)
	if !strings.Contains(ok.Steps[0], "/H_REACTIVATE_EXTD_DBG KIND=USER USER=ALICE") || strings.Contains(ok.Steps[0], "SADT_START_TCODE") {
		t.Errorf("available: %q", ok.Steps[0])
	}
	missing := guiInstructions("ALICE", report, okCodeMissing)
	if !strings.Contains(missing.Steps[0], "SADT_START_TCODE") || !strings.Contains(missing.Steps[0], "D_AIE_TCODE") || strings.Contains(missing.Steps[0], "/H_REACTIVATE") {
		t.Errorf("missing: %q", missing.Steps[0])
	}
	unknown := guiInstructions("ALICE", report, okCodeUnknown)
	if i, j := strings.Index(unknown.Steps[0], "/H_REACTIVATE"), strings.Index(unknown.Steps[0], "SADT_START_TCODE"); i < 0 || j < 0 || i > j {
		t.Errorf("unknown must give both, OK code first: %q", unknown.Steps[0])
	}
	if !strings.Contains(ok.Steps[1], "SE38") || !strings.Contains(ok.Steps[1], "ZPROG") || !strings.Contains(ok.Steps[1], "P_A = 1, P_B = 2") {
		t.Errorf("report step (inputs sorted): %q", ok.Steps[1])
	}
	if last := ok.Steps[len(ok.Steps)-1]; !strings.Contains(last, "detachDebugger") || !strings.Contains(last, "never stepContinue") {
		t.Errorf("last step must end with detachDebugger: %q", last)
	}
	if !strings.Contains(ok.Steps[1], "/nSE38") {
		t.Errorf("after the OK code the person opens the transaction: %q", ok.Steps[1])
	}
	if !strings.Contains(strings.Join(ok.Notes, " "), "stays busy") {
		t.Errorf("notes must say the window stays busy while attached: %v", ok.Notes)
	}
	for _, s := range []string{"/nSADT_START_TCODE", "D_IDE_USER", "ALICE", "required"} {
		if !strings.Contains(missing.Steps[0], s) {
			t.Errorf("missing: step 0 lacks %q: %q", s, missing.Steps[0])
		}
	}
	if strings.Contains(missing.Steps[0], "D_REQUEST_USER =") {
		t.Errorf("the request user is not required (live: transaction + IDE user suffice): %q", missing.Steps[0])
	}
	if strings.Contains(missing.Steps[1], "/n") || !strings.Contains(missing.Steps[1], "SADT_START_TCODE opened") {
		t.Errorf("SADT_START_TCODE opens the transaction itself; no /n: %q", missing.Steps[1])
	}
}

func TestGUIInstructionsPerTarget(t *testing.T) {
	for _, typ := range []string{"report", "transaction", "function_module", "class_method"} {
		name := "ZT"
		if typ == "class_method" {
			name = "zcl_x=>run"
		}
		in := guiInstructions("ALICE", DebugTarget{Type: typ, Name: name}, okCodeAvailable)
		notes := strings.Join(in.Notes, " ")
		if strings.Contains(notes, "Untested") {
			t.Errorf("%s: every target is tested live; no untested note: %v", typ, in.Notes)
		}
		wantUpper := typ == "function_module" || typ == "class_method"
		if got := strings.Contains(notes, "upper case"); got != wantUpper {
			t.Errorf("%s: upper case note = %v, want %v: %v", typ, got, wantUpper, in.Notes)
		}
	}
	cm := guiInstructions("ALICE", DebugTarget{Type: "class_method", Name: "zcl_x=>run"}, okCodeMissing)
	if !strings.Contains(cm.Steps[0], "D_AIE_TCODE") || !strings.Contains(cm.Steps[0], "SE24") ||
		!strings.Contains(cm.Steps[1], "ZCL_X") || !strings.Contains(cm.Steps[1], "Execute Method") {
		t.Errorf("class method: %v", cm.Steps)
	}
	tx := guiInstructions("ALICE", DebugTarget{Type: "transaction", Name: "zt01"}, okCodeMissing)
	if !strings.Contains(tx.Steps[0], "ZT01") || strings.Contains(tx.Steps[1], "/nZT01") {
		t.Errorf("transaction: %v", tx.Steps)
	}
	un := guiInstructions("ALICE", DebugTarget{Type: "transaction", Name: "zt01"}, okCodeUnknown)
	if !strings.Contains(un.Steps[1], "/nZT01") || !strings.Contains(un.Steps[1], "SADT_START_TCODE opened") {
		t.Errorf("unknown: the step must cover both paths: %q", un.Steps[1])
	}
}

func TestOKCodeCache(t *testing.T) {
	var c okCodeCache
	calls := 0
	found := func(context.Context, string) error { calls++; return nil }
	if got := c.lookup(context.Background(), "sysA", found); got != okCodeAvailable {
		t.Errorf("got %v", got)
	}
	c.lookup(context.Background(), "sysA", found)
	if calls != 1 {
		t.Errorf("a definite answer must be cached per system; %d calls", calls)
	}
	notFound := func(_ context.Context, uri string) error {
		if uri != okCodeProgramURI {
			t.Errorf("looked up %q", uri)
		}
		return &adt.ADTError{StatusCode: 404, Message: "not found"}
	}
	if got := c.lookup(context.Background(), "sysB", notFound); got != okCodeMissing {
		t.Errorf("404 must mean missing, got %v", got)
	}
	failing := 0
	fail := func(context.Context, string) error { failing++; return errors.New("timeout") }
	c.lookup(context.Background(), "sysC", fail)
	if got := c.lookup(context.Background(), "sysC", fail); got != okCodeUnknown || failing != 2 {
		t.Errorf("a failed lookup is unknown and retried; got %v after %d calls", got, failing)
	}
}
