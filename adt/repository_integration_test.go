//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestBrowsePackage_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	results, err := client.BrowsePackage(ctx, "STUN")
	if err != nil {
		t.Fatalf("BrowsePackage failed: %v", err)
	}
	t.Logf("got %d objects in package STUN", len(results))
	for i, obj := range results {
		if i >= 10 {
			t.Logf("  ... and %d more", len(results)-10)
			break
		}
		t.Logf("  [%d] %s %s %q", i, obj.Type, obj.Name, obj.Description)
	}
}

func TestGetObjectInfo_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	info, err := client.GetObjectInfo(ctx, "/sap/bc/adt/programs/programs/RSPARAM")
	if err != nil {
		t.Fatalf("GetObjectInfo failed: %v", err)
	}
	t.Logf("name=%s type=%s description=%q package=%s", info.Name, info.Type, info.Description, info.PackageName)
	if info.Name == "" {
		t.Error("expected non-empty name")
	}
}
