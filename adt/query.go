package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

// validateSelectOnly checks that sql is a single SELECT statement.
// Rejects multi-statement (semicolons) and non-SELECT keywords at the start.
func validateSelectOnly(sql string) error {
	if sql == "" {
		return fmt.Errorf("empty SQL statement")
	}
	upper := strings.ToUpper(sql)
	if !strings.HasPrefix(upper, "SELECT") {
		return fmt.Errorf("only SELECT statements are allowed, got: %s",
			strings.SplitN(sql, " ", 2)[0])
	}
	if strings.Contains(sql, ";") {
		return fmt.Errorf("multi-statement SQL is not allowed (semicolons forbidden)")
	}
	return nil
}

// RunQuery executes a read-only SQL query via the ADT data preview endpoint.
// Only single SELECT statements are allowed; anything else is rejected.
// If the caller's context has no deadline, a 5-minute timeout is applied.
func (c *httpClient) RunQuery(ctx context.Context, sql string, maxRows int) (*QueryResult, error) {
	trimmed := strings.TrimSpace(sql)
	if err := validateSelectOnly(trimmed); err != nil {
		return nil, fmt.Errorf("RunQuery: %w", err)
	}

	if maxRows <= 0 {
		maxRows = 1000
	}

	// Apply default timeout if caller didn't set one.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	path := fmt.Sprintf("/sap/bc/adt/datapreview/freestyle?rowNumber=%d", maxRows)
	headers := map[string]string{
		"Content-Type": "text/plain",
		"Accept":       "application/vnd.sap.adt.datapreview.table.v1+xml",
	}

	resp, err := c.doMutateLong(ctx, "POST", path, strings.NewReader(trimmed), headers)
	if err != nil {
		return nil, fmt.Errorf("RunQuery: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("RunQuery: reading response: %w", err)
	}

	// Sanitize: SAP data may contain control characters (e.g. U+000F) that are
	// illegal in XML 1.0. Strip them before parsing to avoid xml.Unmarshal errors.
	body = sanitizeXML(body)

	var dpResult adtxml.DataPreviewResult
	if err := xml.Unmarshal(body, &dpResult); err != nil {
		return nil, fmt.Errorf("RunQuery: parsing XML: %w", err)
	}

	return transposeDataPreview(&dpResult)
}

// sanitizeXML strips C0 control characters (0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F) that are
// illegal in XML 1.0, while preserving tab (0x09), newline (0x0A), and carriage return (0x0D).
// Operates on raw bytes — safe for UTF-8 since multi-byte continuation bytes are always >= 0x80.
// SAP data can contain control characters (e.g. U+000F in TA23HOTELS) that break xml.Unmarshal.
func sanitizeXML(data []byte) []byte {
	clean := make([]byte, 0, len(data))
	for _, b := range data {
		if b == 0x09 || b == 0x0A || b == 0x0D || b >= 0x20 {
			clean = append(clean, b)
		}
		// Drop bytes 0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F
	}
	return clean
}

// transposeDataPreview converts column-oriented DataPreviewResult to
// row-oriented QueryResult.
func transposeDataPreview(dp *adtxml.DataPreviewResult) (*QueryResult, error) {
	numCols := len(dp.Columns)
	if numCols == 0 {
		totalRows, _ := strconv.Atoi(dp.TotalRows)
		execMs, _ := strconv.ParseFloat(dp.QueryExecutionTime, 64)
		return &QueryResult{
			TotalRows:   totalRows,
			ExecutionMs: execMs,
		}, nil
	}

	// Determine number of rows from the first column's data set.
	numRows := len(dp.Columns[0].DataSet.Data)

	columns := make([]QueryColumn, numCols)
	for i, col := range dp.Columns {
		columns[i] = QueryColumn{
			Name:        col.Metadata.Name,
			Type:        col.Metadata.Type,
			Description: col.Metadata.Description,
			IsKey:       col.Metadata.KeyAttribute == "true",
		}
	}

	rows := make([][]string, numRows)
	for r := range numRows {
		row := make([]string, numCols)
		for colIdx, col := range dp.Columns {
			if r < len(col.DataSet.Data) {
				row[colIdx] = col.DataSet.Data[r]
			}
		}
		rows[r] = row
	}

	totalRows, _ := strconv.Atoi(dp.TotalRows)
	execMs, _ := strconv.ParseFloat(dp.QueryExecutionTime, 64)

	return &QueryResult{
		Columns:     columns,
		Rows:        rows,
		TotalRows:   totalRows,
		ExecutionMs: execMs,
	}, nil
}
