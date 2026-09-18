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

// TestRunQuery_PurposeGateRejectsLocally covers both halves of the gate. The
// "present but unrecognised" case matters on its own: it is the only thing
// standing between a caller and a SELECT on a business table, and a handler
// narrowed to `purpose == ""` would still pass the missing-purpose case while
// letting every made-up value straight through.
func TestRunQuery_PurposeGateRejectsLocally(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "missing",
			args: map[string]interface{}{"sql": "SELECT * FROM VBAK"},
		},
		{
			name: "unrecognised",
			args: map[string]interface{}{"sql": "SELECT * FROM VBAK", "purpose": "reporting"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			mock := &mockClient{
				runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
					called = true
					return &adt.QueryResult{}, nil
				},
			}
			s := newTestServerWithFallback(mock, nil)
			result := callTool(t, s, "run_query", tc.args)
			if !result.IsError {
				t.Fatalf("expected a local rejection for a %s purpose", tc.name)
			}
			if called {
				t.Fatalf("RunQuery reached SAP with a %s purpose", tc.name)
			}
		})
	}
}
