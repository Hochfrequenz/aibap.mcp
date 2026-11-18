package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

var bwfFromRe = regexp.MustCompile(`(?i)\bFROM\s+([A-Za-z0-9_/~]+)`)

// bwfFakeQuery answers RunQuery from a table->rows fixture, keeping rows whose
// WFD_ID/TASK/OBJID/CLSNAME value appears quoted in the SQL.
func bwfFakeQuery(tables map[string][]map[string]string, calls *int32) func(context.Context, string, int) (*adt.QueryResult, error) {
	return func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
		atomic.AddInt32(calls, 1)
		m := bwfFromRe.FindStringSubmatch(sql)
		if m == nil {
			return &adt.QueryResult{}, nil
		}
		var matched []map[string]string
		for _, r := range tables[strings.ToUpper(m[1])] {
			found, hit := false, false
			for _, k := range []string{"WFD_ID", "TASK", "OBJID", "CLSNAME"} {
				if v, ok := r[k]; ok {
					found = true
					if strings.Contains(sql, "'"+v+"'") {
						hit = true
					}
				}
			}
			if hit || !found {
				matched = append(matched, r)
			}
		}
		res := &adt.QueryResult{}
		if len(matched) > 0 {
			names := make([]string, 0, len(matched[0]))
			for k := range matched[0] {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, n := range names {
				res.Columns = append(res.Columns, adt.QueryColumn{Name: n})
			}
			for _, r := range matched {
				row := make([]string, len(names))
				for i, n := range names {
					row[i] = r[n]
				}
				res.Rows = append(res.Rows, row)
			}
		}
		res.TotalRows = len(res.Rows)
		return res, nil
	}
}

func bwfServer(t *testing.T, tables map[string][]map[string]string, calls *int32) *mockClient {
	t.Helper()
	return &mockClient{runQueryFn: bwfFakeQuery(tables, calls)}
}

func loadBWFFixture(t *testing.T) map[string][]map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("bwf", "testdata", "ws48900002.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string][]map[string]string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGetWorkflowDefinition_Success(t *testing.T) {
	var calls int32
	s := newTestServerWithFallback(bwfServer(t, loadBWFFixture(t), &calls), nil)
	res := callTool(t, s, "get_workflow_definition", map[string]interface{}{"wfd_id": "WS48900002"})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res.Content)
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want object", res.StructuredContent)
	}
	if sc["version"] != "0001" {
		t.Errorf("version = %v, want 0001", sc["version"])
	}
}

func TestGetWorkflowDefinition_LocalValidation(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"InvalidID":      {"wfd_id": "TS30000018"},
		"InvalidVersion": {"wfd_id": "WS48900002", "version": "1"},
		"InvalidSource":  {"wfd_id": "WS48900002", "source": "db"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			var calls int32
			s := newTestServerWithFallback(bwfServer(t, loadBWFFixture(t), &calls), nil)
			res := callTool(t, s, "get_workflow_definition", args)
			if !res.IsError {
				t.Fatal("expected tool error")
			}
			if n := atomic.LoadInt32(&calls); n != 0 {
				t.Errorf("RunQuery called %d times, want 0", n)
			}
		})
	}
}

func TestGetWorkflowDefinition_NotFoundIsToolError(t *testing.T) {
	var calls int32
	s := newTestServerWithFallback(bwfServer(t, map[string][]map[string]string{}, &calls), nil)
	res := callTool(t, s, "get_workflow_definition", map[string]interface{}{"wfd_id": "WS48900002"})
	if !res.IsError {
		t.Fatal("expected tool error")
	}
	txt := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			txt += tc.Text
		}
	}
	if !strings.Contains(txt, "SWDSHEADER") {
		t.Errorf("error text %q does not name SWDSHEADER", txt)
	}
}
