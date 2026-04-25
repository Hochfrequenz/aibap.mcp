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

func (m *mockBlackMagic) UpdateCustomizing(context.Context, string, []tools.CustomizingEntry, string) error {
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
			return testTransportNum, nil
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

// Intentionally parallel to TestReleaseTransport_ElicitationAccepted — same
// structure exercised against a different destructive tool.
//
//nolint:dupl
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
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteTransportFn to be called after accept")
	}
	if gotTransport != testTransportNum {
		t.Errorf("transport: got %q, want %s", gotTransport, testTransportNum)
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
		"transport": testTransportNum,
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
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteTransportFn to be called with nil elicitor (backwards compat)")
	}
}

// Intentionally parallel to TestDeleteTransport_ElicitationAccepted — same
// structure exercised against a different destructive tool.
//
//nolint:dupl
func TestReleaseTransport_ElicitationAccepted(t *testing.T) {
	called := false
	var gotTransport string
	mock := &mockClient{
		releaseTransportFn: func(_ context.Context, transport string) error {
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
	result := callTool(t, s, "release_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected releaseTransportFn to be called after accept")
	}
	if gotTransport != testTransportNum {
		t.Errorf("transport: got %q, want %s", gotTransport, testTransportNum)
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestReleaseTransport_ElicitationDeclined(t *testing.T) {
	called := false
	mock := &mockClient{
		releaseTransportFn: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "release_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if !result.IsError {
		t.Fatal("expected error result when user declines")
	}
	if called {
		t.Fatal("releaseTransportFn must NOT be called when user declines")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "release_transport aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestReleaseTransport_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := &mockClient{
		releaseTransportFn: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "release_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected releaseTransportFn to be called with nil elicitor (backwards compat)")
	}
}

func removeFromTransportArgs() map[string]interface{} {
	return map[string]interface{}{
		"task_number":      "S4UK902001",
		"parent_transport": testTransportNum,
		"pgmid":            "R3TR",
		"object_type":      "PROG",
		"object_name":      "ZFOO",
		"wb_type":          "PROG/P",
		"position":         "000001",
	}
}

//nolint:dupl // Intentionally parallel to TestDeleteTransport_ElicitationAccepted.
func TestRemoveFromTransport_ElicitationAccepted(t *testing.T) {
	called := false
	mock := &mockClient{
		removeFromTransportFn: func(_ context.Context, _, _, _, _, _, _, _ string) error {
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
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "remove_from_transport", removeFromTransportArgs())
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected removeFromTransportFn to be called after accept")
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestRemoveFromTransport_ElicitationDeclined(t *testing.T) {
	called := false
	mock := &mockClient{
		removeFromTransportFn: func(_ context.Context, _, _, _, _, _, _, _ string) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "remove_from_transport", removeFromTransportArgs())
	if !result.IsError {
		t.Fatal("expected error result on decline")
	}
	if called {
		t.Fatal("removeFromTransportFn should NOT have been called on decline")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "remove_from_transport aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestRemoveFromTransport_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := &mockClient{
		removeFromTransportFn: func(_ context.Context, _, _, _, _, _, _, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "remove_from_transport", removeFromTransportArgs())
	if result.IsError {
		t.Fatalf("expected success with nil elicitor, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected removeFromTransportFn to be called with nil elicitor (backwards compat)")
	}
}
