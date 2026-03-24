//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetCompletions_Integration(t *testing.T) {
	// Tests code completion against the SAP system.
	// Note: response XML parsing may fail if SAP returns asx:abap format
	// instead of the expected <completions> format — that is a separate issue.
	client := newIntegrationClient(t)
	ctx := context.Background()

	source := "REPORT z_adt_mcp_test_report.\nWRITE "
	line := 2
	column := 6

	completions, err := client.GetCompletions(ctx, testReportURI, source, line, column)
	if err != nil {
		t.Fatalf("GetCompletions failed: %v", err)
	}
	t.Logf("got %d completions", len(completions))

	for i, c := range completions {
		if i >= 5 {
			t.Logf("  ... and %d more", len(completions)-5)
			break
		}
		t.Logf("  [%d] %s — %s", i, c.Text, c.Description)
	}
}
