//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetTransportRequests_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// #14: Query modifiable transports — the correct Accept header
	// (application/vnd.sap.adt.transportorganizertree.v1+xml) is required
	// or SAP returns 406. If this call succeeds, the header fix is working.
	transports, err := client.GetTransportRequests(ctx, "", "D")
	if err != nil {
		t.Fatalf("GetTransportRequests failed: %v", err)
	}
	if len(transports) == 0 {
		t.Fatal("expected at least one modifiable transport request, got 0")
	}
	t.Logf("got %d modifiable transport requests", len(transports))

	// Verify returned transports have essential fields populated.
	for i, tr := range transports {
		if tr.Number == "" {
			t.Errorf("transport [%d]: Number is empty", i)
		}
		if tr.Owner == "" {
			t.Errorf("transport [%d]: Owner is empty", i)
		}
		if tr.Status == "" {
			t.Errorf("transport [%d]: Status is empty", i)
		}
		if i < 5 {
			t.Logf("  [%d] %s owner=%s status=%s %q", i, tr.Number, tr.Owner, tr.Status, tr.Description)
		}
	}
}
