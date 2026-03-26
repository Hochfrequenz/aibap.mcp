package custexport

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

// Comparison-time PRAGMAs: when opening an exported database for read-only
// comparison (e.g. ATTACH two databases and run EXCEPT queries), apply these
// settings for optimal performance:
//
//	PRAGMA mmap_size = 1073741824;  -- 1 GB memory-mapped I/O (avoids syscall overhead)
//	PRAGMA cache_size = -200000;    -- 200 MB page cache
//	PRAGMA temp_store = MEMORY;     -- keep temp B-trees in RAM
//
// These are NOT set during export (write-optimized settings differ).

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
	// ANALYZE populates sqlite_stat1 so the query planner can pick optimal
	// JOINs during cross-system comparison queries.
	if _, err := sw.db.Exec("ANALYZE"); err != nil {
		log.Printf("WARNING: ANALYZE failed: %v", err)
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
	// _row_hash: SHA256 of all non-key column values, for fast cross-system comparison.
	// Enables "detect any change by key" queries without comparing every column:
	//   SELECT d.* FROM dev."T001" d JOIN qa."T001" q
	//     ON d."MANDT" = q."MANDT" AND d."BUKRS" = q."BUKRS"
	//     WHERE d."_row_hash" <> q."_row_hash";
	colDefs = append(colDefs, `"_row_hash" BLOB`)

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

// insertMetadata records column info in _metadata. The synthetic _row_hash column
// is intentionally excluded — it is not an ABAP column but a comparison helper.
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
	numCols := len(result.Columns)

	// Build column list + _row_hash.
	quotedCols := make([]string, numCols+1)
	placeholders := make([]string, numCols+1)
	for i, col := range result.Columns {
		quotedCols[i] = fmt.Sprintf("%q", col.Name)
		placeholders[i] = "?"
	}
	quotedCols[numCols] = `"_row_hash"`
	placeholders[numCols] = "?"

	insertSQL := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)",
		result.TableName,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "))

	prepared, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = prepared.Close() }()

	// Identify non-key column indices for hashing.
	nonKeyIndices := make([]int, 0, numCols)
	for i, col := range result.Columns {
		if !col.IsKey {
			nonKeyIndices = append(nonKeyIndices, i)
		}
	}

	var lenBuf [4]byte // for length-prefixed encoding

	for rowIdx, row := range result.Rows {
		if len(row) != numCols {
			return fmt.Errorf("row %d has %d values, expected %d columns", rowIdx, len(row), numCols)
		}

		// Compute SHA256 of non-key column values using length-prefixed encoding.
		// Length-prefix prevents collisions: "AB"+"C" and "A"+"BC" produce different hashes.
		// If all columns are keys, store NULL (no non-key data to hash).
		var hash any
		if len(nonKeyIndices) > 0 {
			h := sha256.New()
			for _, idx := range nonKeyIndices {
				v := []byte(row[idx])
				binary.BigEndian.PutUint32(lenBuf[:], uint32(len(v)))
				h.Write(lenBuf[:])
				h.Write(v)
			}
			hash = h.Sum(nil)
		}
		// hash is nil (SQL NULL) when there are no non-key columns

		vals := make([]any, numCols+1)
		for i, v := range row {
			vals[i] = v
		}
		vals[numCols] = hash

		if _, err := prepared.Exec(vals...); err != nil {
			return fmt.Errorf("insert row %d: %w", rowIdx, err)
		}
	}
	return nil
}
