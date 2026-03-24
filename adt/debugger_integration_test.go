//go:build integration

package adt_test

import (
	"context"
	"os"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestDebugSetBreakpoint_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	dbg := adt.NewDebugSession(client, os.Getenv("SAP_INTEGRATION_USER"))

	bp, err := dbg.SetBreakpoint(context.Background(),
		testReportURI+"/source/main",
		2, "PROG/P", "Z_ADT_MCP_TEST_REPORT")
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	t.Logf("breakpoint: id=%q error=%q", bp.ID, bp.ErrorMessage)
	if bp.ID == "" && bp.ErrorMessage == "" {
		t.Error("expected either ID or error message")
	}
}

func TestDebugStartListenerTimeout_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	dbg := adt.NewDebugSession(client, os.Getenv("SAP_INTEGRATION_USER"))

	// Set a breakpoint first, then start listener with short timeout.
	bp, err := dbg.SetBreakpoint(context.Background(),
		testReportURI+"/source/main",
		2, "PROG/P", "Z_ADT_MCP_TEST_REPORT")
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	t.Logf("breakpoint: id=%q", bp.ID)

	result, err := dbg.StartListener(context.Background(), 2)
	if err != nil {
		t.Fatalf("StartListener: %v", err)
	}
	t.Logf("listener result: status=%q debuggeeID=%q", result.Status, result.DebuggeeID)
}

// TestDebugListenerInvestigation_Integration explores different parameter
// combinations to understand the listener behavior.
func TestDebugListenerInvestigation_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	user := os.Getenv("SAP_INTEGRATION_USER")

	t.Run("listener_before_breakpoint", func(t *testing.T) {
		dbg := adt.NewDebugSession(client, user)

		// Start listener BEFORE setting breakpoint
		result, err := dbg.StartListener(context.Background(), 2)
		if err != nil {
			t.Fatalf("StartListener: %v", err)
		}
		t.Logf("listener (before bp): status=%q debuggeeID=%q", result.Status, result.DebuggeeID)
	})

	t.Run("breakpoint_then_listener", func(t *testing.T) {
		dbg := adt.NewDebugSession(client, user)

		bp, err := dbg.SetBreakpoint(context.Background(),
			testReportURI+"/source/main",
			2, "PROG/P", "Z_ADT_MCP_TEST_REPORT")
		if err != nil {
			t.Fatalf("SetBreakpoint: %v", err)
		}
		t.Logf("breakpoint: id=%q error=%q", bp.ID, bp.ErrorMessage)

		result, err := dbg.StartListener(context.Background(), 3)
		if err != nil {
			t.Fatalf("StartListener: %v", err)
		}
		t.Logf("listener (after bp): status=%q debuggeeID=%q", result.Status, result.DebuggeeID)

		// Cleanup
		_ = dbg.StopListener(context.Background())
	})

	t.Run("get_sessions", func(t *testing.T) {
		dbg := adt.NewDebugSession(client, user)
		data, err := dbg.GetDebuggeeSessions(context.Background())
		if err != nil {
			t.Fatalf("GetDebuggeeSessions: %v", err)
		}
		t.Logf("sessions response:\n%s", string(data))
	})
}
