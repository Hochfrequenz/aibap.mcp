package custexport

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	_ "modernc.org/sqlite"
)

// TableExportResult holds the export data for one table.
type TableExportResult struct {
	TableName string
	Columns   []adt.QueryColumn
	Rows      [][]string
	TotalRows int
	Pages     int
	Error     error
}

// Writer writes table export results to SQLite + JSON.
// NOT goroutine-safe — must be called from a single goroutine.
// The export orchestrator uses a dedicated writer goroutine that
// receives results via a channel and calls WriteTable sequentially.
type Writer struct {
	db        *sql.DB
	jsonDir   string
	outputDir string
}

// NewWriter creates a Writer. Creates customizing.db and json/ subdirectory.
func NewWriter(outputDir string) (*Writer, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	jsonDir := filepath.Join(outputDir, "json")
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		return nil, fmt.Errorf("create json dir: %w", err)
	}

	dbPath := filepath.Join(outputDir, "customizing.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for better performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Create _metadata table.
	const createMetadata = `CREATE TABLE IF NOT EXISTS "_metadata" (
		"table_name" TEXT NOT NULL,
		"column_name" TEXT NOT NULL,
		"position" INTEGER NOT NULL,
		"abap_type" TEXT,
		"description" TEXT,
		"is_key" INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY ("table_name", "column_name")
	)`
	if _, err := db.Exec(createMetadata); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create _metadata table: %w", err)
	}

	return &Writer{
		db:        db,
		jsonDir:   jsonDir,
		outputDir: outputDir,
	}, nil
}

// Close closes the SQLite database.
func (w *Writer) Close() error {
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

// WriteTable writes a single table's data to SQLite and JSON.
func (w *Writer) WriteTable(result *TableExportResult) error {
	if err := w.writeSQLite(result); err != nil {
		return fmt.Errorf("sqlite write %s: %w", result.TableName, err)
	}
	if err := w.writeJSON(result); err != nil {
		return fmt.Errorf("json write %s: %w", result.TableName, err)
	}
	return nil
}

func (w *Writer) writeSQLite(result *TableExportResult) error {
	tx, err := w.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Build CREATE TABLE statement.
	if err := w.createTable(tx, result); err != nil {
		return err
	}

	// Insert metadata.
	if err := w.insertMetadata(tx, result); err != nil {
		return err
	}

	// Insert rows.
	if len(result.Rows) > 0 {
		if err := w.insertRows(tx, result); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (w *Writer) createTable(tx *sql.Tx, result *TableExportResult) error {
	var colDefs []string
	var keyColumns []string

	for _, col := range result.Columns {
		colDefs = append(colDefs, fmt.Sprintf("%q TEXT", col.Name))
		if col.IsKey {
			keyColumns = append(keyColumns, fmt.Sprintf("%q", col.Name))
		}
	}

	// Drop existing table to avoid stale data on re-runs.
	if _, err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q", result.TableName)); err != nil {
		return fmt.Errorf("drop table %s: %w", result.TableName, err)
	}
	ddl := fmt.Sprintf("CREATE TABLE %q (%s", result.TableName, strings.Join(colDefs, ", "))
	if len(keyColumns) > 0 {
		ddl += fmt.Sprintf(", PRIMARY KEY (%s)", strings.Join(keyColumns, ", "))
	}
	ddl += ")"

	_, err := tx.Exec(ddl)
	return err
}

func (w *Writer) insertMetadata(tx *sql.Tx, result *TableExportResult) error {
	const stmt = `INSERT OR REPLACE INTO "_metadata" ("table_name", "column_name", "position", "abap_type", "description", "is_key") VALUES (?, ?, ?, ?, ?, ?)`
	for i, col := range result.Columns {
		isKey := 0
		if col.IsKey {
			isKey = 1
		}
		if _, err := tx.Exec(stmt, result.TableName, col.Name, i, col.Type, col.Description, isKey); err != nil {
			return fmt.Errorf("insert metadata for %s.%s: %w", result.TableName, col.Name, err)
		}
	}
	return nil
}

func (w *Writer) insertRows(tx *sql.Tx, result *TableExportResult) error {
	placeholders := make([]string, len(result.Columns))
	quotedCols := make([]string, len(result.Columns))
	for i, col := range result.Columns {
		placeholders[i] = "?"
		quotedCols[i] = fmt.Sprintf("%q", col.Name)
	}

	insertSQL := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)",
		result.TableName,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "))

	prepared, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = prepared.Close() }()

	numCols := len(result.Columns)
	for rowIdx, row := range result.Rows {
		if len(row) != numCols {
			return fmt.Errorf("row %d has %d values, expected %d columns", rowIdx, len(row), numCols)
		}
		vals := make([]any, numCols)
		for i, v := range row {
			vals[i] = v
		}
		if _, err := prepared.Exec(vals...); err != nil {
			return fmt.Errorf("insert row %d: %w", rowIdx, err)
		}
	}
	return nil
}

// jsonTable is the JSON output format for a single table.
type jsonTable struct {
	Table     string       `json:"table"`
	TotalRows int          `json:"total_rows"`
	Pages     int          `json:"pages"`
	Columns   []jsonColumn `json:"columns"`
	Rows      []jsonRow    `json:"rows"`
}

type jsonColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	IsKey       bool   `json:"is_key"`
}

// jsonRow is an ordered map of column name -> value, using sorted keys.
type jsonRow map[string]string

func (w *Writer) writeJSON(result *TableExportResult) error {
	columns := make([]jsonColumn, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = jsonColumn{
			Name:        col.Name,
			Type:        col.Type,
			Description: col.Description,
			IsKey:       col.IsKey,
		}
	}

	rows := make([]jsonRow, len(result.Rows))
	for i, row := range result.Rows {
		m := make(jsonRow, len(result.Columns))
		for j, col := range result.Columns {
			if j < len(row) {
				m[col.Name] = row[j]
			}
		}
		rows[i] = m
	}

	out := jsonTable{
		Table:     result.TableName,
		TotalRows: result.TotalRows,
		Pages:     result.Pages,
		Columns:   columns,
		Rows:      rows,
	}

	filename := safeFilename(result.TableName) + ".json"
	path := filepath.Join(w.jsonDir, filename)

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// safeFilename replaces / with # for namespace tables.
func safeFilename(tableName string) string {
	return strings.ReplaceAll(tableName, "/", "#")
}
