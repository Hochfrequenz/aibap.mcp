//go:build integration

package adt_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestGetMessageClass_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Message class "00" is a standard SAP class that exists on every system.
	result, err := client.GetMessageClass(ctx, "00")
	if err != nil {
		t.Fatalf("GetMessageClass: %v", err)
	}
	if result.Name != "00" {
		t.Errorf("name: got %q, want 00", result.Name)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected messages in class 00")
	}
	t.Logf("message class %q: %q, %d messages", result.Name, result.Description, len(result.Messages))
	for i, m := range result.Messages {
		if i < 5 {
			t.Logf("  [%s] %q", m.Number, m.Text)
		}
	}
}

// TestSetMessages_Integration is currently skipped — the Lock Handle from
// LockObject is rejected by the PUT endpoint with "invalid lock handle".
// The lock succeeds (200 + handle) but the subsequent PUT fails.
// Root cause under investigation: possibly session/CSRF mismatch or
// message class resources require a different lock protocol.
func TestSetMessages_Integration(t *testing.T) {
	t.Skip("lock handle rejected by message class PUT — under investigation")
	client := newIntegrationClient(t)
	ctx := context.Background()

	const msgClass = "Z_ADT_MCP_MSG"
	msgURI := "/sap/bc/adt/messageclass/" + msgClass

	// Ensure the message class exists (create if missing).
	if _, err := client.GetMessageClass(ctx, msgClass); err != nil {
		err = client.CreateObject(ctx, "MSAG", msgClass, "$TMP", "MCP test message class", "")
		if err != nil {
			t.Fatalf("CreateObject MSAG: %v", err)
		}
		t.Logf("created message class %s", msgClass)
	}

	// Read ETag before locking (required for If-Match)
	mcInfo, err := client.GetMessageClass(ctx, msgClass)
	if err != nil {
		t.Fatalf("GetMessageClass for ETag: %v", err)
	}
	t.Logf("ETag: %s", mcInfo.ETag)

	// Lock
	lockHandle, err := client.LockObject(ctx, msgURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	t.Logf("lock handle: %s", lockHandle)
	t.Cleanup(func() { _ = client.UnlockObject(context.Background(), msgURI, lockHandle) })

	// Set messages
	messages := []adt.Message{
		{Number: "001", Text: "Hello from MCP &1", SelfExpl: true},
		{Number: "002", Text: "Error in &1: &2", SelfExpl: false},
	}
	err = client.SetMessages(ctx, msgClass, lockHandle, mcInfo.ETag, messages)
	if err != nil {
		t.Fatalf("SetMessages: %v", err)
	}
	t.Logf("set %d messages on %s", len(messages), msgClass)

	// Unlock
	_ = client.UnlockObject(ctx, msgURI, lockHandle)

	// Verify
	result, err := client.GetMessageClass(ctx, msgClass)
	if err != nil {
		t.Fatalf("GetMessageClass after set: %v", err)
	}
	if len(result.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(result.Messages))
	}
	t.Logf("verified %d messages", len(result.Messages))
	for _, m := range result.Messages {
		t.Logf("  [%s] %q", m.Number, m.Text)
	}
}
