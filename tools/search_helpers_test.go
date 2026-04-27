package tools

import (
	"context"
	"errors"
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

func TestDdicChainDeps_TABL(t *testing.T) {
	// Depth=1: TABL queries DD03L → discovers DTELs (ROLLNAME) and TABLs (CHECKTABLE).
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "DD03L"):
				if !strings.Contains(sql, "'SCARR'") {
					t.Errorf("DD03L filter missing SCARR, got: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ROLLNAME"}, {Name: "CHECKTABLE"}},
					Rows: [][]string{
						{"S_CARR_ID", ""}, // field with DTEL, no check table
						{"", "TCURC"},     // field without DTEL but with check table
						{"", ""},          // field with neither
					},
				}, nil
			case strings.Contains(sql, "DD02L") || strings.Contains(sql, "TADIR"):
				// classifyDDICObjects is called for TCURC (check table)
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{"TCURC", "TRANSP"}},
				}, nil
			default:
				// Depth=1 means no further BFS — no DD04L/DD01L queries expected.
				return nil, nil
			}
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "SCARR", "TABL", 1)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2", len(deps))
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.UseType
	}
	if byName["S_CARR_ID"] != useTypeDataElement {
		t.Errorf("S_CARR_ID: got %q, want DATA_ELEMENT", byName["S_CARR_ID"])
	}
	if byName["TCURC"] != useTypeTable {
		t.Errorf("TCURC: got %q, want TABLE", byName["TCURC"])
	}
}

func TestDdicChainDeps_DTEL(t *testing.T) {
	// Depth=1: DTEL queries DD04L → discovers DOMA.
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if !strings.Contains(sql, "DD04L") {
				t.Errorf("expected DD04L query, got: %s", sql)
				return nil, nil
			}
			if !strings.Contains(sql, "'S_CARR_ID'") {
				t.Errorf("DD04L missing S_CARR_ID, got: %s", sql)
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "DOMNAME"}},
				Rows:    [][]string{{"S_CARR_ID"}},
			}, nil
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "S_CARR_ID", "DTEL", 1)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].Name != "S_CARR_ID" || deps[0].UseType != useTypeDomain {
		t.Errorf("dep[0]: got {%q, %q}, want {S_CARR_ID, DOMAIN}", deps[0].Name, deps[0].UseType)
	}
}

func TestDdicChainDeps_DOMA(t *testing.T) {
	// Depth=1: DOMA queries DD01L → discovers ENTITYTAB.
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if !strings.Contains(sql, "DD01L") {
				t.Errorf("expected DD01L query, got: %s", sql)
				return nil, nil
			}
			if !strings.Contains(sql, "'MANDT'") {
				t.Errorf("DD01L missing MANDT, got: %s", sql)
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "ENTITYTAB"}},
				Rows:    [][]string{{"T000"}},
			}, nil
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "MANDT", "DOMA", 1)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].Name != "T000" || deps[0].UseType != useTypeTable {
		t.Errorf("dep[0]: got {%q, %q}, want {T000, TABLE}", deps[0].Name, deps[0].UseType)
	}
}

func TestDdicChainDeps_TTYP_E(t *testing.T) {
	// TTYP with ROWKIND='E' → direct DTEL, no classification needed.
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if !strings.Contains(sql, "DD40L") {
				t.Errorf("expected DD40L query, got: %s", sql)
				return nil, nil
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "ROWTYPE"}, {Name: "ROWKIND"}},
				Rows:    [][]string{{"S_CARR_ID", "E"}},
			}, nil
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "TT_CARR_IDS", "TTYP", 1)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].Name != "S_CARR_ID" || deps[0].UseType != useTypeDataElement {
		t.Errorf("dep[0]: got {%q, %q}, want {S_CARR_ID, DATA_ELEMENT}", deps[0].Name, deps[0].UseType)
	}
}

func TestDdicChainDeps_TTYP_S(t *testing.T) {
	// TTYP with ROWKIND='S' → classifyDDICObjects path.
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "DD40L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ROWTYPE"}, {Name: "ROWKIND"}},
					Rows:    [][]string{{"SCARR", "S"}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{"SCARR", "TRANSP"}},
				}, nil
			default:
				return nil, nil
			}
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "TT_SCARR", "TTYP", 1)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(deps) != 1 {
		t.Fatalf("got %d deps, want 1", len(deps))
	}
	if deps[0].Name != "SCARR" || deps[0].UseType != useTypeTable {
		t.Errorf("dep[0]: got {%q, %q}, want {SCARR, TABLE}", deps[0].Name, deps[0].UseType)
	}
}

func TestDdicChainDeps_CycleDetection(t *testing.T) {
	// Classic DDIC cycle: DOMA MANDT → ENTITYTAB T000 → field ROLLNAME S_MANDT (DTEL) → DOMNAME MANDT
	// The cycle closes at MANDT which is already visited.
	var queryCount int
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			queryCount++
			switch {
			case strings.Contains(sql, "DD01L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ENTITYTAB"}},
					Rows:    [][]string{{"T000"}},
				}, nil
			case strings.Contains(sql, "DD03L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ROLLNAME"}, {Name: "CHECKTABLE"}},
					Rows:    [][]string{{"S_MANDT", ""}},
				}, nil
			case strings.Contains(sql, "DD04L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "DOMNAME"}},
					Rows:    [][]string{{"MANDT"}},
				}, nil
			default:
				return nil, nil
			}
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "MANDT", "DOMA", 10)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	// Expected: T000 (TABLE), S_MANDT (DTEL) — MANDT itself is the root and not in output.
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2 (T000 + S_MANDT)", len(deps))
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.UseType
	}
	if byName["T000"] != useTypeTable {
		t.Errorf("T000: got %q, want TABLE", byName["T000"])
	}
	if byName["S_MANDT"] != useTypeDataElement {
		t.Errorf("S_MANDT: got %q, want DATA_ELEMENT", byName["S_MANDT"])
	}
	// MANDT must not appear in deps (it is the root, visited from the start).
	if _, ok := byName["MANDT"]; ok {
		t.Error("MANDT should not appear in deps (it is the root object)")
	}
}

func TestDdicChainDeps_MaxDepth(t *testing.T) {
	// depth=1 returns only T000; depth=2 also returns S_MANDT (from DD03L of T000).
	makeQuery := func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
		switch {
		case strings.Contains(sql, "DD01L"):
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "ENTITYTAB"}},
				Rows:    [][]string{{"T000"}},
			}, nil
		case strings.Contains(sql, "DD03L"):
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "ROLLNAME"}, {Name: "CHECKTABLE"}},
				Rows:    [][]string{{"S_MANDT", ""}},
			}, nil
		default:
			return nil, nil
		}
	}
	deps1, _ := ddicChainDeps(context.Background(), &mockQueryClient{runQueryFn: makeQuery}, "MANDT", "DOMA", 1)
	if len(deps1) != 1 {
		t.Errorf("depth=1: got %d deps, want 1 (T000 only)", len(deps1))
	}
	deps2, _ := ddicChainDeps(context.Background(), &mockQueryClient{runQueryFn: makeQuery}, "MANDT", "DOMA", 2)
	if len(deps2) != 2 {
		t.Errorf("depth=2: got %d deps, want 2 (T000 + S_MANDT)", len(deps2))
	}
}

func TestDdicChainDeps_QueryError_Warning(t *testing.T) {
	// A failed DD03L query produces a warning but does NOT return an error.
	mock := &mockQueryClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "DD03L") {
				return nil, errors.New("connection timeout")
			}
			return nil, nil
		},
	}
	deps, warns := ddicChainDeps(context.Background(), mock, "SCARR", "TABL", 1)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps on query error, got %d", len(deps))
	}
	if len(warns) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "DD03L query failed") {
		t.Errorf("warning should mention DD03L query failed, got: %q", warns[0])
	}
	if !strings.Contains(warns[0], "connection timeout") {
		t.Errorf("warning should include original error, got: %q", warns[0])
	}
}
