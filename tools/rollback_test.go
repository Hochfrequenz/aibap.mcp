package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// emptyTransportObjectsMock makes `rollback_transport` short-circuit quickly
// by returning an empty object list — enough to exercise the elicitation
// gate without driving the full rollback pipeline.
func emptyTransportObjectsMock(calledGate *bool) *mockClient {
	return &mockClient{
		getTransportObjectsFn: func(_ context.Context, _ string) ([]adt.TransportObject, error) {
			if calledGate != nil {
				*calledGate = true
			}
			return nil, nil
		},
	}
}

func TestRollbackTransport_ElicitationAccepted(t *testing.T) {
	called := false
	mock := emptyTransportObjectsMock(&called)
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "rollback_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected getTransportObjectsFn to be called after accept")
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestRollbackTransport_ElicitationDeclined(t *testing.T) {
	called := false
	mock := emptyTransportObjectsMock(&called)
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "rollback_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if !result.IsError {
		t.Fatal("expected error result on decline")
	}
	if called {
		t.Fatal("getTransportObjectsFn should NOT have been called on decline")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "rollback_transport aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestRollbackTransport_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := emptyTransportObjectsMock(&called)
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "rollback_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected getTransportObjectsFn to be called with nil elicitor (backwards compat)")
	}
}
