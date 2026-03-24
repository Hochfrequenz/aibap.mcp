//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestSyntaxCheck_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	msgs, err := client.SyntaxCheck(ctx, testReportURI)
	if err != nil {
		t.Fatalf("SyntaxCheck failed: %v", err)
	}
	t.Logf("got %d syntax messages for %s", len(msgs), testReportURI)
	for i, m := range msgs {
		if i >= 10 {
			t.Logf("  ... and %d more", len(msgs)-10)
			break
		}
		t.Logf("  [%d] %s line=%d col=%d %q", i, m.Type, m.Line, m.Column, m.Text)
	}
}
