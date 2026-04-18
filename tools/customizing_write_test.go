package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
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

func (m *mockBlackMagicCust) CreateObjectFallback(context.Context, string, string, string, string, string) error {
	return nil
}

func (m *mockBlackMagicCust) UpdateCustomizing(ctx context.Context, table string, entries []tools.CustomizingEntry) error {
	if m.updateCustomizingFn != nil {
		return m.updateCustomizingFn(ctx, table, entries)
	}
	return nil
}

func newTestServerWithCustFallback(client adt.Client, fallback tools.BlackMagicClient) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, nil)
	return s
}

func newTestServerWithCustFallbackElicitor(client adt.Client, fallback tools.BlackMagicClient, elicitor tools.Elicitor) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, elicitor)
	return s
}

func customizingArgs() map[string]interface{} {
	return map[string]interface{}{
		"table": "V_T077D",
		"entries": []map[string]interface{}{
			{"keys": map[string]interface{}{"BUKRS": "1000"}, "values": map[string]interface{}{"BUTXT": "Test"}},
		},
	}
}

func TestUpdateCustomizing_ElicitationAccepted(t *testing.T) {
	called := false
	fb := &mockBlackMagicCust{
		updateCustomizingFn: func(_ context.Context, _ string, _ []tools.CustomizingEntry) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithCustFallbackElicitor(&mockClient{}, fb, el)
	result := callTool(t, s, "update_customizing", customizingArgs())
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected updateCustomizingFn to be called after accept")
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestUpdateCustomizing_ElicitationDeclined(t *testing.T) {
	called := false
	fb := &mockBlackMagicCust{
		updateCustomizingFn: func(_ context.Context, _ string, _ []tools.CustomizingEntry) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithCustFallbackElicitor(&mockClient{}, fb, el)
	result := callTool(t, s, "update_customizing", customizingArgs())
	if !result.IsError {
		t.Fatal("expected error result on decline")
	}
	if called {
		t.Fatal("updateCustomizingFn should NOT have been called on decline")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "update_customizing aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestUpdateCustomizing_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	fb := &mockBlackMagicCust{
		updateCustomizingFn: func(_ context.Context, _ string, _ []tools.CustomizingEntry) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithCustFallbackElicitor(&mockClient{}, fb, nil)
	result := callTool(t, s, "update_customizing", customizingArgs())
	if result.IsError {
		t.Fatalf("expected success with nil elicitor, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected updateCustomizingFn to be called with nil elicitor (backwards compat)")
	}
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
