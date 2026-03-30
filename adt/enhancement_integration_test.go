//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetEnhancementSpot_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetEnhancementSpot(ctx, "BADI_ACC_DOCUMENT")
	if err != nil {
		t.Fatalf("GetEnhancementSpot: %v", err)
	}
	if result.Name != "BADI_ACC_DOCUMENT" {
		t.Errorf("name: got %q, want BADI_ACC_DOCUMENT", result.Name)
	}
	if len(result.Definitions) == 0 {
		t.Fatal("expected at least one BAdI definition")
	}
	t.Logf("spot %q: %q, package %q, %d definitions", result.Name, result.Description, result.Package, len(result.Definitions))
	for _, d := range result.Definitions {
		t.Logf("  BAdI %q: interface=%q, single_use=%v, filters=%d", d.Name, d.Interface.Name, d.SingleUse, len(d.Filters))
	}
}

func TestGetEnhancementImplementation_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetEnhancementImplementation(ctx, "ZEI_BADI_BPEM")
	if err != nil {
		t.Fatalf("GetEnhancementImplementation: %v", err)
	}
	if result.Name != "ZEI_BADI_BPEM" {
		t.Errorf("name: got %q, want ZEI_BADI_BPEM", result.Name)
	}
	if len(result.Implementations) == 0 {
		t.Fatal("expected at least one BAdI implementation entry")
	}
	t.Logf("impl %q: %q, package %q, %d entries", result.Name, result.Description, result.Package, len(result.Implementations))
	for _, e := range result.Implementations {
		t.Logf("  %q: class=%q, active=%v, default=%v", e.Name, e.ImplementingClass.Name, e.IsActive, e.IsDefault)
	}
}
