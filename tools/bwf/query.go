package bwf

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// maxRows is the row cap passed to every query; hitting it is treated as truncation.
const maxRows = 5000

// Row is one result row: upper-case column name to trimmed value.
type Row map[string]string

var (
	taskRe = regexp.MustCompile(`^WS\d{8}$`)
	nameRe = regexp.MustCompile(`^[A-Z0-9_/]{1,30}$`)
)

// NormalizeTask upper-cases and trims a workflow task id, adding the WS prefix
// when only the 8 digits are given.
func NormalizeTask(in string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(in))
	if len(s) == 8 && strings.Trim(s, "0123456789") == "" {
		s = "WS" + s
	}
	if !taskRe.MatchString(s) {
		return "", fmt.Errorf("invalid workflow task %q: expected WS followed by 8 digits", in)
	}
	return s, nil
}

// validName reports whether s is safe to embed as an SQL literal.
func validName(s string) bool { return nameRe.MatchString(s) }

// fetch runs sql and maps the rows by column name. A truncated result is an error.
func fetch(ctx context.Context, q adt.QueryClient, sql string) ([]Row, error) {
	res, err := q.RunQuery(ctx, sql, maxRows)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []Row{}, nil
	}
	if res.TotalRows > len(res.Rows) || len(res.Rows) >= maxRows {
		return nil, fmt.Errorf("query result truncated (%d of %d rows, limit %d)", len(res.Rows), res.TotalRows, maxRows)
	}
	rows := make([]Row, 0, len(res.Rows))
	for _, r := range res.Rows {
		row := make(Row, len(res.Columns))
		for i, c := range res.Columns {
			if i < len(r) {
				row[strings.ToUpper(c.Name)] = strings.TrimSpace(r[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
