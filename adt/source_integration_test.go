//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
)

func TestGetSource_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetSource(ctx, testReportURI)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if result.Source == "" {
		t.Fatal("GetSource returned empty source")
	}
	if result.ETag == "" {
		t.Fatal("GetSource returned empty ETag")
	}
	if !strings.Contains(strings.ToLower(result.Source), "report") {
		t.Errorf("source does not contain REPORT keyword: %q", result.Source[:min(100, len(result.Source))])
	}
	t.Logf("ETag: %s, source length: %d, first line: %s",
		result.ETag, len(result.Source), strings.SplitN(result.Source, "\n", 2)[0])
}

func TestGetSource_NonExistent_ReturnsError(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	_, err := client.GetSource(ctx, "/sap/bc/adt/programs/programs/Z_DOES_NOT_EXIST_99999")
	if err == nil {
		t.Fatal("expected error for non-existent object, got nil")
	}
	t.Logf("non-existent object correctly returned error: %v", err)
}
