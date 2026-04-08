package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)

type mockBlackMagicCust struct {
	updateCustomizingFn func(ctx context.Context, table string, entries []tools.CustomizingEntry) error
}

func (m *mockBlackMagicCust) ReleaseTransportFallback(context.Context, string) error {
	return nil
}

func (m *mockBlackMagicCust) CreateTransportFallback(context.Context, string, string, string, string) (string, error) {
	return "", nil
}

func (m *mockBlackMagicCust) UpdateCustomizing(ctx context.Context, table string, entries []tools.CustomizingEntry) error {
	if m.updateCustomizingFn != nil {
		return m.updateCustomizingFn(ctx, table, entries)
	}
	return nil
}

func newTestServerWithCustFallback(client adt.Client, fallback tools.BlackMagicClient) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback)
	return s
}

func TestUpdateCustomizing_NoFallback_ReturnsError(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "update_customizing", map[string]interface{}{
		"table": "V_T077D",
		"entries": []map[string]interface{}{
			{"keys": map[string]interface{}{"BUKRS": "1000"}, "values": map[string]interface{}{"BUTXT": "Test"}},
		},
	})
	if !result.IsError {
		t.Fatal("expected error without BlackMagic fallback")
	}
}

func TestUpdateCustomizing_WithFallback_Succeeds(t *testing.T) {
	called := false
	fb := &mockBlackMagicCust{
		updateCustomizingFn: func(_ context.Context, table string, entries []tools.CustomizingEntry) error {
			called = true
			if table != "V_T077D" {
				t.Errorf("expected table V_T077D, got %s", table)
			}
			if len(entries) != 1 {
				t.Errorf("expected 1 entry, got %d", len(entries))
			}
			return nil
		},
	}
	s := newTestServerWithCustFallback(&mockClient{}, fb)
	result := callTool(t, s, "update_customizing", map[string]interface{}{
		"table": "V_T077D",
		"entries": []map[string]interface{}{
			{"keys": map[string]interface{}{"BUKRS": "1000"}, "values": map[string]interface{}{"BUTXT": "Test"}},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected BlackMagic fallback to be called")
	}
}

func TestUpdateCustomizing_NoEntries_ReturnsError(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "update_customizing", map[string]interface{}{
		"table":   "V_T077D",
		"entries": []map[string]interface{}{},
	})
	if !result.IsError {
		t.Fatal("expected error when no entries provided")
	}
}
