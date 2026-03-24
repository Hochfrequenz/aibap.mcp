//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
)

func TestCreateObject_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	const (
		objectType  = "PROG"
		objectName  = "Z_ADT_MCP_INTTEST_TMP"
		packageName = "$TMP"
		description = "Temporary integration test program"
		objectURI   = "/sap/bc/adt/programs/programs/" + objectName
	)

	// Best-effort cleanup of leftovers from a previous failed run.
	// DeleteObject currently fails (needs lockHandle, see #40), so this
	// may not succeed. If it doesn't, the test will fail on CreateObject
	// with "already exists" and needs manual SAP cleanup.
	_ = client.DeleteObject(ctx, objectURI, "")

	// Create the object.
	err := client.CreateObject(ctx, objectType, objectName, packageName, description, "")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			t.Skipf("object %s already exists from a previous run; manual cleanup required: %v", objectName, err)
		}
		t.Fatalf("CreateObject failed: %v", err)
	}
	t.Logf("created %s %s in %s", objectType, objectName, packageName)

	// Register cleanup — best-effort since DeleteObject is currently broken (#40).
	t.Cleanup(func() {
		_ = client.DeleteObject(context.Background(), objectURI, "")
	})

	// Verify the object exists by reading its source.
	src, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource after create failed: %v", err)
	}
	t.Logf("source length after create: %d", len(src.Source))
}

func TestDeleteObject_NonExistentReturnsError(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	err := client.DeleteObject(ctx, "/sap/bc/adt/programs/programs/Z_ADT_MCP_DOES_NOT_EXIST_99", "")
	if err == nil {
		t.Fatal("expected error when deleting non-existent object, got nil")
	}
	t.Logf("delete non-existent correctly returned error: %v", err)
}
