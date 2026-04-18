package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

const testRenameURI = "/sap/bc/adt/programs/programs/Z/source/main#start=5,7"

func TestRename_ElicitationAccepted(t *testing.T) {
	called := false
	var gotURI, gotNewName string
	mock := &mockClient{
		renameFn: func(_ context.Context, uri, newName, _ string) (*adt.RenameResult, error) {
			called = true
			gotURI = uri
			gotNewName = newName
			return &adt.RenameResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "rename", map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected renameFn to be called after accept")
	}
	if gotURI != testRenameURI {
		t.Errorf("source_uri: got %q, want %q", gotURI, testRenameURI)
	}
	if gotNewName != "NEW_SYM" {
		t.Errorf("new_name: got %q, want NEW_SYM", gotNewName)
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestRename_ElicitationDeclined(t *testing.T) {
	called := false
	mock := &mockClient{
		renameFn: func(_ context.Context, _, _, _ string) (*adt.RenameResult, error) {
			called = true
			return &adt.RenameResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "rename", map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	})
	if !result.IsError {
		t.Fatal("expected error result when user declines")
	}
	if called {
		t.Fatal("renameFn must NOT be called when user declines")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "rename aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestRename_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := &mockClient{
		renameFn: func(_ context.Context, _, _, _ string) (*adt.RenameResult, error) {
			called = true
			return &adt.RenameResult{}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "rename", map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected renameFn to be called with nil elicitor (backwards compat)")
	}
}
