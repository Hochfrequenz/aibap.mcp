package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestRunQuery_ValidPurpose_CallsRunQuery(t *testing.T) {
	// A valid purpose must reach SAP without any further gate.
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
	s := newTestServerWithFallback(mock, nil)
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

func TestRunQuery_MissingPurpose_IsRejectedLocally(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql": "SELECT * FROM VBAK",
	})
	if !result.IsError {
		t.Fatal("expected a local rejection when purpose is missing")
	}
	if called {
		t.Fatal("RunQuery must not be called on hard block")
	}
}
