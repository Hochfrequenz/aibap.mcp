package adt

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

func TestRunQuery_RejectNonSelect(t *testing.T) {
	c := &httpClient{}
	ctx := context.Background()

	tests := []struct {
		name string
		sql  string
	}{
		{"DELETE", "DELETE FROM T001 WHERE BUKRS = '9999'"},
		{"UPDATE", "UPDATE T001 SET BUTXT = 'x' WHERE BUKRS = '0001'"},
		{"INSERT", "INSERT INTO T001 VALUES ('9999', 'Test')"},
		{"DROP", "DROP TABLE T001"},
		{"empty", ""},
		{"whitespace only", "   "},
		{"lowercase delete", "delete from T001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.RunQuery(ctx, tt.sql, 10)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tt.sql)
			}
		})
	}
}

func TestParseDataPreviewResult(t *testing.T) {
	t.Run("zero columns", func(t *testing.T) {
		dp := &adtmodel.DataPreviewResult{
			TotalRows:          "0",
			QueryExecutionTime: "1.5",
		}
		result, err := transposeDataPreview(dp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Columns) != 0 {
			t.Errorf("expected 0 columns, got %d", len(result.Columns))
		}
		if len(result.Rows) != 0 {
			t.Errorf("expected 0 rows, got %d", len(result.Rows))
		}
		if result.ExecutionMs != 1.5 {
			t.Errorf("expected ExecutionMs 1.5, got %f", result.ExecutionMs)
		}
	})

	t.Run("one column", func(t *testing.T) {
		dp := &adtmodel.DataPreviewResult{
			TotalRows:          "2",
			QueryExecutionTime: "3.14",
			Columns: []adtmodel.DataPreviewColumn{
				{
					Metadata: adtmodel.DataPreviewMetadata{
						Name:         "BUKRS",
						Type:         "C",
						Description:  "Company Code",
						KeyAttribute: "true",
					},
					DataSet: adtmodel.DataPreviewDataSet{
						Data: []string{"0001", "0002"},
					},
				},
			},
		}
		result, err := transposeDataPreview(dp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Columns) != 1 {
			t.Fatalf("expected 1 column, got %d", len(result.Columns))
		}
		if result.Columns[0].Name != "BUKRS" {
			t.Errorf("column name: got %q", result.Columns[0].Name)
		}
		if !result.Columns[0].IsKey {
			t.Error("expected BUKRS to be key column")
		}
		if len(result.Rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(result.Rows))
		}
		if result.Rows[0][0] != "0001" {
			t.Errorf("row 0 col 0: got %q", result.Rows[0][0])
		}
		if result.Rows[1][0] != "0002" {
			t.Errorf("row 1 col 0: got %q", result.Rows[1][0])
		}
		if result.TotalRows != 2 {
			t.Errorf("TotalRows: got %d", result.TotalRows)
		}
	})

	t.Run("two columns", func(t *testing.T) {
		dp := &adtmodel.DataPreviewResult{
			TotalRows:          "3",
			QueryExecutionTime: "42.0",
			Columns: []adtmodel.DataPreviewColumn{
				{
					Metadata: adtmodel.DataPreviewMetadata{
						Name: "BUKRS",
						Type: "C",
					},
					DataSet: adtmodel.DataPreviewDataSet{
						Data: []string{"0001", "0002", "0003"},
					},
				},
				{
					Metadata: adtmodel.DataPreviewMetadata{
						Name:         "BUTXT",
						Type:         "C",
						Description:  "Company Name",
						KeyAttribute: "false",
					},
					DataSet: adtmodel.DataPreviewDataSet{
						Data: []string{"SAP AG", "Hochfrequenz", "Test Corp"},
					},
				},
			},
		}
		result, err := transposeDataPreview(dp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Columns) != 2 {
			t.Fatalf("expected 2 columns, got %d", len(result.Columns))
		}
		if len(result.Rows) != 3 {
			t.Fatalf("expected 3 rows, got %d", len(result.Rows))
		}
		// Check row-major layout
		if result.Rows[0][0] != "0001" || result.Rows[0][1] != "SAP AG" {
			t.Errorf("row 0: got %v", result.Rows[0])
		}
		if result.Rows[2][0] != "0003" || result.Rows[2][1] != "Test Corp" {
			t.Errorf("row 2: got %v", result.Rows[2])
		}
		if result.Columns[1].IsKey {
			t.Error("BUTXT should not be a key column")
		}
		if result.ExecutionMs != 42.0 {
			t.Errorf("ExecutionMs: got %f", result.ExecutionMs)
		}
	})
}
