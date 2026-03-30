//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetTransportObjects_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	objects, err := client.GetTransportObjects(ctx, "HFQK900178")
	if err != nil {
		t.Fatalf("GetTransportObjects failed: %v", err)
	}

	t.Logf("transport HFQK900178 contains %d objects", len(objects))
	for i, obj := range objects {
		t.Logf("  [%d] pgmid=%s type=%s name=%s wb_type=%s", i, obj.PgmID, obj.Type, obj.Name, obj.WBType)
	}
}
