//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetATCCustomizing_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetATCCustomizing(ctx)
	if err != nil {
		t.Fatalf("GetATCCustomizing failed: %v", err)
	}

	if result.SystemCheckVariant == "" {
		t.Error("expected non-empty systemCheckVariant")
	}
	t.Logf("systemCheckVariant: %s", result.SystemCheckVariant)
	t.Logf("properties: %v", result.Properties)
}

// TestRunATCCheck_Integration attempts to run ATC checks on a test object.
//
// KNOWN ISSUE (2026-03-24): The POST /sap/bc/adt/atc/runs endpoint returns
// HTTP 500 "An exception was raised" on the S/4 HANA test system
// (srvhfuhana.sap.msp.local:44300). This affects ALL request formats and
// even empty object sets — the server crashes before processing the request.
// Root cause is likely a missing SAP Note or misconfigured ATC check variant
// (ZCB_CLEAN_ABAP_1). See issue #24 for details.
//
// This test documents the current behavior and will start passing once the
// server-side issue is resolved.
func TestRunATCCheck_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Read the system check variant to pass explicitly — avoids NULL pointer
	// on ECC systems where no default variant is configured.
	cust, err := client.GetATCCustomizing(ctx)
	if err != nil {
		t.Skipf("GetATCCustomizing failed, cannot determine check variant: %v", err)
	}
	variant := cust.SystemCheckVariant
	t.Logf("using check variant: %q", variant)

	objectURI := testClassURI
	result, err := client.RunATCCheck(ctx, []string{objectURI}, variant)
	if err != nil {
		// Expected to fail on current S/4 system — see KNOWN ISSUE above.
		t.Skipf("RunATCCheck not available on this system: %v", err)
	}

	t.Logf("worklist ID: %s", result.WorklistID)
	t.Logf("findings: %d", len(result.Findings))
	for i, f := range result.Findings {
		if i >= 10 {
			t.Logf("  ... and %d more", len(result.Findings)-10)
			break
		}
		t.Logf("  [%s] %s: %s (%s)", f.Priority, f.CheckTitle, f.MessageTitle, f.ObjectURI)
	}
}
