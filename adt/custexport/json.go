package custexport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

// jsonRow is a map of column name → value. Go's encoding/json sorts map keys
// alphabetically, ensuring deterministic output.
type jsonRow map[string]string

// writeJSON writes a single table's data as a JSON file.
func writeJSON(jsonDir string, result *TableExportResult) error {
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
	path := filepath.Join(jsonDir, filename)

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
