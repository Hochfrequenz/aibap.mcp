//go:build integration

package tools_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools/bwf"
)

// callWorkflowDefinition invokes get_workflow_definition and decodes the result.
// It also returns the raw JSON text for whole-document assertions.
func callWorkflowDefinition(t *testing.T, args map[string]interface{}) (bwf.Result, string) {
	t.Helper()
	res := callTool(t, sharedServer, "get_workflow_definition", args)
	if res.IsError {
		t.Fatalf("get_workflow_definition(%v) returned IsError=true: %s", args, textOf(res))
	}
	raw := textOf(res)
	var out bwf.Result
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal result: %v\nraw: %s", err, raw)
	}
	return out, raw
}

func findNodeByID(d bwf.Result, id string) (bwf.Node, bool) {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return bwf.Node{}, false
}

func edgeLabel(d bwf.Result, from, to string) (string, bool) {
	for _, e := range d.Edges {
		if e.From == from && e.To == to {
			return e.Label, true
		}
	}
	return "", false
}

func TestIntegration_WorkflowDefinition_WS48900002(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			d, raw := callWorkflowDefinition(t, map[string]interface{}{
				"wfd_id": "WS48900002",
				"depth":  1,
			})

			if d.Version != "0001" || d.VersionSource != "HRS1205" {
				t.Errorf("version/source: got %q/%q, want 0001/HRS1205", d.Version, d.VersionSource)
			}

			// Collapsed edge labels.
			for _, want := range []struct{ from, to, label string }{
				{"36", "42", "true"}, {"36", "50", "false"},
				{"23", "30", "A"}, {"23", "33", "R"}, {"23", "10", "others"},
				{"10", "55", "true"}, {"10", "15", "false"},
			} {
				got, ok := edgeLabel(d, want.from, want.to)
				if !ok {
					t.Errorf("edge %s->%s missing", want.from, want.to)
				} else if got != want.label {
					t.Errorf("edge %s->%s label: got %q, want %q", want.from, want.to, got, want.label)
				}
			}

			// Node-level expectations.
			if n, ok := findNodeByID(d, "36"); !ok || !strings.Contains(n.Condition, "&COUNTER& > 1") {
				t.Errorf("node 36 condition: got %+v (found=%v), want &COUNTER& > 1", n.Condition, ok)
			}
			if n, ok := findNodeByID(d, "10"); !ok || !strings.Contains(n.Condition, "&FLAG& = X") {
				t.Errorf("node 10 (until loop) condition: got %q (found=%v), want &FLAG& = X", n.Condition, ok)
			}
			if n, ok := findNodeByID(d, "23"); !ok || !strings.Contains(n.Condition, "&RESULT&") {
				t.Errorf("node 23 (multiple condition) condition: got %q (found=%v), want &RESULT&", n.Condition, ok)
			}
			for id, want := range map[string]string{
				"33": "counter := &counter& + 1",
				"30": "flag := X",
				"47": "complete workflow",
			} {
				if n, ok := findNodeByID(d, id); !ok || n.Operation != want {
					t.Errorf("node %s operation: got %q (found=%v), want %q", id, n.Operation, ok, want)
				}
			}
			if n, ok := findNodeByID(d, "56"); !ok || n.Label != "2 of 2" {
				t.Errorf("join node 56 label: got %q (found=%v), want %q", n.Label, ok, "2 of 2")
			}

			// Sub-workflow at depth 1.
			var subFound bool
			for _, s := range d.Subworkflows {
				if s.WfdID == "WS48900024" {
					subFound = true
					if !s.Found {
						t.Errorf("sub-workflow WS48900024 present but found=false")
					}
				}
			}
			if !subFound {
				t.Errorf("sub-workflow WS48900024 not in subworkflows: %+v", d.Subworkflows)
			}

			// Container elements. None of them is import/export (PROPS), so only
			// presence, category and type are asserted.
			byName := map[string]bwf.ContainerElement{}
			for _, c := range d.Container {
				byName[c.Name] = c
			}
			for _, name := range []string{"ABSENCFORM", "COUNTER", "FLAG", "RESULT"} {
				if _, ok := byName[name]; !ok {
					t.Errorf("container element %s missing", name)
				}
			}
			if c := byName["ABSENCFORM"]; c.Category != "BO" || c.Type != "FORMABSENC" {
				t.Errorf("ABSENCFORM: got category %q type %q, want BO / FORMABSENC", c.Category, c.Type)
			}

			// Step 18 bindings.
			step, ok := findNodeByID(d, "18")
			if !ok {
				t.Fatalf("step node 18 missing")
			}
			var haveExport, haveImport bool
			for _, b := range step.Bindings {
				if b.Direction == "export" && b.StepElement == "FORMABSENC" && b.Expression == "&ABSENCFORM&" && b.BindType == "WW" {
					haveExport = true
				}
				if b.Direction == "import" && b.StepElement == "RESULT" && b.Expression == "&RESULT&" && b.BindType == "WW" {
					haveImport = true
				}
			}
			if !haveExport {
				t.Errorf("step 18: export FORMABSENC <- &ABSENCFORM& (WW) missing: %+v", step.Bindings)
			}
			if !haveImport {
				t.Errorf("step 18: import RESULT -> &RESULT& (WW) missing: %+v", step.Bindings)
			}

			// No logon IDs / change-info.
			if m := regexp.MustCompile(`[0-9]{14}`).FindString(raw); m != "" {
				t.Errorf("result contains a 14-digit run %q (timestamp from change-info?)", m)
			}
			for _, banned := range []string{"CREATED_BY", "CHANGED_BY", "ACTIVAT_BY", "UNAME", "CHGDTA"} {
				if strings.Contains(raw, banned) {
					t.Errorf("result contains forbidden string %q", banned)
				}
			}
		})
	}
}

func TestIntegration_WorkflowDefinition_Constants_ISU(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			q := callTool(t, sharedServer, "run_query", map[string]interface{}{
				"sql":     "SELECT swdsheader~task FROM swdsheader WHERE swdsheader~task = 'WS00288801'",
				"purpose": "development_metadata",
			})
			if q.IsError {
				t.Fatalf("run_query existence check failed: %s", textOf(q))
			}
			var qr struct {
				Rows [][]string `json:"rows"`
			}
			if err := json.Unmarshal([]byte(textOf(q)), &qr); err != nil {
				t.Fatalf("unmarshal run_query result: %v\nraw: %s", err, textOf(q))
			}
			if len(qr.Rows) == 0 {
				t.Skipf("WS00288801 not present — system has no IS-U common layer; constant resolution via workflow not verified on %s", sys)
			}

			d, _ := callWorkflowDefinition(t, map[string]interface{}{"wfd_id": "WS00288801"})
			if d.Version != "0002" {
				t.Errorf("version: got %q, want 0002", d.Version)
			}
			var haveTrue, haveUtilmd bool
			for _, n := range d.Nodes {
				if strings.Contains(n.Condition, "&CONSTANTS.GC_ABAP_TRUE&") && strings.Contains(n.ConditionResolved, "'X'") {
					haveTrue = true
				}
				if strings.Contains(n.Condition, "&CONSTANTS.GC_PROC_VAL_UTILMD&") && strings.Contains(n.ConditionResolved, "'UTILMD'") {
					haveUtilmd = true
				}
			}
			if !haveTrue {
				t.Errorf("no node resolves &CONSTANTS.GC_ABAP_TRUE& to 'X'")
			}
			if !haveUtilmd {
				t.Errorf("no node resolves &CONSTANTS.GC_PROC_VAL_UTILMD& to 'UTILMD'")
			}
		})
	}
}

// workflowQueryRows runs a development-metadata query and returns the rows.
func workflowQueryRows(t *testing.T, sql string) [][]string {
	t.Helper()
	q := callTool(t, sharedServer, "run_query", map[string]interface{}{
		"sql":     sql,
		"purpose": "development_metadata",
	})
	if q.IsError {
		t.Fatalf("run_query failed: %s\nsql: %s", textOf(q), sql)
	}
	var qr struct {
		Rows [][]string `json:"rows"`
	}
	if err := json.Unmarshal([]byte(textOf(q)), &qr); err != nil {
		t.Fatalf("unmarshal run_query result: %v\nraw: %s", err, textOf(q))
	}
	return qr.Rows
}

func TestIntegration_WorkflowDefinition_Runtime(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "get_workflow_definition", map[string]interface{}{
				"wfd_id": "WS48900002",
				"source": "runtime",
			})
			if res.IsError {
				if !strings.Contains(textOf(res), "no runtime copy") {
					t.Fatalf("tool error without \"no runtime copy\": %s", textOf(res))
				}
				t.Logf("WS48900002 has no runtime copy on this system (expected branch): %s", textOf(res))
				return
			}
			var d bwf.Result
			if err := json.Unmarshal([]byte(textOf(res)), &d); err != nil {
				t.Fatalf("unmarshal result: %v\nraw: %s", err, textOf(res))
			}
			if d.VersionSource != "runtime" {
				t.Errorf("version_source: got %q, want runtime", d.VersionSource)
			}
			if len(d.Nodes) == 0 {
				t.Errorf("runtime copy decoded with no nodes")
			}
		})
	}
}

func TestIntegration_WorkflowDefinition_Legacy(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			// Legacy-only: a client-dependent SWD copy (EXETYP 'S') with no
			// counterpart in SWDSHEADER.
			rows := workflowQueryRows(t, "SELECT swd_header~task FROM swd_header "+
				"WHERE swd_header~exetyp = 'S' AND NOT EXISTS "+
				"( SELECT swdsheader~task FROM swdsheader WHERE swdsheader~task = swd_header~task ) "+
				"ORDER BY swd_header~task")
			if len(rows) == 0 || len(rows[0]) == 0 {
				t.Fatalf("no legacy-only workflow task found (SWD_HEADER EXETYP 'S' not in SWDSHEADER); legacy path unverified on %s", sys)
			}
			task := strings.TrimSpace(rows[0][0])

			d, _ := callWorkflowDefinition(t, map[string]interface{}{"wfd_id": task})
			if d.VersionSource != "legacy_client" {
				t.Errorf("%s version_source: got %q, want legacy_client", task, d.VersionSource)
			}
			if len(d.Nodes) == 0 {
				t.Errorf("%s decoded with no nodes", task)
			}
		})
	}
}
