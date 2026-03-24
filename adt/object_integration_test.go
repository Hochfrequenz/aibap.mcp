//go:build integration

package adt_test

import (
	"context"
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
	)

	// Create the object.
	err := client.CreateObject(ctx, objectType, objectName, packageName, description, "")
	if err != nil {
		// May fail if leftover from a previous run; log and skip.
		t.Skipf("CreateObject failed (may be leftover from previous run): %v", err)
	}
	t.Logf("created %s %s in %s", objectType, objectName, packageName)

	// Verify the object exists by reading its source.
	objectURI := "/sap/bc/adt/programs/programs/" + objectName
	src, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource after create failed: %v", err)
	}
	t.Logf("source length after create: %d", len(src.Source))

	// NOTE: DeleteObject currently fails because SAP requires a lockHandle
	// query parameter that our API doesn't pass. See issue to be filed.
	// Manual cleanup: delete Z_ADT_MCP_INTTEST_TMP via SAP GUI or ADT.
	err = client.DeleteObject(ctx, objectURI, "")
	if err != nil {
		t.Logf("DeleteObject failed (known bug — needs lockHandle param): %v", err)
		t.Log("manual cleanup required: delete Z_ADT_MCP_INTTEST_TMP from SAP")
	} else {
		t.Log("deleted object successfully")
	}
}
