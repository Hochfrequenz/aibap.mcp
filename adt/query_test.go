package adt

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
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
		{"multi-statement semicolon", "SELECT * FROM T001; DROP TABLE T001"},
		{"semicolon in select", "SELECT 1 FROM DUMMY; DELETE FROM USR02"},
		{"select with trailing semicolon", "SELECT * FROM T001;"},
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
		dp := &adtxml.DataPreviewResult{
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
		dp := &adtxml.DataPreviewResult{
			TotalRows:          "2",
			QueryExecutionTime: "3.14",
			Columns: []adtxml.DataPreviewColumn{
				{
					Metadata: adtxml.DataPreviewMetadata{
						Name:         "BUKRS",
						Type:         "C",
						Description:  "Company Code",
						KeyAttribute: "true",
					},
					DataSet: adtxml.DataPreviewDataSet{
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
		dp := &adtxml.DataPreviewResult{
			TotalRows:          "3",
			QueryExecutionTime: "42.0",
			Columns: []adtxml.DataPreviewColumn{
				{
					Metadata: adtxml.DataPreviewMetadata{
						Name: "BUKRS",
						Type: "C",
					},
					DataSet: adtxml.DataPreviewDataSet{
						Data: []string{"0001", "0002", "0003"},
					},
				},
				{
					Metadata: adtxml.DataPreviewMetadata{
						Name:         "BUTXT",
						Type:         "C",
						Description:  "Company Name",
						KeyAttribute: "false",
					},
					DataSet: adtxml.DataPreviewDataSet{
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

func TestSanitizeXML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean XML passes through", "<root>hello</root>", "<root>hello</root>"},
		{"strips U+000F", "abc\x0Fdef", "abcdef"},
		{"strips null byte", "abc\x00def", "abcdef"},
		{"strips mixed control chars", "\x01\x02\x03hello\x0E\x0Fworld\x1F", "helloworld"},
		{"preserves tab", "hello\tworld", "hello\tworld"},
		{"preserves newline", "hello\nworld", "hello\nworld"},
		{"preserves carriage return", "hello\rworld", "hello\rworld"},
		{"preserves all allowed controls", "\x09\x0A\x0Dtext", "\x09\x0A\x0Dtext"},
		{"preserves UTF-8 umlauts", "Ä Ö Ü ä ö ü ß", "Ä Ö Ü ä ö ü ß"},
		{"preserves 3-byte UTF-8", "日本語", "日本語"},
		{"empty input", "", ""},
		{"only control chars", "\x00\x01\x02\x0F", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(sanitizeXML([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("sanitizeXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
