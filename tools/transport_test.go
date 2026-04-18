package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const testTransportNumber = "HFQK900001"

// mockBlackMagic implements tools.BlackMagicClient for testing.
type mockBlackMagic struct {
	createTransportFn func(ctx context.Context, category, target, description, devClass string) (string, error)
}

func (m *mockBlackMagic) ReleaseTransportFallback(context.Context, string) error {
	return nil
}

func (m *mockBlackMagic) UpdateCustomizing(context.Context, string, []tools.CustomizingEntry) error {
	return nil
}

func (m *mockBlackMagic) CreateObjectFallback(context.Context, string, string, string, string, string) error {
	return nil
}

func (m *mockBlackMagic) CreateTransportFallback(ctx context.Context, category, target, description, devClass string) (string, error) {
	if m.createTransportFn != nil {
		return m.createTransportFn(ctx, category, target, description, devClass)
	}
	return testTransportNumber, nil
}

func newTestServerWithFallback(client adt.Client, fallback tools.BlackMagicClient) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, nil)
	return s
}

func newTestServerWithFallbackElicitor(client adt.Client, fallback tools.BlackMagicClient, elicitor tools.Elicitor) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, elicitor)
	return s
}

func TestCreateTransport_CategoryW_NoFallback_ReturnsError(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "create_transport", map[string]interface{}{
		"category":    "W",
		"description": "Test customizing",
	})
	if !result.IsError {
		t.Fatal("expected error for category W without fallback")
	}
	text := result.Content[0].(mcp.TextContent).Text
	for _, want := range []string{"category W", "SAP limitation", "SE09"} {
		if !strings.Contains(text, want) {
			t.Errorf("error message should contain %q, got: %s", want, text)
		}
	}
}

func TestCreateTransport_CategoryW_WithFallback_UsesFallback(t *testing.T) {
	called := false
	fb := &mockBlackMagic{
		createTransportFn: func(_ context.Context, category, _, _, _ string) (string, error) {
			called = true
			if category != "W" {
				t.Errorf("expected category W, got %s", category)
			}
			return testTransportNumber, nil
		},
	}
	s := newTestServerWithFallback(&mockClient{}, fb)
	result := callTool(t, s, "create_transport", map[string]interface{}{
		"category":    "W",
		"description": "Test customizing",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected BlackMagic fallback to be called")
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &out)
	if out["transport_number"] != testTransportNumber {
		t.Errorf("expected HFQK900001, got %s", out["transport_number"])
	}
}

func TestCreateTransport_CategoryW_Force_UsesADT(t *testing.T) {
	adtCalled := false
	client := &mockClient{
		createTransportFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			adtCalled = true
			return "DEVK900123", nil
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_transport", map[string]interface{}{
		"category":    "W",
		"description": "Test forced",
		"force":       true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if !adtCalled {
		t.Fatal("expected ADT client to be called with force=true")
	}
}

func TestCreateTransport_CategoryK_UsesADT(t *testing.T) {
	adtCalled := false
	client := &mockClient{
		createTransportFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			adtCalled = true
			return "DEVK900456", nil
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_transport", map[string]interface{}{
		"category":    "K",
		"description": "Workbench transport",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if !adtCalled {
		t.Fatal("expected ADT client to be called for category K")
	}
}

func TestDeleteTransport_ElicitationAccepted(t *testing.T) {
	called := false
	var gotTransport string
	mock := &mockClient{
		deleteTransportFn: func(_ context.Context, transport string) error {
			called = true
			gotTransport = transport
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_transport", map[string]interface{}{
		"transport": "DEVK900123",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteTransportFn to be called after accept")
	}
	if gotTransport != "DEVK900123" {
		t.Errorf("transport: got %q, want DEVK900123", gotTransport)
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestDeleteTransport_ElicitationDeclined(t *testing.T) {
	called := false
	mock := &mockClient{
		deleteTransportFn: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_transport", map[string]interface{}{
		"transport": "DEVK900123",
	})
	if !result.IsError {
		t.Fatal("expected error result when user declines")
	}
	if called {
		t.Fatal("deleteTransportFn must NOT be called when user declines")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "delete_transport aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestDeleteTransport_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := &mockClient{
		deleteTransportFn: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "delete_transport", map[string]interface{}{
		"transport": "DEVK900123",
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteTransportFn to be called with nil elicitor (backwards compat)")
	}
}
