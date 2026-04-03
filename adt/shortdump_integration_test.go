//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestListShortDumps_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// List dumps from the last 7 days
	dumps, err := client.ListShortDumps(ctx, "20260326000000", "", "")
	if err != nil {
		t.Fatalf("ListShortDumps: %v", err)
	}
	t.Logf("found %d dumps", len(dumps))
	for i, d := range dumps {
		if i >= 5 {
			t.Logf("  ... and %d more", len(dumps)-5)
			break
		}
		t.Logf("  [%d] %s %s user=%s time=%s", i, d.RuntimeError, d.Program, d.User, d.Timestamp)
	}
}

func TestGetShortDumps_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Get full details for dumps from the last 24 hours
	dumps, err := client.GetShortDumps(ctx, "20260402000000", "", "")
	if err != nil {
		t.Fatalf("GetShortDumps: %v", err)
	}
	t.Logf("found %d dumps with details", len(dumps))
	for i, d := range dumps {
		if i >= 2 {
			break
		}
		t.Logf("  [%d] %s — %s", i, d.RuntimeError, d.Program)
		if d.AbortLocation != "" {
			t.Logf("    Abort: %s", d.AbortLocation)
		}
		if d.SourceLink != "" {
			t.Logf("    Source: %s", d.SourceLink)
		}
		if d.CallStack != "" {
			lines := 0
			for _, c := range d.CallStack {
				if c == '\n' {
					lines++
				}
			}
			t.Logf("    Stack: %d lines", lines+1)
		}
	}
}
