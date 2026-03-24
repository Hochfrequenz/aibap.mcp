//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetTransportRequests_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Query modifiable transports for the integration test user.
	transports, err := client.GetTransportRequests(ctx, "", "D")
	if err != nil {
		t.Fatalf("GetTransportRequests failed: %v", err)
	}
	t.Logf("got %d modifiable transport requests", len(transports))

	for i, tr := range transports {
		if i >= 5 {
			t.Logf("  ... and %d more", len(transports)-5)
			break
		}
		t.Logf("  [%d] %s owner=%s status=%s %q", i, tr.Number, tr.Owner, tr.Status, tr.Description)
	}
}
