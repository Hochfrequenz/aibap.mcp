//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestCreateAndDeleteObject_Integration(t *testing.T) {
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
	if lh, err := client.LockObject(ctx, objectURI); err == nil {
		_ = client.DeleteObject(ctx, objectURI, lh, "")
	}

	// Create the object.
	err := client.CreateObject(ctx, objectType, objectName, packageName, description, "")
	if err != nil {
		t.Fatalf("CreateObject failed: %v", err)
	}
	t.Logf("created %s %s in %s", objectType, objectName, packageName)

	// Register cleanup via lock + delete.
	t.Cleanup(func() {
		lh, err := client.LockObject(context.Background(), objectURI)
		if err != nil {
			return
		}
		_ = client.DeleteObject(context.Background(), objectURI, lh, "")
	})

	// Verify the object exists by reading its source.
	src, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource after create failed: %v", err)
	}
	t.Logf("source length after create: %d", len(src.Source))

	// Delete it explicitly (the main test).
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject for delete failed: %v", err)
	}
	err = client.DeleteObject(ctx, objectURI, lockHandle, "")
	if err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	t.Log("deleted successfully")

	// Verify deletion.
	_, err = client.GetObjectInfo(ctx, objectURI)
	if err == nil {
		t.Error("expected error after deletion, object still exists")
	}
}

func TestCreatePackage_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()
	cfg := integrationConfig()

	const pkgName = "Z_ADT_MCP_INTTEST_PKG"

	err := client.CreatePackage(ctx, pkgName, "Integration test package",
		cfg.User, "HOME", "ZS4U", "")
	if err != nil {
		// Package may already exist from a previous run.
		if _, browseErr := client.BrowsePackage(ctx, pkgName); browseErr != nil {
			t.Fatalf("CreatePackage failed and package does not exist: %v", err)
		}
		t.Logf("package %s already exists, reusing", pkgName)
	} else {
		t.Logf("created package %s", pkgName)
	}

	// Verify it exists via BrowsePackage.
	objects, err := client.BrowsePackage(ctx, pkgName)
	if err != nil {
		t.Fatalf("BrowsePackage after create failed: %v", err)
	}
	t.Logf("package %s is browsable, contains %d objects", pkgName, len(objects))
}

func TestDeleteObject_NonExistentReturnsError(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	err := client.DeleteObject(ctx, "/sap/bc/adt/programs/programs/Z_ADT_MCP_DOES_NOT_EXIST_99", "fake-handle", "")
	if err == nil {
		t.Fatal("expected error when deleting non-existent object, got nil")
	}
	t.Logf("delete non-existent correctly returned error: %v", err)
}
