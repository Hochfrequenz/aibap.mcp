//go:build integration

package adt_test

import (
	"context"
	"testing"
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
