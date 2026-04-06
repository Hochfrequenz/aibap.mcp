//go:build integration && transport

package adt_test

import (
	"context"
	"testing"
)

// TestRemoveFromTransport_Integration verifies that RemoveFromTransport
// removes an object entry from a transport task via PUT with TM XML body.
// Endpoint verification for #193.
func TestRemoveFromTransport_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// 1. Create transport
	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "MCP RemoveFromTransport test", testPackage)
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("[1] created transport: %s", trNumber)
	t.Cleanup(func() { _ = client.ReleaseTransportWithTasks(context.Background(), trNumber) })

	// 2. Create a program assigned to the transport
	const objName = "Z_ADT_MCP_RMFTR"
	err = client.CreateObject(ctx, "PROG", objName, testPackage, "RemoveFromTransport test", trNumber)
	if err != nil {
		t.Fatalf("[2] CreateObject: %v", err)
	}
	t.Logf("[2] created %s", objName)

	// 3. Find task and verify object is in transport
	tasks, err := client.GetTransportTasks(ctx, trNumber)
	if err != nil || len(tasks) == 0 {
		t.Fatalf("[3] GetTransportTasks: %v (tasks=%v)", err, tasks)
	}
	taskNumber := tasks[0]
	t.Logf("[3] task: %s", taskNumber)

	objects, err := client.GetTransportObjects(ctx, trNumber)
	if err != nil {
		t.Fatalf("[3] GetTransportObjects: %v", err)
	}
	var obj *struct{ wbType, position string }
	for _, o := range objects {
		if o.Name == objName {
			obj = &struct{ wbType, position string }{o.WBType, o.Position}
			t.Logf("[3] found: pgmid=%s type=%s name=%s wbtype=%s pos=%s", o.PgmID, o.Type, o.Name, o.WBType, o.Position)
		}
	}
	if obj == nil {
		t.Fatalf("[3] object %s not found in transport", objName)
	}

	// 4. Remove the object
	err = client.RemoveFromTransport(ctx, taskNumber, trNumber, "R3TR", "PROG", objName, obj.wbType, obj.position)
	if err != nil {
		t.Fatalf("[4] RemoveFromTransport: %v", err)
	}
	t.Logf("[4] RemoveFromTransport succeeded")

	// 5. Verify removal
	objects, err = client.GetTransportObjects(ctx, trNumber)
	if err != nil {
		t.Fatalf("[5] GetTransportObjects: %v", err)
	}
	for _, o := range objects {
		if o.Name == objName {
			t.Fatalf("[5] object %s still in transport after removal", objName)
		}
	}
	t.Logf("[5] verified: %s removed from transport", objName)
}
