package custexport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
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
	sqlite  *sqliteWriter
	jsonDir string
}

// NewWriter creates a Writer. Creates customizing_{client}.db and json/ subdirectory.
// If client is empty, falls back to "customizing.db".
func NewWriter(outputDir string, client string) (*Writer, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	jsonDir := filepath.Join(outputDir, "json")
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		return nil, fmt.Errorf("create json dir: %w", err)
	}

	dbName := "customizing.db"
	if client != "" {
		dbName = fmt.Sprintf("customizing_%s.db", client)
	}
	dbPath := filepath.Join(outputDir, dbName)
	sw, err := newSQLiteWriter(dbPath)
	if err != nil {
		return nil, err
	}

	return &Writer{
		sqlite:  sw,
		jsonDir: jsonDir,
	}, nil
}

// Close closes the SQLite database.
func (w *Writer) Close() error {
	return w.sqlite.Close()
}

// WriteTable writes a single table's data to SQLite and JSON.
func (w *Writer) WriteTable(result *TableExportResult) error {
	if err := w.sqlite.WriteTable(result); err != nil {
		return fmt.Errorf("sqlite write %s: %w", result.TableName, err)
	}
	if err := writeJSON(w.jsonDir, result); err != nil {
		return fmt.Errorf("json write %s: %w", result.TableName, err)
	}
	return nil
}

// safeFilename replaces / with # for namespace tables.
func safeFilename(tableName string) string {
	return strings.ReplaceAll(tableName, "/", "#")
}
