package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestD010tabDeps(t *testing.T) {
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				if !strings.Contains(sql, "MASTER = 'SAPLZFUGR'") {
					t.Errorf("unexpected MASTER in D010TAB query: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{"SYST"}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{"SYST", "INTTAB"}},
				}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	deps, err := d010tabDeps(context.Background(), mock, "SAPLZFUGR", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].Name != "SYST" {
		t.Errorf("name: got %q, want SYST", deps[0].Name)
	}
	if deps[0].UseType != "STRUCTURE" {
		t.Errorf("use_type: got %q, want STRUCTURE", deps[0].UseType)
	}
}

func TestOoDeps(t *testing.T) {
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if !strings.Contains(sql, "SEOMETAREL") {
				t.Errorf("expected SEOMETAREL query, got: %s", sql)
				return nil, nil
			}
			if !strings.Contains(sql, "CLSNAME = 'ZCL_FOO'") {
				t.Errorf("expected CLSNAME filter, got: %s", sql)
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "REFCLSNAME"}, {Name: "RELTYPE"}},
				Rows: [][]string{
					{"ZIF_BAR", "1"},
					{"ZCL_PARENT", "2"},
				},
			}, nil
		},
	}
	deps, err := ooDeps(context.Background(), mock, "ZCL_FOO", []string{"1", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2", len(deps))
	}
	if deps[0].Name != "ZIF_BAR" || deps[0].UseType != "INTERFACE" {
		t.Errorf("dep[0]: got {%q, %q}, want {ZIF_BAR, INTERFACE}", deps[0].Name, deps[0].UseType)
	}
	if deps[1].Name != "ZCL_PARENT" || deps[1].UseType != "SUPERCLASS" {
		t.Errorf("dep[1]: got {%q, %q}, want {ZCL_PARENT, SUPERCLASS}", deps[1].Name, deps[1].UseType)
	}
}

// mockQueryClient is a minimal adt.QueryClient implementation for white-box tests
// that call package-level helpers directly.
type mockQueryClient struct {
	runQueryFn func(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error)
}

func (m *mockQueryClient) RunQuery(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
	if m.runQueryFn != nil {
		return m.runQueryFn(ctx, sql, maxRows)
	}
	return nil, nil
}
