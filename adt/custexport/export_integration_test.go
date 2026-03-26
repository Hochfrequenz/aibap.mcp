//go:build integration

package custexport_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/adt/custexport"
	"github.com/Hochfrequenz/mcp-server-abap/config"

	_ "modernc.org/sqlite"
)

// newClient creates a real ADT client from environment variables.
// Inline version of adt_test.newIntegrationClient since we cannot
// import test helpers across packages.
func newClient(t *testing.T) adt.Client {
	t.Helper()
	host := strings.TrimSpace(os.Getenv("SAP_INTEGRATION_HOST"))
	if host == "" {
		t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
	}
	user := strings.TrimSpace(os.Getenv("SAP_INTEGRATION_USER"))
	if user == "" {
		t.Fatal("SAP_INTEGRATION_USER must be set when SAP_INTEGRATION_HOST is set")
	}
	password := os.Getenv("SAP_INTEGRATION_PASSWORD")
	if password == "" {
		t.Fatal("SAP_INTEGRATION_PASSWORD must be set when SAP_INTEGRATION_HOST is set")
	}
	return adt.NewClient(config.SAPConfig{
		Host:          host,
		User:          user,
		Password:      password,
		Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
		TLSSkipVerify: true,
	})
}

func TestExportCustomizing_SmallTableSet(t *testing.T) {
	client := newClient(t)
	ctx := context.Background()
	outputDir := t.TempDir()

	tables := []string{"T001", "T005", "T006", "TVARVC", "T000"}

	summary, err := custexport.RunExport(ctx, client, custexport.ExportConfig{
		OutputDir: outputDir,
		Tables:    tables,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	// Verify no errors in summary.
	if len(summary.Errors) > 0 {
		for _, e := range summary.Errors {
			t.Errorf("export error for %s: %s", e.Table, e.Error)
		}
		t.FailNow()
	}

	// Verify customizing.db exists.
	dbPath := filepath.Join(outputDir, "customizing.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("customizing.db not found: %v", err)
	}

	// Verify json/ directory has one JSON file per table.
	jsonDir := filepath.Join(outputDir, "json")
	entries, err := os.ReadDir(jsonDir)
	if err != nil {
		t.Fatalf("reading json dir: %v", err)
	}
	jsonFiles := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles++
		}
	}
	if jsonFiles != len(tables) {
		t.Errorf("expected %d JSON files, got %d", len(tables), jsonFiles)
	}

	// Open SQLite DB and verify _metadata has entries for all tables.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	for _, table := range tables {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM "_metadata" WHERE "table_name" = ?`, table).Scan(&count)
		if err != nil {
			t.Errorf("querying _metadata for %s: %v", table, err)
			continue
		}
		if count == 0 {
			t.Errorf("no _metadata entries for table %s", table)
		}
	}

	// Verify T001 table exists in SQLite and has rows.
	var t001Count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "T001"`).Scan(&t001Count); err != nil {
		t.Fatalf("querying T001 row count: %v", err)
	}
	if t001Count == 0 {
		t.Error("T001 has 0 rows in SQLite, expected at least 1")
	}
	t.Logf("T001 has %d rows", t001Count)

	// Verify export_summary.json exists and has correct counts.
	summaryPath := filepath.Join(outputDir, "export_summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("reading export_summary.json: %v", err)
	}

	var fileSummary custexport.ExportSummary
	if err := json.Unmarshal(summaryData, &fileSummary); err != nil {
		t.Fatalf("parsing export_summary.json: %v", err)
	}
	if fileSummary.TotalTables != len(tables) {
		t.Errorf("summary total_tables: got %d, want %d", fileSummary.TotalTables, len(tables))
	}
	if fileSummary.ExportedTables+fileSummary.EmptyTables != len(tables) {
		t.Errorf("exported(%d) + empty(%d) != total(%d)",
			fileSummary.ExportedTables, fileSummary.EmptyTables, len(tables))
	}

	t.Logf("export summary: %d tables, %d exported, %d empty, %d total rows",
		fileSummary.TotalTables, fileSummary.ExportedTables, fileSummary.EmptyTables, fileSummary.TotalRows)
}

func TestExportCustomizing_EmptyTable(t *testing.T) {
	client := newClient(t)
	ctx := context.Background()
	outputDir := t.TempDir()

	// TPARA is a small customizing table that may be empty on test systems.
	// We export it alongside T000 (which is never empty) to verify the
	// writer handles empty tables correctly regardless.
	tables := []string{"T000", "TPARA"}

	summary, err := custexport.RunExport(ctx, client, custexport.ExportConfig{
		OutputDir: outputDir,
		Tables:    tables,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	dbPath := filepath.Join(outputDir, "customizing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Both tables should exist in SQLite (schema created even if empty).
	for _, table := range tables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&count)
		if err != nil {
			t.Errorf("checking existence of table %s: %v", table, err)
			continue
		}
		if count == 0 {
			t.Errorf("table %s was not created in SQLite", table)
		}
	}

	// Both tables should have JSON files.
	for _, table := range tables {
		jsonPath := filepath.Join(outputDir, "json", table+".json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			t.Errorf("reading %s.json: %v", table, err)
			continue
		}

		var jt struct {
			Rows []json.RawMessage `json:"rows"`
		}
		if err := json.Unmarshal(data, &jt); err != nil {
			t.Errorf("parsing %s.json: %v", table, err)
			continue
		}
		t.Logf("%s.json has %d rows", table, len(jt.Rows))
	}

	// T000 should always have rows.
	var t000Count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "T000"`).Scan(&t000Count); err != nil {
		t.Fatalf("querying T000 row count: %v", err)
	}
	if t000Count == 0 {
		t.Error("T000 has 0 rows, expected at least 1")
	}

	// Log which tables were empty for diagnostic purposes.
	t.Logf("summary: exported=%d, empty=%d, errors=%d",
		summary.ExportedTables, summary.EmptyTables, len(summary.Errors))
}

func TestExportCustomizing_Pagination(t *testing.T) {
	client := newClient(t)
	ctx := context.Background()
	outputDir := t.TempDir()

	// T006 (units of measurement) typically has ~410 rows.
	// Using page_size=100 forces multiple pages.
	tables := []string{"T006"}
	pageSize := 100

	summary, err := custexport.RunExport(ctx, client, custexport.ExportConfig{
		OutputDir: outputDir,
		Tables:    tables,
		PageSize:  pageSize,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}
	if len(summary.Errors) > 0 {
		t.Fatalf("export had errors: %v", summary.Errors)
	}

	// Verify all rows landed in SQLite (not just the first page).
	dbPath := filepath.Join(outputDir, "customizing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "T006"`).Scan(&rowCount); err != nil {
		t.Fatalf("querying T006 row count: %v", err)
	}
	t.Logf("T006: %d rows in SQLite", rowCount)

	if rowCount <= pageSize {
		t.Errorf("T006 has %d rows, expected more than %d (page_size) to verify pagination", rowCount, pageSize)
	}

	// Verify the JSON file shows pages > 1.
	jsonPath := filepath.Join(outputDir, "json", "T006.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading T006.json: %v", err)
	}

	var jt struct {
		TotalRows int `json:"total_rows"`
		Pages     int `json:"pages"`
	}
	if err := json.Unmarshal(data, &jt); err != nil {
		t.Fatalf("parsing T006.json: %v", err)
	}

	if jt.Pages <= 1 {
		t.Errorf("T006 JSON pages=%d, expected >1 with page_size=%d", jt.Pages, pageSize)
	}
	if jt.TotalRows != rowCount {
		t.Errorf("JSON total_rows=%d != SQLite count=%d", jt.TotalRows, rowCount)
	}

	t.Logf("T006: %d rows across %d pages (page_size=%d)", jt.TotalRows, jt.Pages, pageSize)
}

func TestExportCustomizing_IncludeTables(t *testing.T) {
	client := newClient(t)
	ctx := context.Background()
	outputDir := t.TempDir()

	// /US4G/BITCAT_RD and /US4G/CDLIST_D have .INCLUDE pseudo-fields in DD03L
	// that previously caused "invalid key column" errors. Verify they export now.
	tables := []string{"/US4G/BITCAT_RD", "/US4G/CDLIST_D"}

	summary, err := custexport.RunExport(ctx, client, custexport.ExportConfig{
		OutputDir: outputDir,
		Tables:    tables,
		PageSize:  100000,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("RunExport failed: %v", err)
	}

	// Verify zero errors — these tables should export cleanly after the .INCLUDE fix.
	if len(summary.Errors) > 0 {
		for _, e := range summary.Errors {
			t.Errorf("unexpected error for %s: %s", e.Table, e.Error)
		}
	}

	// Verify both tables have JSON files.
	for _, table := range tables {
		jsonName := strings.ReplaceAll(table, "/", "#") + ".json"
		jsonPath := filepath.Join(outputDir, "json", jsonName)
		if _, err := os.Stat(jsonPath); err != nil {
			t.Errorf("missing JSON for %s: %v", table, err)
			continue
		}
		t.Logf("%s exported successfully", table)
	}

	// Verify SQLite has the tables with PRIMARY KEY.
	dbPath := filepath.Join(outputDir, "customizing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, table := range tables {
		var ddl string
		err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = ?`, table).Scan(&ddl)
		if err != nil {
			t.Errorf("%s: not found in SQLite: %v", table, err)
			continue
		}
		if !strings.Contains(ddl, "PRIMARY KEY") {
			t.Errorf("%s: expected PRIMARY KEY in SQLite DDL, got: %s", table, ddl)
		} else {
			t.Logf("%s: has PRIMARY KEY", table)
		}
	}

	t.Logf("summary: %d exported, %d errors", summary.ExportedTables, len(summary.Errors))
}
