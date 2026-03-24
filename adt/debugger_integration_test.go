//go:build integration

package adt_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestDebugSetBreakpoint_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	dbg := adt.NewDebugSession(client, os.Getenv("SAP_INTEGRATION_USER"))

	// Line 3 has executable code: lv_test = 'Hello debugger'.
	bp, err := dbg.SetBreakpoint(context.Background(),
		testReportURI+"/source/main",
		3, "PROG/P", "Z_ADT_MCP_TEST_REPORT")
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	t.Logf("breakpoint: id=%q error=%q", bp.ID, bp.ErrorMessage)
	if bp.ID == "" && bp.ErrorMessage == "" {
		t.Error("expected either ID or error message")
	}
}

// TestDebugFullFlow_Integration tests the complete debug flow:
// 1. Set breakpoint (syncMode=full)
// 2. Start listener (long poll)
// 3. Trigger code execution via unit test runner
// 4. Listener wakes up with debuggee session
// 5. Attach to debugger
//
// Note: Step/Variables/Stack require RFC (stateful session) and are not
// yet supported over HTTP. See docs/debugger-investigation.md.
func TestDebugFullFlow_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	user := os.Getenv("SAP_INTEGRATION_USER")
	dbg := adt.NewDebugSession(client, user)
	ctx := context.Background()

	// Ensure the test report has a test class with executable code.
	// The report must contain a test class for the unit test runner to execute it.
	ensureTestReportWithTestClass(t, client)

	// 1. Set breakpoint on the test method body (line 14: lv_val = 'test'.)
	bp, err := dbg.SetBreakpoint(ctx, testReportURI+"/source/main",
		14, "PROG/P", "Z_ADT_MCP_TEST_REPORT")
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	if bp.ErrorMessage != "" {
		t.Fatalf("breakpoint error: %s", bp.ErrorMessage)
	}
	t.Logf("breakpoint set: id=%q", bp.ID)

	// 2. Start listener in goroutine
	type listenerOut struct {
		result *adt.ListenerResult
		err    error
	}
	listenerCh := make(chan listenerOut, 1)
	go func() {
		r, err := dbg.StartListener(ctx, 60)
		listenerCh <- listenerOut{r, err}
	}()

	// 3. Give listener time to register, then trigger execution
	time.Sleep(2 * time.Second)
	t.Log("running unit tests to trigger breakpoint...")
	_, err = client.RunUnitTests(ctx, testReportURI, 60)
	// RunUnitTests will block until the debuggee times out or is continued.
	// We don't check its result here — the listener result is what matters.

	// 4. Wait for listener
	lo := <-listenerCh
	if lo.err != nil {
		t.Fatalf("StartListener: %v", lo.err)
	}
	if lo.result.Status != "attached" {
		t.Fatalf("listener status: got %q, want attached", lo.result.Status)
	}
	if lo.result.DebuggeeID == "" {
		t.Fatal("listener returned no debuggee ID")
	}
	t.Logf("listener attached: debuggeeID=%q", lo.result.DebuggeeID)

	// 5. Attach
	err = dbg.Attach(ctx, lo.result.DebuggeeID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	t.Log("attached to debugger successfully")

	// Step/Variables/Stack require RFC stateful session — not yet supported.
	// See docs/debugger-investigation.md for details.
	t.Log("NOTE: step/variables/stack require RFC and are not tested here")
}

// ensureTestReportWithTestClass creates or updates the test report to include
// a test class so the unit test runner can execute it.
func ensureTestReportWithTestClass(t *testing.T, client adt.Client) {
	t.Helper()
	ctx := context.Background()

	src, err := client.GetSource(ctx, testReportURI)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}

	// Check if it already has a test class
	if strings.Contains(src.Source, "FOR TESTING") {
		return
	}

	newSource := `REPORT z_adt_mcp_test_report.
DATA: lv_test TYPE string.
lv_test = 'Hello debugger'.
WRITE: / lv_test.

CLASS lcl_test DEFINITION FOR TESTING RISK LEVEL HARMLESS DURATION SHORT.
  PRIVATE SECTION.
    METHODS test_hello FOR TESTING.
ENDCLASS.

CLASS lcl_test IMPLEMENTATION.
  METHOD test_hello.
    DATA: lv_val TYPE string.
    lv_val = 'test'.
    cl_abap_unit_assert=>assert_equals( act = lv_val exp = 'test' ).
  ENDMETHOD.
ENDCLASS.
`

	lockHandle, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	defer func() { _ = client.UnlockObject(ctx, testReportURI, lockHandle) }()

	_, err = client.SetSource(ctx, testReportURI, newSource, lockHandle, "", src.ETag)
	if err != nil {
		t.Fatalf("SetSource: %v", err)
	}

	result, err := client.ActivateObjects(ctx, []string{testReportURI})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !result.Success {
		t.Fatalf("activation failed: %v", result.Messages)
	}
	t.Log("test report updated with test class")
}
