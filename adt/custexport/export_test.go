package custexport

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

// mockClient implements adt.Client for testing.
// Only RunQuery is wired; all other methods panic if called.
type mockClient struct {
	runQueryFn func(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error)
}

func (m *mockClient) RunQuery(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
	if m.runQueryFn != nil {
		return m.runQueryFn(ctx, sql, maxRows)
	}
	return &adt.QueryResult{}, nil
}

func (m *mockClient) GetSource(context.Context, string) (*adt.SourceResult, error) {
	panic("not implemented")
}
func (m *mockClient) SetSource(context.Context, string, string, string, string, string) (string, error) {
	panic("not implemented")
}
func (m *mockClient) ActivateObjects(context.Context, []string) (*adt.ActivationResult, error) {
	panic("not implemented")
}
func (m *mockClient) SearchObjects(context.Context, string, string, int) ([]adt.ObjectInfo, error) {
	panic("not implemented")
}
func (m *mockClient) WhereUsed(context.Context, string) ([]adt.ObjectInfo, error) {
	panic("not implemented")
}
func (m *mockClient) BrowsePackage(context.Context, string) ([]adt.ObjectInfo, error) {
	panic("not implemented")
}
func (m *mockClient) GetObjectInfo(context.Context, string) (*adt.ObjectInfo, error) {
	panic("not implemented")
}
func (m *mockClient) SyntaxCheck(context.Context, string) ([]adt.SyntaxMessage, error) {
	panic("not implemented")
}
func (m *mockClient) RunUnitTests(context.Context, string, int) (*adt.TestResult, error) {
	panic("not implemented")
}
func (m *mockClient) GetTransportRequests(context.Context, string, string) ([]adt.TransportRequest, error) {
	panic("not implemented")
}
func (m *mockClient) AddToTransport(context.Context, string, string) error {
	panic("not implemented")
}
func (m *mockClient) LockObject(context.Context, string) (string, error) {
	panic("not implemented")
}
func (m *mockClient) UnlockObject(context.Context, string, string) error {
	panic("not implemented")
}
func (m *mockClient) PrettyPrint(context.Context, string) (string, error) {
	panic("not implemented")
}
func (m *mockClient) CreateObject(context.Context, string, string, string, string, string) error {
	panic("not implemented")
}
func (m *mockClient) DeleteObject(context.Context, string, string, string) error {
	panic("not implemented")
}
func (m *mockClient) GetCompletions(context.Context, string, string, int, int) ([]adt.CompletionItem, error) {
	panic("not implemented")
}
func (m *mockClient) ExportPackage(context.Context, string) ([]byte, error) {
	panic("not implemented")
}
func (m *mockClient) GetATCCustomizing(context.Context) (*adt.ATCCustomizingResult, error) {
	panic("not implemented")
}
func (m *mockClient) RunATCCheck(context.Context, []string) (*adt.ATCResult, error) {
	panic("not implemented")
}
func (m *mockClient) SystemInfo() (string, string) {
	return "https://mock.example.com:443", "100"
}

func TestDiscoverTables(t *testing.T) {
	var capturedSQL string
	var capturedMaxRows int
	client := &mockClient{
		runQueryFn: func(_ context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
			capturedSQL = sql
			capturedMaxRows = maxRows
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "TABNAME", Type: "C"},
					{Name: "CONTFLAG", Type: "C"},
				},
				Rows: [][]string{
					{"T001", "C"},
					{"T002", "C"},
					{"ZTABLE", "G"},
				},
			}, nil
		},
	}

	tables, err := discoverTables(context.Background(), client)
	if err != nil {
		t.Fatalf("discoverTables: %v", err)
	}

	// Verify SQL.
	if !strings.Contains(capturedSQL, "DD02L") {
		t.Errorf("expected SQL to query DD02L, got: %s", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "CONTFLAG IN ('C','G')") {
		t.Errorf("expected SQL to filter CONTFLAG, got: %s", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "AS4LOCAL = 'A'") {
		t.Errorf("expected SQL to filter AS4LOCAL, got: %s", capturedSQL)
	}
	if capturedMaxRows != 200000 {
		t.Errorf("expected maxRows=200000, got %d", capturedMaxRows)
	}

	// Verify results.
	if len(tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(tables))
	}
	expected := []string{"T001", "T002", "ZTABLE"}
	for i, want := range expected {
		if tables[i] != want {
			t.Errorf("tables[%d]: expected %q, got %q", i, want, tables[i])
		}
	}
}

func TestFetchAllKeys(t *testing.T) {
	client := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if !strings.Contains(sql, "DD03L") {
				t.Errorf("expected SQL to query DD03L, got: %s", sql)
			}
			if !strings.Contains(sql, "'T001'") {
				t.Errorf("expected SQL to filter for T001, got: %s", sql)
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "FIELDNAME", Type: "C"},
					{Name: "POSITION", Type: "N"},
				},
				Rows: [][]string{
					{"MANDT", "0001"},
					{"BUKRS", "0002"},
				},
			}, nil
		},
	}

	keys, err := fetchTableKeys(context.Background(), client, "T001")
	if err != nil {
		t.Fatalf("fetchTableKeys: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("expected 2 keys for T001, got %d", len(keys))
	}
	if keys[0] != "MANDT" || keys[1] != "BUKRS" {
		t.Errorf("T001 keys: expected [MANDT BUKRS], got %v", keys)
	}
}

func TestFetchTableKeys_SkipsPseudoFields(t *testing.T) {
	client := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "FIELDNAME", Type: "C"},
					{Name: "POSITION", Type: "N"},
				},
				Rows: [][]string{
					{"MANDT", "0001"},
					{".INCLUDE", "0002"},
					{"GRDB_ITEM_SCEN", "0003"},
					{".APPEND", "0004"},
				},
			}, nil
		},
	}

	keys, err := fetchTableKeys(context.Background(), client, "SOMETABLE")
	if err != nil {
		t.Fatalf("fetchTableKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys (MANDT, GRDB_ITEM_SCEN), got %d: %v", len(keys), keys)
	}
	if keys[0] != "MANDT" || keys[1] != "GRDB_ITEM_SCEN" {
		t.Errorf("expected [MANDT GRDB_ITEM_SCEN], got %v", keys)
	}
}

func TestExportTable_SinglePage(t *testing.T) {
	client := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "MANDT", Type: "C", IsKey: true},
					{Name: "BUKRS", Type: "C", IsKey: true},
					{Name: "BUTXT", Type: "C", IsKey: false},
				},
				Rows: [][]string{
					{"100", "1000", "Company A"},
					{"100", "2000", "Company B"},
				},
			}, nil
		},
	}

	result, err := exportTable(context.Background(), client, "T001", []string{"MANDT", "BUKRS"}, 100)
	if err != nil {
		t.Fatalf("exportTable: %v", err)
	}

	if result.TableName != "T001" {
		t.Errorf("expected table T001, got %s", result.TableName)
	}
	if result.Pages != 1 {
		t.Errorf("expected 1 page, got %d", result.Pages)
	}
	if result.TotalRows != 2 {
		t.Errorf("expected 2 rows, got %d", result.TotalRows)
	}
	if len(result.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(result.Columns))
	}
}

func TestExportTable_ThreePages(t *testing.T) {
	pageSize := 2
	callCount := 0

	client := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			callCount++
			cols := []adt.QueryColumn{
				{Name: "MANDT", Type: "C", IsKey: true},
				{Name: "BUKRS", Type: "C", IsKey: true},
				{Name: "BUTXT", Type: "C", IsKey: false},
			}

			switch callCount {
			case 1:
				// First page: full (2 rows = pageSize).
				if strings.Contains(sql, "WHERE") {
					t.Error("first page should not have WHERE clause")
				}
				return &adt.QueryResult{
					Columns: cols,
					Rows: [][]string{
						{"100", "1000", "Company A"},
						{"100", "2000", "Company B"},
					},
				}, nil
			case 2:
				// Second page: full (2 rows = pageSize).
				if !strings.Contains(sql, "BUKRS > '2000'") {
					t.Errorf("second page WHERE should reference BUKRS > '2000', got: %s", sql)
				}
				return &adt.QueryResult{
					Columns: cols,
					Rows: [][]string{
						{"100", "3000", "Company C"},
						{"100", "4000", "Company D"},
					},
				}, nil
			case 3:
				// Third page: partial (1 row < pageSize).
				if !strings.Contains(sql, "BUKRS > '4000'") {
					t.Errorf("third page WHERE should reference BUKRS > '4000', got: %s", sql)
				}
				return &adt.QueryResult{
					Columns: cols,
					Rows: [][]string{
						{"100", "5000", "Company E"},
					},
				}, nil
			default:
				t.Fatal("unexpected fourth call to RunQuery")
				return nil, nil
			}
		},
	}

	result, err := exportTable(context.Background(), client, "T001", []string{"MANDT", "BUKRS"}, pageSize)
	if err != nil {
		t.Fatalf("exportTable: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 RunQuery calls, got %d", callCount)
	}
	if result.Pages != 3 {
		t.Errorf("expected 3 pages, got %d", result.Pages)
	}
	if result.TotalRows != 5 {
		t.Errorf("expected 5 total rows, got %d", result.TotalRows)
	}
	if len(result.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(result.Rows))
	}
	// Verify last row.
	if result.Rows[4][1] != "5000" {
		t.Errorf("expected last row BUKRS=5000, got %s", result.Rows[4][1])
	}
}

func TestExportTable_NoKeys(t *testing.T) {
	callCount := 0
	client := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			callCount++
			if strings.Contains(sql, "ORDER BY") {
				t.Error("no-key tables should not have ORDER BY")
			}
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "FIELD1", Type: "C"},
				},
				Rows: [][]string{{"value1"}, {"value2"}},
			}, nil
		},
	}

	result, err := exportTable(context.Background(), client, "T000", nil, 100)
	if err != nil {
		t.Fatalf("exportTable: %v", err)
	}

	// Should only make one call (no pagination without keys).
	if callCount != 1 {
		t.Errorf("expected 1 call for no-key table, got %d", callCount)
	}
	if result.TotalRows != 2 {
		t.Errorf("expected 2 rows, got %d", result.TotalRows)
	}
}

func TestExportTable_ErrorOnQuery(t *testing.T) {
	client := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}

	_, err := exportTable(context.Background(), client, "T001", []string{"MANDT", "BUKRS"}, 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestExtractKeyValues(t *testing.T) {
	columns := []adt.QueryColumn{
		{Name: "MANDT", Type: "C"},
		{Name: "BUKRS", Type: "C"},
		{Name: "GJAHR", Type: "N"},
		{Name: "BUTXT", Type: "C"},
	}
	row := []string{"100", "1000", "2025", "Test"}

	values := extractKeyValues(columns, []string{"BUKRS", "GJAHR"}, row)
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "1000" || values[1] != "2025" {
		t.Errorf("expected [1000 2025], got %v", values)
	}

	// Missing key returns nil.
	values = extractKeyValues(columns, []string{"BUKRS", "MISSING"}, row)
	if values != nil {
		t.Errorf("expected nil for missing key, got %v", values)
	}
}

func TestRunExport_EndToEnd(t *testing.T) {
	callCount := 0
	client := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			callCount++
			if strings.Contains(sql, "DD02L") {
				// discoverTables — not called since we provide tables.
				t.Error("should not query DD02L when tables provided")
			}
			if strings.Contains(sql, "DD03L") {
				// fetchAllKeys.
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{
						{Name: "TABNAME", Type: "C"},
						{Name: "FIELDNAME", Type: "C"},
						{Name: "POSITION", Type: "N"},
					},
					Rows: [][]string{
						{"T001", "MANDT", "0001"},
						{"T001", "BUKRS", "0002"},
					},
				}, nil
			}
			// Table export query.
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "MANDT", Type: "C", IsKey: true},
					{Name: "BUKRS", Type: "C", IsKey: true},
					{Name: "BUTXT", Type: "C", IsKey: false},
				},
				Rows: [][]string{
					{"100", "1000", "Test Company"},
				},
			}, nil
		},
	}

	dir := t.TempDir()
	cfg := ExportConfig{
		OutputDir: dir,
		Tables:    []string{"T001"},
		PageSize:  100,
		Workers:   1,
		System:    "https://mock.example.com:443",
		Client:    "100",
	}

	summary, err := RunExport(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("RunExport: %v", err)
	}

	if summary.TotalTables != 1 {
		t.Errorf("expected 1 total table, got %d", summary.TotalTables)
	}
	if summary.ExportedTables != 1 {
		t.Errorf("expected 1 exported table, got %d", summary.ExportedTables)
	}
	if summary.TotalRows != 1 {
		t.Errorf("expected 1 total row, got %d", summary.TotalRows)
	}
	if len(summary.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(summary.Errors))
	}
	if summary.Workers != 1 {
		t.Errorf("expected workers=1, got %d", summary.Workers)
	}
	if summary.System != "https://mock.example.com:443" {
		t.Errorf("expected system %q, got %q", "https://mock.example.com:443", summary.System)
	}
	if summary.Client != "100" {
		t.Errorf("expected client %q, got %q", "100", summary.Client)
	}
}
