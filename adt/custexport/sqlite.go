package custexport

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

// sqliteWriter handles all SQLite database operations for the customizing export.
type sqliteWriter struct {
	db *sql.DB
}

func newSQLiteWriter(dbPath string) (*sqliteWriter, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single-connection mode: prevents SQLITE_BUSY from connection pool contention.
	db.SetMaxOpenConns(1)

	// WAL mode for write performance during export.
	// Compacted back to DELETE mode on Close() via VACUUM.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	// Synchronous=NORMAL is safe with WAL and much faster than FULL.
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}
	// Busy timeout: retry on lock contention instead of failing immediately.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

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

	return &sqliteWriter{db: db}, nil
}

// Close compacts the database (VACUUM) and switches from WAL to DELETE journal
// mode for portability, then closes the connection.
// VACUUM runs first while still in WAL mode (faster than in DELETE mode),
// then journal_mode=DELETE checkpoints the WAL and removes -wal/-shm files.
func (sw *sqliteWriter) Close() error {
	if sw.db == nil {
		return nil
	}
	if _, err := sw.db.Exec("VACUUM"); err != nil {
		log.Printf("WARNING: VACUUM failed: %v", err)
	}
	_, _ = sw.db.Exec("PRAGMA journal_mode=DELETE")
	return sw.db.Close()
}

// WriteTable creates the table, inserts metadata, and inserts all rows in a transaction.
func (sw *sqliteWriter) WriteTable(result *TableExportResult) error {
	tx, err := sw.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := sw.createTable(tx, result); err != nil {
		return err
	}
	if err := sw.insertMetadata(tx, result); err != nil {
		return err
	}
	if len(result.Rows) > 0 {
		if err := sw.insertRows(tx, result); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (sw *sqliteWriter) createTable(tx *sql.Tx, result *TableExportResult) error {
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

func (sw *sqliteWriter) insertMetadata(tx *sql.Tx, result *TableExportResult) error {
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

func (sw *sqliteWriter) insertRows(tx *sql.Tx, result *TableExportResult) error {
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
