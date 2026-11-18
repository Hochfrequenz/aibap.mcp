package bwf

import (
	"strings"
	"testing"
)

func loadRuntime(t *testing.T, tables map[string][]Row) *rawModel {
	t.Helper()
	m, err := loadSWD(t.Context(), &fakeQuery{tables: tables}, "WS48900002", "0000", "E", "E", true)
	if err != nil {
		t.Fatalf("loadSWD: %v", err)
	}
	return m
}

func edgeLabel(t *testing.T, edges []Edge, from, to string) string {
	t.Helper()
	for _, e := range edges {
		if e.From == from && e.To == to {
			return e.Label
		}
	}
	t.Fatalf("edge %s -> %s not found in %+v", from, to, edges)
	return ""
}

func TestLoadSWD_RuntimeCopy(t *testing.T) {
	m := loadRuntime(t, loadFixture(t, "ws48900002_runtime"))
	if m.Model != "SWD" || len(m.Nodes) != 26 || len(m.Lines) != 31 {
		t.Fatalf("model=%q nodes=%d lines=%d, want SWD/26/31", m.Model, len(m.Nodes), len(m.Lines))
	}
	if m.MasterLanguage != "D" || m.ActivatedOn != "20050825" {
		t.Errorf("header = %q %q", m.MasterLanguage, m.ActivatedOn)
	}
	nodes, _ := decodeNodes(m, "E")
	if len(nodes) != 26 {
		t.Fatalf("raw nodes = %d, want 26", len(nodes))
	}
	n := nodeByID(t, nodes, "36")
	if n.Type != "I" || n.Kind != "condition" || n.Condition != "&COUNTER& > 1" || n.Label != "Already Revised Once?" {
		t.Errorf("node 36 = %+v", n)
	}
	edges := rawEdges(m)
	// Confirmed against SWDSMNODES IFRS/UNRS RETURN of the same node ids (see swdReturnCodes).
	for _, c := range []struct{ from, to, want string }{
		{"36", "42", "true"}, {"36", "50", "false"},
		{"10", "55", "true"}, {"10", "9", "false"},
		{"23", "30", "A"}, {"23", "33", "R"}, {"23", "67", "others"},
		{"1", "4", ""}, {"55", "60", ""},
		{"42", "44", "event FORMABSENC.DELETED"},
	} {
		if got := edgeLabel(t, edges, c.from, c.to); got != c.want {
			t.Errorf("edge %s -> %s label = %q, want %q", c.from, c.to, got, c.want)
		}
	}
	kinds := map[string]string{"1": "start", "999999": "end", "4": "step", "18": "subworkflow",
		"10": "condition", "23": "multiple_condition", "15": "container_operation",
		"47": "process_control", "55": "fork", "56": "join", "64": "trigger_event", "6": "other", "9": "other"}
	for id, want := range kinds {
		if got := nodeByID(t, nodes, id); got.Kind != want {
			t.Errorf("node %s (%s) kind = %q, want %q", id, got.Type, got.Kind, want)
		}
	}
	if got := nodeByID(t, nodes, "18"); got.Task != "WS48900024" {
		t.Errorf("node 18 task = %q", got.Task)
	}
	if got := nodeByID(t, nodes, "4"); got.Task != "TS30200106" || got.Agent != "&_WF_INITIATOR&" || len(got.Bindings) != 1 {
		t.Errorf("node 4 = %+v", got)
	}
	if got := nodeByID(t, nodes, "47"); got.Operation != "complete workflow" {
		t.Errorf("node 47 operation = %q", got.Operation)
	}
	if got := nodeByID(t, nodes, "23"); got.Condition != "&RESULT&" {
		t.Errorf("node 23 condition = %q", got.Condition)
	}
	if got := nodeByID(t, nodes, "33"); got.Operation != "counter := &counter& + 1" {
		t.Errorf("node 33 operation = %q", got.Operation)
	}
	if got := nodeByID(t, nodes, "64"); got.Event != "FLOWITEM.DELETED" {
		t.Errorf("node 64 event = %q", got.Event)
	}
}

func TestLoadSWD_ContainerOperationFromParameters(t *testing.T) {
	tables := loadFixture(t, "ws48900002_runtime")
	var keep []Row
	for _, r := range tables["SWD_EXPR"] {
		if r["NODEID"] != "0000000030" {
			keep = append(keep, r)
		}
	}
	tables["SWD_EXPR"] = keep
	nodes, _ := decodeNodes(loadRuntime(t, tables), "E")
	if got := nodeByID(t, nodes, "30"); got.Operation != "flag := X" {
		t.Errorf("node 30 operation = %q, want %q", got.Operation, "flag := X")
	}
}

func TestLoadSWD_IgnoresOtherExecutionTypes(t *testing.T) {
	tables := loadFixture(t, "ws48900002_runtime")
	tables["SWD_NODES"] = append(tables["SWD_NODES"], Row{"WFD_ID": "WS48900002", "VERSION": "0000",
		"EXETYP": "S", "NODEID": "0000000099", "NODETYPE": "S"})
	if m := loadRuntime(t, tables); len(m.Nodes) != 26 {
		t.Errorf("nodes = %d, want 26 (EXETYP S row must be ignored)", len(m.Nodes))
	}
}

func TestLoadSWD_QueriesKeyByExecutionType(t *testing.T) {
	f := &fakeQuery{tables: loadFixture(t, "ws48900002_runtime")}
	if _, err := loadSWD(t.Context(), f, "WS48900002", "0000", "E", "E", true); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		lc := strings.ToLower(c)
		if strings.Contains(lc, "swds") {
			t.Errorf("runtime copy read a client-independent table: %s", c)
		}
		if strings.Contains(lc, " from swd_") && !strings.Contains(lc, "exetyp = 'e'") {
			t.Errorf("query without EXETYP key: %s", c)
		}
		if strings.Contains(lc, "client") {
			t.Errorf("query names CLIENT: %s", c)
		}
	}
}

// Legacy definitions (EXETYP 'S') are stored in SWD_MNODES/SWD_MLINES with the
// same block model as SWDSMNODES; verified live, SWD_NODES holds only EXETYP 'E'.
func TestLoadSWD_Legacy(t *testing.T) {
	f := &fakeQuery{tables: loadFixture(t, "legacy")}
	m, err := loadSWD(t.Context(), f, "0000002303", "0000", "S", "E", true)
	if err != nil {
		t.Fatal(err)
	}
	if m.Model != "SWDS" || len(m.Nodes) != 26 || len(m.Lines) != 28 {
		t.Fatalf("model=%q nodes=%d lines=%d", m.Model, len(m.Nodes), len(m.Lines))
	}
	nodes, _ := decodeNodes(m, "E")
	if got := nodeByID(t, nodes, "60"); got.Kind != "subworkflow" || got.Task != "WS00000023" {
		t.Errorf("node 60 = %+v", got)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.ToLower(c), "swds") {
			t.Errorf("legacy definition read a client-independent table: %s", c)
		}
	}
}

func TestLoadSWD_RejectsBadKeys(t *testing.T) {
	f := &fakeQuery{}
	for _, c := range [][4]string{
		{"WS48900002'", "0000", "E", "E"},
		{"WS48900002", "00", "E", "E"},
		{"WS48900002", "0000", "C", "E"},
		{"WS48900002", "0000", "E", "EN"},
	} {
		if _, err := loadSWD(t.Context(), f, c[0], c[1], c[2], c[3], true); err == nil {
			t.Errorf("%v: expected error", c)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("queries sent: %v", f.calls)
	}
}
