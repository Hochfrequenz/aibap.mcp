//go:build integration && transport

package adt_test

import (
	"context"
	"testing"
)

func TestCreateTransportTask_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Add a task for current user under Konstantin's existing transport
	taskNumber, err := client.CreateTransportTask(ctx, "S4UK902339", "MCP fixture cleanup")
	if err != nil {
		t.Fatalf("CreateTransportTask: %v", err)
	}
	t.Logf("Created task: %s under S4UK902339", taskNumber)

	if taskNumber == "" {
		t.Fatal("expected non-empty task number")
	}
}
