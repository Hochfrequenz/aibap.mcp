package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunQuery_ValidPurpose_CallsRunQuery(t *testing.T) {
	// nil elicitor is intentional: valid purpose must bypass elicitor entirely.
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			called = true
			if sql != "SELECT * FROM DD01L" {
				t.Errorf("unexpected sql: %q", sql)
			}
			return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "DOMNAME"}}, Rows: [][]string{{"CHAR10"}}}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql":     "SELECT * FROM DD01L",
		"purpose": "ddic_inspection",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("RunQuery was not called")
	}
}

func TestRunQuery_MissingPurpose_ElicitorAccepts_CallsRunQuery(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql": "SELECT * FROM DD01L",
	})
	if result.IsError {
		t.Fatalf("expected success after elicitor accept, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("RunQuery was not called after elicitor accept")
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitor call, got %d", el.called)
	}
}

func TestRunQuery_InvalidPurpose_ElicitorDeclines_ReturnsError(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql":     "SELECT * FROM VBAK",
		"purpose": "reporting",
	})
	if !result.IsError {
		t.Fatal("expected error when elicitor declines")
	}
	if called {
		t.Fatal("RunQuery must not be called when elicitor declines")
	}
}

func TestRunQuery_MissingPurpose_NilElicitor_HardBlock(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql": "SELECT * FROM VBAK",
	})
	if !result.IsError {
		t.Fatal("expected hard block when elicitor is nil and purpose is missing")
	}
	if called {
		t.Fatal("RunQuery must not be called on hard block")
	}
}
