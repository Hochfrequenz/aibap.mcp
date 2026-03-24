//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestSearchObjects_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Search for the test report by exact name.
	results, err := client.SearchObjects(ctx, "Z_ADT_MCP_TEST_REPORT", "", 10)
	if err != nil {
		t.Fatalf("SearchObjects failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SearchObjects returned no results for Z_ADT_MCP_TEST_REPORT")
	}

	found := false
	for _, r := range results {
		t.Logf("  %s (%s) — %s", r.Name, r.Type, r.URI)
		if r.Name == "Z_ADT_MCP_TEST_REPORT" {
			found = true
		}
	}
	if !found {
		t.Error("Z_ADT_MCP_TEST_REPORT not found in search results")
	}
}

func TestSearchObjects_Wildcard(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Wildcard search limited to 5 results.
	results, err := client.SearchObjects(ctx, "Z_ADT_MCP_TEST*", "", 5)
	if err != nil {
		t.Fatalf("SearchObjects wildcard failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SearchObjects wildcard returned no results")
	}
	t.Logf("wildcard search returned %d results", len(results))
	for _, r := range results {
		t.Logf("  %s (%s)", r.Name, r.Type)
	}
}

func TestSearchObjects_NoResults(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	results, err := client.SearchObjects(ctx, "Z_DEFINITELY_DOES_NOT_EXIST_99", "", 5)
	if err != nil {
		t.Fatalf("SearchObjects failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
