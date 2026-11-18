package bwf

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

var fromRe = regexp.MustCompile(`(?i)\bFROM\s+([A-Za-z0-9_/~]+)`)

// fakeQuery is an in-memory adt.QueryClient keyed by table name.
type fakeQuery struct {
	tables map[string][]Row
	calls  []string
}

func (f *fakeQuery) RunQuery(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
	f.calls = append(f.calls, sql)
	m := fromRe.FindStringSubmatch(sql)
	if m == nil {
		return &adt.QueryResult{}, nil
	}
	table := strings.ToUpper(m[1])
	var matched []Row
	for _, r := range f.tables[table] {
		if rowMatches(r, sql) {
			matched = append(matched, r)
		}
	}
	var cols []adt.QueryColumn
	var rows [][]string
	if len(matched) > 0 {
		names := make([]string, 0, len(matched[0]))
		for k := range matched[0] {
			names = append(names, k)
		}
		sortStrings(names)
		for _, n := range names {
			cols = append(cols, adt.QueryColumn{Name: n})
		}
		for _, r := range matched {
			out := make([]string, len(names))
			for i, n := range names {
				out[i] = r[n]
			}
			rows = append(rows, out)
		}
	}
	return &adt.QueryResult{Columns: cols, Rows: rows, TotalRows: len(rows)}, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// rowMatches keeps rows whose WFD_ID/TASK/OBJID/CLSNAME value (if any such
// column exists) appears as a quoted literal in the SQL.
func rowMatches(r Row, sql string) bool {
	found := false
	for _, k := range []string{"WFD_ID", "TASK", "OBJID", "CLSNAME"} {
		v, ok := r[k]
		if !ok {
			continue
		}
		found = true
		if strings.Contains(sql, "'"+v+"'") {
			return true
		}
	}
	return !found
}

func loadFixture(t *testing.T, name string) map[string][]Row {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var out map[string][]Row
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return out
}
