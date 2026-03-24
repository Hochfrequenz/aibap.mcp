//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestSyntaxCheck_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Use the fixture with an unused variable — SAP should return at least a warning.
	const synWarnURI = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_SYNWARN"
	msgs, err := client.SyntaxCheck(ctx, synWarnURI)
	if err != nil {
		t.Fatalf("SyntaxCheck failed: %v", err)
	}

	// #12: Verify that check run response parsing works — we expect messages
	// because the fixture has an unused DATA variable.
	if len(msgs) == 0 {
		t.Fatal("expected at least one syntax message for fixture with unused variable, got 0")
	}
	t.Logf("got %d syntax messages for %s", len(msgs), synWarnURI)

	for i, m := range msgs {
		// Every message must have Type and Text populated.
		if m.Type == "" {
			t.Errorf("message [%d]: Type is empty", i)
		}
		if m.Text == "" {
			t.Errorf("message [%d]: Text is empty", i)
		}
		// Line/Column are parsed from URI fragments — verify they are > 0
		// for at least the first message (the unused variable warning).
		if i == 0 && m.Line == 0 {
			t.Errorf("message [0]: Line is 0, expected non-zero (URI fragment parsing may be broken)")
		}
		if i < 10 {
			t.Logf("  [%d] %s line=%d col=%d %q", i, m.Type, m.Line, m.Column, m.Text)
		}
	}
}

func TestSyntaxCheck_CleanCode_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// The clean test report should have no errors (may have informational messages).
	msgs, err := client.SyntaxCheck(ctx, testReportURI)
	if err != nil {
		t.Fatalf("SyntaxCheck failed: %v", err)
	}
	t.Logf("got %d syntax messages for %s", len(msgs), testReportURI)

	for _, m := range msgs {
		if m.Type == "E" {
			t.Errorf("unexpected error in clean fixture: %s (line %d)", m.Text, m.Line)
		}
	}
}
