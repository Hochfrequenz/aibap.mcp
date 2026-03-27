//go:build integration && transport

package adt_test

import (
	"context"
	"testing"
)

// TestTransportFullCycle_Integration tests the complete transport lifecycle:
// 1. Create transport
// 2. Create a program in a real package (assigned to transport)
// 3. CheckTransport to verify the object is recordable
// 4. Delete the program
// 5. Release the transport
//
// This test creates real transports on the SAP system and is excluded from
// normal integration test runs. Run with:
//
//	go test ./adt/ -tags 'integration transport' -run TestTransportFullCycle
func TestTransportFullCycle_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// 1. Create transport
	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP full cycle test", "Z_ADT_MCP_TEST")
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("[1] created transport: %s", trNumber)

	// 2. Create a program in a real package, assigned to this transport
	const objName = "Z_ADT_MCP_FULLCYCLE"
	objectURI := "/sap/bc/adt/programs/programs/" + objName
	err = client.CreateObject(ctx, "PROG", objName, "Z_ADT_MCP_TEST", "Full cycle test", trNumber)
	if err != nil {
		// Try to release transport on failure so it doesn't pile up.
		_ = client.ReleaseTransport(ctx, trNumber)
		t.Fatalf("CreateObject: %v", err)
	}
	t.Logf("[2] created %s in Z_ADT_MCP_TEST (transport %s)", objName, trNumber)

	// 3. CheckTransport — verify the object is known to CTS
	check, err := client.CheckTransport(ctx, "R3TR", "PROG", objName)
	if err != nil {
		t.Fatalf("CheckTransport: %v", err)
	}
	t.Logf("[3] CheckTransport: result=%s recording=%v devclass=%s", check.Result, check.Recording, check.DevClass)
	if check.DevClass != "Z_ADT_MCP_TEST" {
		t.Errorf("expected DevClass Z_ADT_MCP_TEST, got %q", check.DevClass)
	}

	// 4. Delete the program (needs lock + transport)
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	err = client.DeleteObject(ctx, objectURI, lockHandle, trNumber)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		t.Fatalf("DeleteObject: %v", err)
	}
	t.Logf("[4] deleted %s", objName)

	// 5. Release the transport
	err = client.ReleaseTransport(ctx, trNumber)
	if err != nil {
		t.Fatalf("ReleaseTransport: %v", err)
	}
	t.Logf("[5] released transport %s", trNumber)
}
