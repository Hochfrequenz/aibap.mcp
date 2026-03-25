package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

// RunQuery executes a read-only SQL query via the ADT data preview endpoint.
// Only SELECT statements are allowed; anything else is rejected.
func (c *httpClient) RunQuery(ctx context.Context, sql string, maxRows int) (*QueryResult, error) {
	trimmed := strings.TrimSpace(sql)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("RunQuery: only SELECT statements are allowed, got: %s",
			strings.SplitN(trimmed, " ", 2)[0])
	}

	if maxRows <= 0 {
		maxRows = 1000
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

	var dpResult adtmodel.DataPreviewResult
	if err := xml.Unmarshal(body, &dpResult); err != nil {
		return nil, fmt.Errorf("RunQuery: parsing XML: %w", err)
	}

	return transposeDataPreview(&dpResult)
}

// transposeDataPreview converts column-oriented DataPreviewResult to
// row-oriented QueryResult.
func transposeDataPreview(dp *adtmodel.DataPreviewResult) (*QueryResult, error) {
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
