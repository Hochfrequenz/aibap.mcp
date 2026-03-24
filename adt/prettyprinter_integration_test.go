//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
)

func TestPrettyPrint_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Unformatted ABAP source — mixed case, inconsistent spacing.
	input := "report z_test.\nwrite: / 'hello'.\n"

	formatted, err := client.PrettyPrint(ctx, input)
	if err != nil {
		t.Fatalf("PrettyPrint failed: %v", err)
	}
	if formatted == "" {
		t.Fatal("PrettyPrint returned empty result")
	}
	t.Logf("formatted source:\n%s", formatted)

	// The pretty printer should return valid ABAP; at minimum the output
	// should contain the original keywords (possibly upper-cased).
	lower := strings.ToLower(formatted)
	if !strings.Contains(lower, "report") {
		t.Error("formatted source missing REPORT keyword")
	}
	if !strings.Contains(lower, "write") {
		t.Error("formatted source missing WRITE keyword")
	}
}

func TestPrettyPrint_EmptySource(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Empty source should not cause a server error.
	result, err := client.PrettyPrint(ctx, "")
	if err != nil {
		t.Fatalf("PrettyPrint with empty source failed: %v", err)
	}
	t.Logf("empty source result: %q", result)
}
