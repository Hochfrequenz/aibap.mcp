//go:build integration && transport

package adt_test

import (
	"context"
	"regexp"
	"testing"
	"time"
)

// extractTransportFromError parses a transport number from SAP error messages
// like "already locked in request S4UK902592 of user ...".
func extractTransportFromError(err error) string {
	re := regexp.MustCompile(`(?:request|Auftrag)\s+([A-Z0-9]{10})`)
	if m := re.FindStringSubmatch(err.Error()); len(m) > 1 {
		return m[1]
	}
	return ""
}

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

	// Verify description is set (regression test for #226).
	info, err := client.GetTransportInfo(ctx, trNumber)
	if err != nil {
		t.Fatalf("GetTransportInfo: %v", err)
	}
	if info.Description == "" {
		t.Errorf("transport %s has empty description — REQUEST_TEXT not working", trNumber)
	} else {
		t.Logf("description: %q", info.Description)
	}
	if info.Status != "D" {
		t.Errorf("expected status D, got %q", info.Status)
	}
}

func TestReleaseTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP release test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport failed: %v", err)
	}
	t.Logf("created transport: %s", trNumber)

	err = client.ReleaseTransportWithTasks(ctx, trNumber)
	if err != nil {
		t.Fatalf("ReleaseTransportWithTasks failed: %v", err)
	}
	t.Logf("released transport: %s", trNumber)

	// Verify transport is actually released.
	// Release may be async — poll until status changes or timeout.
	released := false
	for i := 0; i < 6; i++ {
		info, err := client.GetTransportInfo(ctx, trNumber)
		if err != nil {
			t.Fatalf("GetTransportInfo: %v", err)
		}
		if info.Status == "L" || info.Status == "R" {
			t.Logf("verified: %s status=%s (released)", trNumber, info.Status)
			released = true
			break
		}
		t.Logf("status=%q, waiting for release to complete...", info.Status)
		time.Sleep(10 * time.Second)
	}
	if !released {
		t.Errorf("transport %s not released after polling", trNumber)
	}
}

func TestAddToTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP AddToTransport test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("created transport: %s", trNumber)
	t.Cleanup(func() { _ = client.ReleaseTransportWithTasks(context.Background(), trNumber) })

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

	// Verify object appears in transport object list.
	objects, err := client.GetTransportObjects(ctx, trNumber)
	if err != nil {
		t.Fatalf("GetTransportObjects: %v", err)
	}
	found := false
	for _, obj := range objects {
		if obj.Name == objName {
			found = true
			t.Logf("verified: %s found in transport (pgmid=%s type=%s)", obj.Name, obj.PgmID, obj.Type)
		}
	}
	if !found {
		t.Errorf("object %s not found in transport %s objects: %v", objName, trNumber, objects)
	}
}

// TestTransportFullCycle_Integration tests the complete transport lifecycle:
// 1. Create transport
// 2. Create a program in a real package (assigned to transport)
// 3. CheckTransport to verify the object is recordable
// 4. Activate the object (creates a version in VRSD)
// 5. Verify version history and retrieve historical source
// 6. Delete the program
// 7. Release the transport and verify status
func TestTransportFullCycle_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// 1. Create transport
	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP full cycle test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("[1] created transport: %s", trNumber)

	// Verify description is set.
	info, err := client.GetTransportInfo(ctx, trNumber)
	if err != nil {
		t.Fatalf("GetTransportInfo: %v", err)
	}
	if info.Description == "" {
		t.Errorf("transport %s has empty description", trNumber)
	}

	// 2. Create a program in a real package, assigned to this transport.
	// If it exists from a previous aborted run, clean it up first.
	const objName = "Z_ADT_MCP_FULLCYCLE"
	objectURI := "/sap/bc/adt/programs/programs/" + objName
	err = client.CreateObject(ctx, "PROG", objName, testPackage, "Full cycle test", trNumber)
	if err != nil {
		if _, infoErr := client.GetObjectInfo(ctx, objectURI); infoErr != nil {
			_ = client.ReleaseTransportWithTasks(ctx, trNumber)
			t.Fatalf("CreateObject failed and object does not exist: %v", err)
		}
		// Object exists from a previous aborted run. It may be locked in
		// another transport. Extract that transport number from the error
		// or from CheckTransport, then attach our own task to it so we can
		// work with the object (same as the SAP GUI dialog offers).
		lockingTR := extractTransportFromError(err)
		if lockingTR == "" {
			if chk, chkErr := client.CheckTransport(ctx, "R3TR", "PROG", objName); chkErr == nil && len(chk.Requests) > 0 {
				lockingTR = chk.Requests[0].Number
			}
		}
		if lockingTR != "" && lockingTR != trNumber {
			// Delete our empty transport — we'll use the existing one.
			_ = client.DeleteTransport(ctx, trNumber)
			trNumber = lockingTR
			// Create our own task on the existing transport.
			taskNr, taskErr := client.CreateTransportTask(ctx, trNumber, "", "Full cycle test cleanup")
			if taskErr != nil {
				t.Fatalf("[2] cannot create task on %s: %v", trNumber, taskErr)
			}
			t.Logf("[2] object %s locked in %s, created task %s", objName, trNumber, taskNr)
		} else {
			t.Logf("[2] object %s already exists, reusing with transport %s", objName, trNumber)
		}
	} else {
		t.Logf("[2] created %s in %s (transport %s)", objName, testPackage, trNumber)
	}

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

	// Release residual ENQUEUE lock from CreateObject/Activate.
	if lh, lockErr := client.LockObject(ctx, objectURI); lockErr == nil {
		_ = client.UnlockObject(ctx, objectURI, lh)
	}

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

	// Verify objects are in the transport before release.
	objects, err := client.GetTransportObjects(ctx, trNumber)
	if err != nil {
		t.Fatalf("GetTransportObjects: %v", err)
	}
	if len(objects) == 0 {
		t.Error("expected objects in transport before release, got none")
	}
	t.Logf("[5b] transport has %d object(s)", len(objects))
	foundObj := false
	for _, obj := range objects {
		t.Logf("     %s %s %s", obj.PgmID, obj.Type, obj.Name)
		if obj.Name == objName {
			foundObj = true
		}
	}
	if !foundObj {
		t.Errorf("expected %s in transport objects, not found", objName)
	}

	// 6. Delete the program (needs lock + transport)
	// Clear any residual ENQUEUE lock before re-locking for delete.
	if lh, lockErr := client.LockObject(ctx, objectURI); lockErr == nil {
		_ = client.UnlockObject(ctx, objectURI, lh)
	}
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

	// 7. Release the transport and verify status
	err = client.ReleaseTransportWithTasks(ctx, trNumber)
	if err != nil {
		t.Fatalf("ReleaseTransportWithTasks: %v", err)
	}
	t.Logf("[7] released transport %s", trNumber)

	released := false
	for i := 0; i < 6; i++ {
		info, err = client.GetTransportInfo(ctx, trNumber)
		if err != nil {
			t.Fatalf("GetTransportInfo after release: %v", err)
		}
		if info.Status == "L" || info.Status == "R" {
			t.Logf("[7] verified: %s status=%s (released)", trNumber, info.Status)
			released = true
			break
		}
		t.Logf("[7] status=%q, waiting...", info.Status)
		time.Sleep(10 * time.Second)
	}
	if !released {
		t.Errorf("transport %s not released after polling", trNumber)
	}
}
