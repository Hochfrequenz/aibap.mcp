package custexport

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	_ "modernc.org/sqlite"
)

func testColumns() []adt.QueryColumn {
	return []adt.QueryColumn{
		{Name: "MANDT", Type: "C", Description: "Client", IsKey: true},
		{Name: "BUKRS", Type: "C", Description: "Company Code", IsKey: true},
		{Name: "BUTXT", Type: "C", Description: "Company Name", IsKey: false},
	}
}

func testResult() *TableExportResult {
	return &TableExportResult{
		TableName: "T001",
		Columns:   testColumns(),
		Rows: [][]string{
			{"100", "1000", "Test Company"},
			{"100", "2000", "Other Company"},
		},
		TotalRows: 2,
		Pages:     1,
	}
}

func TestNewWriter_CreatesDBAndDir(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Verify customizing.db exists.
	dbPath := filepath.Join(dir, "customizing.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("customizing.db not created")
	}

	// Verify json/ directory exists.
	jsonDir := filepath.Join(dir, "json")
	info, err := os.Stat(jsonDir)
	if os.IsNotExist(err) {
		t.Error("json/ directory not created")
	} else if !info.IsDir() {
		t.Error("json/ is not a directory")
	}

	// Verify _metadata table exists.
	var name string
	err = w.sqlite.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='_metadata'`).Scan(&name)
	if err != nil {
		t.Fatalf("_metadata table not found: %v", err)
	}
}

func TestWriteTable_WithRows(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	result := testResult()
	if err := w.WriteTable(result); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	// Verify SQLite rows.
	var count int
	if err := w.sqlite.db.QueryRow(`SELECT COUNT(*) FROM "T001"`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	// Verify a specific row.
	var butxt string
	err = w.sqlite.db.QueryRow(`SELECT "BUTXT" FROM "T001" WHERE "BUKRS" = '1000'`).Scan(&butxt)
	if err != nil {
		t.Fatalf("query row: %v", err)
	}
	if butxt != "Test Company" {
		t.Errorf("expected 'Test Company', got %q", butxt)
	}

	// Verify _metadata entries.
	var metaCount int
	err = w.sqlite.db.QueryRow(`SELECT COUNT(*) FROM "_metadata" WHERE "table_name" = 'T001'`).Scan(&metaCount)
	if err != nil {
		t.Fatalf("count metadata: %v", err)
	}
	if metaCount != 3 {
		t.Errorf("expected 3 metadata rows, got %d", metaCount)
	}

	// Verify metadata content.
	var isKey int
	err = w.sqlite.db.QueryRow(`SELECT "is_key" FROM "_metadata" WHERE "table_name" = 'T001' AND "column_name" = 'MANDT'`).Scan(&isKey)
	if err != nil {
		t.Fatalf("query metadata: %v", err)
	}
	if isKey != 1 {
		t.Errorf("expected MANDT is_key=1, got %d", isKey)
	}

	// Verify JSON file.
	jsonPath := filepath.Join(dir, "json", "T001.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}

	var parsed jsonTable
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if parsed.Table != "T001" {
		t.Errorf("json table name: expected T001, got %s", parsed.Table)
	}
	if parsed.TotalRows != 2 {
		t.Errorf("json total_rows: expected 2, got %d", parsed.TotalRows)
	}
	if len(parsed.Rows) != 2 {
		t.Errorf("json rows count: expected 2, got %d", len(parsed.Rows))
	}
	if parsed.Rows[0]["BUKRS"] != "1000" {
		t.Errorf("json row[0] BUKRS: expected 1000, got %s", parsed.Rows[0]["BUKRS"])
	}
}

func TestWriteTable_EmptyTable(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	result := &TableExportResult{
		TableName: "T002",
		Columns:   testColumns(),
		Rows:      nil,
		TotalRows: 0,
		Pages:     0,
	}

	if err := w.WriteTable(result); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	// Verify table exists in SQLite.
	var name string
	err = w.sqlite.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='T002'`).Scan(&name)
	if err != nil {
		t.Fatalf("table T002 not found: %v", err)
	}

	// Verify no rows.
	var count int
	if err := w.sqlite.db.QueryRow(`SELECT COUNT(*) FROM "T002"`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}

	// Verify JSON file exists with empty rows.
	data, err := os.ReadFile(filepath.Join(dir, "json", "T002.json"))
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var parsed jsonTable
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Rows) != 0 {
		t.Errorf("expected 0 rows in json, got %d", len(parsed.Rows))
	}
}

func TestWriteTable_NamespaceTable(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	result := &TableExportResult{
		TableName: "/HFQ/TABLE",
		Columns: []adt.QueryColumn{
			{Name: "MANDT", Type: "C", Description: "Client", IsKey: true},
			{Name: "VALUE", Type: "C", Description: "Value", IsKey: false},
		},
		Rows:      [][]string{{"100", "test"}},
		TotalRows: 1,
		Pages:     1,
	}

	if err := w.WriteTable(result); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	// Verify JSON filename uses # instead of /.
	jsonPath := filepath.Join(dir, "json", "#HFQ#TABLE.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("namespace JSON file not found at expected path with # replacements")
	}

	// Verify SQLite table with namespace name.
	var count int
	if err := w.sqlite.db.QueryRow(`SELECT COUNT(*) FROM "/HFQ/TABLE"`).Scan(&count); err != nil {
		t.Fatalf("query namespace table: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestWriteTable_PrimaryKey(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer func() { _ = w.Close() }()

	result := testResult()
	if err := w.WriteTable(result); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	// Inserting a duplicate key should fail.
	_, err = w.sqlite.db.Exec(`INSERT INTO "T001" ("MANDT", "BUKRS", "BUTXT") VALUES ('100', '1000', 'Duplicate')`)
	if err == nil {
		t.Error("expected PRIMARY KEY violation, got nil error")
	}
}

func TestWriteTable_PersistsAfterClose(t *testing.T) {
	dir := t.TempDir()

	// Write data and close.
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	result := testResult()
	if err := w.WriteTable(result); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify.
	dbPath := filepath.Join(dir, "customizing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "T001"`).Scan(&count); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows after reopen, got %d", count)
	}

	var metaCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "_metadata" WHERE "table_name" = 'T001'`).Scan(&metaCount); err != nil {
		t.Fatalf("count metadata after reopen: %v", err)
	}
	if metaCount != 3 {
		t.Errorf("expected 3 metadata rows after reopen, got %d", metaCount)
	}
}
