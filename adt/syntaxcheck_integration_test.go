//go:build integration

package adt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestSyntaxCheck_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Use the fixture with an unused variable — SAP should return at least a warning.
	msgs, err := client.SyntaxCheck(ctx, testSynWarnURI)
	if err != nil {
		t.Fatalf("SyntaxCheck failed: %v", err)
	}

	// #12: Verify that check run response parsing works — we expect messages
	// because the fixture has an unused DATA variable.
	if len(msgs) == 0 {
		t.Fatal("expected at least one syntax message for fixture with unused variable, got 0")
	}
	t.Logf("got %d syntax messages for %s", len(msgs), testSynWarnURI)

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

func TestInlineSyntaxCheck_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Check source with an error — unknown type should produce an error message.
	source := "REPORT z_mcp_inline_test.\nDATA lv_x TYPE znonexistent_type_abc.\nWRITE lv_x.\n"
	msgs, err := client.InlineSyntaxCheck(ctx, testReportURI, source)
	if err != nil {
		if errors.Is(err, adt.ErrInlineSyntaxCheckNotSupported) {
			t.Skip("inline syntax check not supported on this system")
		}
		t.Fatalf("InlineSyntaxCheck failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one message for source with unknown type")
	}
	t.Logf("got %d messages", len(msgs))
	for i, m := range msgs {
		t.Logf("  [%d] %s line=%d col=%d %q", i, m.Type, m.Line, m.Column, m.Text)
	}

	hasError := false
	for _, m := range msgs {
		if m.Type == "E" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("expected at least one error message")
	}
}

func TestInlineSyntaxCheck_Clean_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Valid source — should produce no errors.
	source := "REPORT z_mcp_inline_test.\nWRITE: / 'Hello'.\n"
	msgs, err := client.InlineSyntaxCheck(ctx, testReportURI, source)
	if err != nil {
		if errors.Is(err, adt.ErrInlineSyntaxCheckNotSupported) {
			t.Skip("inline syntax check not supported on this system")
		}
		t.Fatalf("InlineSyntaxCheck failed: %v", err)
	}
	for _, m := range msgs {
		if m.Type == "E" {
			t.Errorf("unexpected error for valid source: %q (line %d)", m.Text, m.Line)
		}
	}
	t.Logf("got %d messages for valid source", len(msgs))
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
