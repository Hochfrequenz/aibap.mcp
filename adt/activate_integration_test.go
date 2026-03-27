//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestActivateObjects_Clean_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Activate the test report which should have valid source.
	result, err := client.ActivateObjects(ctx, []string{testReportURI})
	if err != nil {
		t.Fatalf("ActivateObjects failed: %v", err)
	}
	t.Logf("success=%v messages=%d", result.Success, len(result.Messages))
	if !result.Success {
		for _, m := range result.Messages {
			t.Logf("  [%s] %s (uri=%s)", m.Type, m.Text, m.ObjectURI)
		}
		t.Error("expected success for valid source")
	}
}

func TestGetInactiveObjects_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	objects, err := client.GetInactiveObjects(ctx)
	if err != nil {
		t.Fatalf("GetInactiveObjects: %v", err)
	}
	t.Logf("got %d inactive objects", len(objects))
	for i, o := range objects {
		if i >= 10 {
			t.Logf("  ... and %d more", len(objects)-10)
			break
		}
		t.Logf("  [%d] %s (%s) %s", i, o.Name, o.Type, o.URI)
	}
}

func TestActivateObjects_WithErrors_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Write invalid source, activate, check for errors, then restore.
	lockHandle, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	t.Cleanup(func() {
		// Restore valid source and activate.
		lh, err := client.LockObject(context.Background(), testReportURI)
		if err != nil {
			return
		}
		src, _ := client.GetSource(context.Background(), testReportURI)
		validSource := "REPORT z_adt_mcp_test_report.\nWRITE: / 'Hello from MCP integration test'.\n"
		client.SetSource(context.Background(), testReportURI, validSource, lh, "", src.ETag)
		client.UnlockObject(context.Background(), testReportURI, lh)
		client.ActivateObjects(context.Background(), []string{testReportURI})
	})

	src, err := client.GetSource(ctx, testReportURI)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}

	invalidSource := "REPORT z_adt_mcp_test_report.\nTHIS IS NOT VALID ABAP.\n"
	_, err = client.SetSource(ctx, testReportURI, invalidSource, lockHandle, "", src.ETag)
	if err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	_ = client.UnlockObject(ctx, testReportURI, lockHandle)

	// Activate should now report errors.
	result, err := client.ActivateObjects(ctx, []string{testReportURI})
	if err != nil {
		t.Fatalf("ActivateObjects: %v", err)
	}
	t.Logf("success=%v messages=%d", result.Success, len(result.Messages))
	for _, m := range result.Messages {
		t.Logf("  [%s] %s (uri=%s)", m.Type, m.Text, m.ObjectURI)
	}
	if result.Success {
		t.Error("expected Success=false for invalid ABAP source")
	}
	if len(result.Messages) == 0 {
		t.Error("expected at least one error message")
	}
}
