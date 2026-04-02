//go:build integration && transport

package adt_test

import (
	"context"
	"testing"
)

func TestCreateAndDeleteTransportTask_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Create a transport to test task operations
	trNumber, err := client.CreateTransport(ctx, "K", "DUM", "task lifecycle test", "Z_ADT_MCP_TEST")
	if err != nil {
		t.Fatalf("CreateTransport: %v", err)
	}
	t.Logf("Created transport: %s", trNumber)

	// Create a task under the transport
	taskNumber, err := client.CreateTransportTask(ctx, trNumber, "Integration test task")
	if err != nil {
		t.Fatalf("CreateTransportTask: %v", err)
	}
	t.Logf("Created task: %s under %s", taskNumber, trNumber)

	if taskNumber == "" {
		t.Fatal("expected non-empty task number")
	}
	if taskNumber == trNumber {
		t.Error("task number should differ from parent transport number")
	}

	// Delete the task
	if err := client.DeleteTransport(ctx, taskNumber); err != nil {
		t.Fatalf("DeleteTransport (task): %v", err)
	}
	t.Logf("Deleted task: %s", taskNumber)

	// Delete the transport
	if err := client.DeleteTransport(ctx, trNumber); err != nil {
		t.Fatalf("DeleteTransport (request): %v", err)
	}
	t.Logf("Deleted transport: %s", trNumber)
}
