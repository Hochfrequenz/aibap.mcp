//go:build integration && transport

package adt_test

import (
	"context"
	"testing"
)

// Tests in this file create, modify, and release transport requests on the
// SAP system. They are excluded from normal integration runs and require:
//
//	go test ./adt/ -tags 'integration transport' -v

func TestCreateTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP integration test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport failed: %v", err)
	}
	if trNumber == "" {
		t.Fatal("expected non-empty transport number")
	}
	if len(trNumber) != 10 {
		t.Errorf("expected 10-char transport number (e.g. S4UK900001), got %q", trNumber)
	}
	t.Logf("created transport: %s", trNumber)
}

func TestReleaseTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP release test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport failed: %v", err)
	}
	t.Logf("created transport: %s", trNumber)

	err = client.ReleaseTransport(ctx, trNumber)
	if err != nil {
		t.Fatalf("ReleaseTransport failed: %v", err)
	}
	t.Logf("released transport: %s", trNumber)
}

func TestAddToTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP AddToTransport test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("created transport: %s", trNumber)
	t.Cleanup(func() { _ = client.ReleaseTransport(context.Background(), trNumber) })

	const objName = "Z_ADT_MCP_TR_ADD_TST"
	objectURI := "/sap/bc/adt/programs/programs/" + objName
	err = client.CreateObject(ctx, "PROG", objName, testPackage, "Transport add test", trNumber)
	if err != nil {
		if _, infoErr := client.GetObjectInfo(ctx, objectURI); infoErr != nil {
			t.Fatalf("CreateObject failed and object does not exist: %v", err)
		}
		t.Logf("object %s already exists, reusing", objName)
	} else {
		t.Logf("created %s in %s", objName, testPackage)
	}

	check, err := client.CheckTransport(ctx, "R3TR", "PROG", objName)
	if err != nil {
		t.Fatalf("CheckTransport: %v", err)
	}
	if len(check.Requests) == 0 {
		t.Skip("no transport requests available")
	}

	taskNumber := check.Requests[0].Number
	err = client.AddToTransport(ctx, objectURI, taskNumber)
	if err != nil {
		t.Fatalf("AddToTransport: %v", err)
	}
	t.Logf("added %s to %s", objectURI, taskNumber)
}

// TestTransportFullCycle_Integration tests the complete transport lifecycle:
// 1. Create transport
// 2. Create a program in a real package (assigned to transport)
// 3. CheckTransport to verify the object is recordable
// 4. Activate the object (creates a version in VRSD)
// 5. Verify version history and retrieve historical source
// 6. Delete the program
// 7. Release the transport
func TestTransportFullCycle_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// 1. Create transport
	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP full cycle test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("[1] created transport: %s", trNumber)

	// 2. Create a program in a real package, assigned to this transport
	const objName = "Z_ADT_MCP_FULLCYCLE"
	objectURI := "/sap/bc/adt/programs/programs/" + objName
	err = client.CreateObject(ctx, "PROG", objName, testPackage, "Full cycle test", trNumber)
	if err != nil {
		_ = client.ReleaseTransport(ctx, trNumber)
		t.Fatalf("CreateObject: %v", err)
	}
	t.Logf("[2] created %s in %s (transport %s)", objName, testPackage, trNumber)

	// 3. CheckTransport — verify the object is known to CTS
	check, err := client.CheckTransport(ctx, "R3TR", "PROG", objName)
	if err != nil {
		t.Fatalf("CheckTransport: %v", err)
	}
	t.Logf("[3] CheckTransport: result=%s recording=%v devclass=%s", check.Result, check.Recording, check.DevClass)
	if check.DevClass != testPackage {
		t.Errorf("expected DevClass %s, got %q", testPackage, check.DevClass)
	}

	// 4. Activate the object — this creates a version entry
	result, err := client.ActivateObjects(ctx, []string{objectURI})
	if err != nil {
		t.Fatalf("ActivateObjects: %v", err)
	}
	t.Logf("[4] activated %s (success=%v, messages=%d)", objName, result.Success, len(result.Messages))

	// 5. Verify version history and retrieve historical source
	versions, err := client.GetVersionHistory(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetVersionHistory: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one version after activation")
	}
	t.Logf("[5] version history: %d version(s)", len(versions))
	for i, v := range versions {
		t.Logf("    [%d] version=%s author=%s transport=%s date=%s", i, v.VersionNumber, v.Author, v.Transport, v.Date)
	}

	latest := versions[0]
	if latest.ContentURI == "" {
		t.Fatal("expected content_uri in version entry")
	}
	src, err := client.GetVersionSource(ctx, latest.ContentURI)
	if err != nil {
		t.Fatalf("GetVersionSource: %v", err)
	}
	if src == "" {
		t.Fatal("expected non-empty source from version")
	}
	t.Logf("[5] retrieved version source (%d bytes)", len(src))

	// 6. Delete the program (needs lock + transport)
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	err = client.DeleteObject(ctx, objectURI, lockHandle, trNumber)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		t.Fatalf("DeleteObject: %v", err)
	}
	t.Logf("[6] deleted %s", objName)

	// 7. Release the transport
	err = client.ReleaseTransport(ctx, trNumber)
	if err != nil {
		t.Fatalf("ReleaseTransport: %v", err)
	}
	t.Logf("[7] released transport %s", trNumber)
}
