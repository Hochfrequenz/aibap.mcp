//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetTextElements_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetTextElements(ctx, testReportURI)
	if err != nil {
		// Text elements endpoint may not exist on ECC — skip gracefully
		t.Skipf("GetTextElements not available: %v", err)
	}
	t.Logf("symbols: %d, selections: %d", len(result.Symbols), len(result.Selections))
	for _, s := range result.Symbols {
		t.Logf("  symbol [%s] = %q (max %d)", s.Key, s.Text, s.MaxLength)
	}
	for _, s := range result.Selections {
		t.Logf("  selection [%s] = %q", s.Name, s.Text)
	}
}
