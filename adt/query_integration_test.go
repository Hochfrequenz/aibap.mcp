//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestRunQuery_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.RunQuery(ctx, "SELECT BUKRS, BUTXT FROM T001 ORDER BY BUKRS", 10)
	if err != nil {
		t.Fatalf("RunQuery failed: %v", err)
	}

	if len(result.Columns) < 2 {
		t.Fatalf("expected at least 2 columns, got %d", len(result.Columns))
	}
	if result.Columns[0].Name != "BUKRS" {
		t.Errorf("first column name: got %q, want BUKRS", result.Columns[0].Name)
	}
	if result.Columns[1].Name != "BUTXT" {
		t.Errorf("second column name: got %q, want BUTXT", result.Columns[1].Name)
	}

	if len(result.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	if len(result.Rows) > 10 {
		t.Errorf("expected at most 10 rows, got %d", len(result.Rows))
	}
	t.Logf("got %d rows (totalRows=%d, executionMs=%.1f)", len(result.Rows), result.TotalRows, result.ExecutionMs)
	for i, row := range result.Rows {
		if len(row) < 2 {
			t.Errorf("row %d: expected at least 2 columns, got %d", i, len(row))
			continue
		}
		t.Logf("  %s = %s", row[0], row[1])
	}
}
