//go:build integration

package adt_test

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
)

func TestExportPackage_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Export the test package — its objects are created by the fixture setup
	// or already exist on the system from a previous run.
	data, err := client.ExportPackage(ctx, "Z_ADT_MCP_TEST")
	if err != nil {
		t.Fatalf("ExportPackage failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ExportPackage returned empty data")
	}
	t.Logf("ZIP size: %d bytes", len(data))

	// Verify it's a valid ZIP.
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid ZIP: %v", err)
	}
	t.Logf("ZIP contains %d files:", len(zr.File))
	for _, f := range zr.File {
		t.Logf("  %s (%d bytes)", f.Name, f.UncompressedSize64)
	}

	// Expect at least package.devc.xml and .abapgit.xml.
	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names[".abapgit.xml"] {
		t.Error("ZIP missing .abapgit.xml")
	}
	if !names["src/package.devc.xml"] {
		t.Error("ZIP missing src/package.devc.xml")
	}
}

func TestExportPackage_NonExistent(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	_, err := client.ExportPackage(ctx, "Z_DEFINITELY_DOES_NOT_EXIST_99")
	if err == nil {
		t.Fatal("expected error for non-existent package, got nil")
	}
	t.Logf("expected error: %v", err)
}
