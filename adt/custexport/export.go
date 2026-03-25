package custexport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

// ExportConfig holds parameters for RunExport.
type ExportConfig struct {
	OutputDir    string
	Tables       []string // specific tables, or nil for all customizing tables
	CustomerOnly bool     // if true, only export tables that were actually transported (E071K)
	PageSize     int      // rows per page (default 100000)
	Workers      int      // parallel workers (default 20, max 40)
}

// ExportSummary is the result of a full export run.
type ExportSummary struct {
	System         string       `json:"system"`
	Client         string       `json:"client"`
	StartedAt      string       `json:"started_at"`
	FinishedAt     string       `json:"finished_at"`
	DurationSecs   float64      `json:"duration_seconds"`
	TotalTables    int          `json:"total_tables"`
	ExportedTables int          `json:"exported_tables"`
	EmptyTables    int          `json:"empty_tables"`
	TotalRows      int          `json:"total_rows"`
	PageSize       int          `json:"page_size"`
	Workers        int          `json:"workers"`
	Errors         []TableError `json:"errors"`
}

// TableError records a per-table failure.
type TableError struct {
	Table string `json:"table"`
	Error string `json:"error"`
}

const (
	defaultPageSize = 100000
	// defaultWorkers is the default number of parallel export workers.
	// Benchmarked on srvhfuhana (2026-03-25, 500 tables, per-table key fetch):
	//   10 workers: 2.7 tables/sec → ~5.8h for 57K tables
	//   20 workers: 4.5 tables/sec → ~3.5h for 57K tables (sweet spot)
	//   30 workers: 4.3 tables/sec → no improvement, SAP saturated
	//   40 workers: 4.3 tables/sec → no improvement
	// Each worker does 2 sequential HTTP requests per table (DD03L keys + data).
	defaultWorkers     = 20
	maxWorkers         = 40
	maxKeysForPaginate = 4
	perQueryTimeout    = 120 * time.Second
	progressInterval   = 100
)

// discoverTables queries DD02L for active customizing tables (CONTFLAG C or G).
func discoverTables(ctx context.Context, client adt.Client) ([]string, error) {
	sql := "SELECT TABNAME, CONTFLAG FROM DD02L WHERE CONTFLAG IN ('C','G') AND TABCLASS = 'TRANSP' AND AS4LOCAL = 'A' AND AS4VERS = '0000' ORDER BY TABNAME"
	queryCtx, cancel := context.WithTimeout(ctx, perQueryTimeout)
	defer cancel()

	result, err := client.RunQuery(queryCtx, sql, 200000) // ~70K customizing tables, 200K gives headroom
	if err != nil {
		return nil, fmt.Errorf("discoverTables: %w", err)
	}

	tables := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			name := strings.TrimSpace(row[0])
			if name != "" {
				tables = append(tables, name)
			}
		}
	}
	sort.Strings(tables)
	return tables, nil
}

// discoverCustomerTables returns only customizing tables that were actually
// modified and transported. Queries E071K transport keys for objects of type
// TABU (table contents), CDAT (view cluster data via SM34), VDAT (view data
// via SM30), and TDAT (table data, client-independent).
//
// Known limitations:
//   - Only transported customizing is captured. Local changes in $TMP or local
//     requests are invisible. For dev systems, use customer_only=false instead.
//   - CDAT/VDAT entries use the view/cluster name, not the underlying table name.
//     We include them as-is — some will match DD02L (when the view name equals
//     the table name), others won't (filtered out by the intersection).
//     This is an 80/20 heuristic, not a guarantee of completeness.
func discoverCustomerTables(ctx context.Context, client adt.Client) ([]string, error) {
	// Get all transported table/view names from E071K.
	// TABU = table contents, CDAT = view cluster (SM34), VDAT = view data (SM30), TDAT = table data.
	sql := "SELECT DISTINCT OBJNAME FROM E071K WHERE PGMID = 'R3TR' AND OBJECT IN ('TABU','CDAT','VDAT','TDAT') ORDER BY OBJNAME"
	queryCtx, cancel := context.WithTimeout(ctx, perQueryTimeout)
	defer cancel()

	result, err := client.RunQuery(queryCtx, sql, 200000)
	if err != nil {
		return nil, fmt.Errorf("discoverCustomerTables (E071K): %w", err)
	}

	transported := make(map[string]bool, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 {
			transported[strings.TrimSpace(row[0])] = true
		}
	}

	// Get all customizing tables (delivery class C and G).
	// G (protected) is included because customers can add entries to G tables,
	// and those entries appear in E071K when transported.
	allTables, err := discoverTables(ctx, client)
	if err != nil {
		return nil, err
	}

	// Intersect: customizing tables that were actually transported.
	var customerTables []string
	for _, t := range allTables {
		if transported[t] {
			customerTables = append(customerTables, t)
		}
	}

	log.Printf("Customer customizing: %d tables transported (E071K) ∩ %d customizing (DD02L) = %d tables",
		len(transported), len(allTables), len(customerTables))

	return customerTables, nil
}

// fetchTableKeys queries DD03L for key fields of a single table.
// Called by each worker just before exporting the table.
func fetchTableKeys(ctx context.Context, client adt.Client, table string) ([]string, error) {
	sql := fmt.Sprintf(
		"SELECT FIELDNAME, POSITION FROM DD03L WHERE TABNAME = '%s' AND KEYFLAG = 'X' AND AS4LOCAL = 'A' ORDER BY POSITION",
		adt.EscapeValue(table),
	)
	queryCtx, cancel := context.WithTimeout(ctx, perQueryTimeout)
	defer cancel()

	result, err := client.RunQuery(queryCtx, sql, 1000)
	if err != nil {
		return nil, fmt.Errorf("fetchTableKeys %s: %w", table, err)
	}

	var keys []string
	for _, row := range result.Rows {
		if len(row) > 0 {
			name := strings.TrimSpace(row[0])
			if name != "" {
				keys = append(keys, name)
			}
		}
	}
	return keys, nil
}

// exportTable performs a paginated export of a single table.
// keys is the full list of key fields (including MANDT).
func exportTable(ctx context.Context, client adt.Client, table string, keys []string, pageSize int) (*TableExportResult, error) {
	nonMandtKeys := adt.FilterNonMandtKeys(keys)

	// Limit pagination keys to first maxKeysForPaginate for tables with many keys.
	paginateKeys := nonMandtKeys
	if len(paginateKeys) > maxKeysForPaginate {
		paginateKeys = paginateKeys[:maxKeysForPaginate]
	}

	var allRows [][]string
	var columns []adt.QueryColumn
	pages := 0
	var lastValues []string

	for {
		sqlStr, err := adt.BuildExportSQL(table, keys, paginateKeys, lastValues)
		if err != nil {
			return nil, fmt.Errorf("build SQL for %s: %w", table, err)
		}

		queryCtx, cancel := context.WithTimeout(ctx, perQueryTimeout)
		result, err := client.RunQuery(queryCtx, sqlStr, pageSize)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("query %s page %d: %w", table, pages+1, err)
		}

		pages++
		if pages == 1 {
			columns = result.Columns
			// The ADT data preview endpoint returns keyAttribute=false for all columns.
			// Mark key columns using the DD03L key info we already have.
			keySet := make(map[string]bool, len(keys))
			for _, k := range keys {
				keySet[k] = true
			}
			for i := range columns {
				if keySet[columns[i].Name] {
					columns[i].IsKey = true
				}
			}
		}

		allRows = append(allRows, result.Rows...)

		// Stop if we got fewer rows than pageSize (last page) or no pagination keys.
		if len(result.Rows) < pageSize || len(paginateKeys) == 0 {
			break
		}

		// Extract last row's non-MANDT key values for pagination.
		lastRow := result.Rows[len(result.Rows)-1]
		lastValues = extractKeyValues(columns, paginateKeys, lastRow)
		if lastValues == nil {
			break // cannot paginate further
		}
	}

	return &TableExportResult{
		TableName: table,
		Columns:   columns,
		Rows:      allRows,
		TotalRows: len(allRows),
		Pages:     pages,
	}, nil
}

// extractKeyValues gets the values for the given key fields from a row,
// using the column definitions to find column indices.
func extractKeyValues(columns []adt.QueryColumn, keys []string, row []string) []string {
	colIndex := make(map[string]int, len(columns))
	for i, col := range columns {
		colIndex[col.Name] = i
	}

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		idx, ok := colIndex[key]
		if !ok || idx >= len(row) {
			return nil
		}
		values = append(values, row[idx])
	}
	return values
}

// RunExport performs a full customizing table export.
func RunExport(ctx context.Context, client adt.Client, cfg ExportConfig) (*ExportSummary, error) {
	// Create cancellable context — writer errors cancel all workers.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	startedAt := time.Now()

	// Apply defaults.
	pageSize := cfg.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	workers := cfg.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}

	// Discover or use provided tables.
	var tables []string
	if len(cfg.Tables) > 0 {
		tables = make([]string, len(cfg.Tables))
		copy(tables, cfg.Tables)
		sort.Strings(tables)
	} else if cfg.CustomerOnly {
		var err error
		tables, err = discoverCustomerTables(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("discover customer tables: %w", err)
		}
	} else {
		var err error
		tables, err = discoverTables(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("discover tables: %w", err)
		}
	}

	// Create writer.
	writer, err := NewWriter(cfg.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("create writer: %w", err)
	}
	defer func() { _ = writer.Close() }()

	// Channels.
	workCh := make(chan string, len(tables))
	resultCh := make(chan *TableExportResult, workers*2)

	// Feed work channel.
	for _, t := range tables {
		workCh <- t
	}
	close(workCh)

	// Start workers.
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for table := range workCh {
				keys, kErr := fetchTableKeys(ctx, client, table)
				if kErr != nil {
					resultCh <- &TableExportResult{TableName: table, Error: kErr}
					continue
				}
				result, err := exportTable(ctx, client, table, keys, pageSize)
				if err != nil {
					resultCh <- &TableExportResult{
						TableName: table,
						Error:     err,
					}
				} else {
					resultCh <- result
				}
			}
		}()
	}

	// Close results channel when all workers finish.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Writer goroutine: the ONLY thing that touches the Writer.
	var writerErr error
	var exported, empty, totalRows, errCount int
	var tableErrors []TableError

	for result := range resultCh {
		processed := exported + empty + errCount + 1
		if result.Error != nil {
			errCount++
			tableErrors = append(tableErrors, TableError{
				Table: result.TableName,
				Error: result.Error.Error(),
			})
		} else if writerErr != nil {
			// Writer already failed — skip writes, just drain results.
		} else if len(result.Rows) == 0 {
			empty++
			if wErr := writer.WriteTable(result); wErr != nil {
				log.Printf("WARNING: write error for empty table %s: %v", result.TableName, wErr)
			}
		} else {
			totalRows += len(result.Rows)
			exported++
			if wErr := writer.WriteTable(result); wErr != nil {
				writerErr = wErr
				log.Printf("ERROR: write failed for %s: %v — cancelling workers", result.TableName, wErr)
				cancel()
			}
		}

		if processed%progressInterval == 0 {
			elapsed := time.Since(startedAt).Seconds()
			log.Printf("[%d/%d] exported, %d errors, elapsed %.0fs", processed, len(tables), errCount, elapsed)
		}
	}

	if writerErr != nil {
		return nil, fmt.Errorf("writer error: %w", writerErr)
	}

	finishedAt := time.Now()
	summary := &ExportSummary{
		StartedAt:      startedAt.UTC().Format(time.RFC3339),
		FinishedAt:     finishedAt.UTC().Format(time.RFC3339),
		DurationSecs:   finishedAt.Sub(startedAt).Seconds(),
		TotalTables:    len(tables),
		ExportedTables: exported,
		EmptyTables:    empty,
		TotalRows:      totalRows,
		PageSize:       pageSize,
		Workers:        workers,
		Errors:         tableErrors,
	}

	// Write summary file.
	summaryData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return summary, fmt.Errorf("marshal summary: %w", err)
	}
	summaryPath := filepath.Join(cfg.OutputDir, "export_summary.json")
	if err := os.WriteFile(summaryPath, summaryData, 0o644); err != nil {
		return summary, fmt.Errorf("write summary: %w", err)
	}

	return summary, nil
}
