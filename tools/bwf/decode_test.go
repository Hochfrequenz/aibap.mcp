package bwf

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func loadModel(t *testing.T, tables map[string][]Row, wfdID, language string) *rawModel {
	t.Helper()
	m, err := loadSWDS(context.Background(), &fakeQuery{tables: tables}, wfdID, "0001", language, true)
	if err != nil {
		t.Fatalf("loadSWDS: %v", err)
	}
	return m
}

func decodeFixture(t *testing.T, language string) ([]Node, []string) {
	t.Helper()
	return decodeNodes(loadModel(t, loadFixture(t, "ws48900002"), "WS48900002", language), language)
}

func nodeByID(t *testing.T, nodes []Node, id string) Node {
	t.Helper()
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %q not found", id)
	return Node{}
}

func TestDecode_NodeCountRaw(t *testing.T) {
	m := loadModel(t, loadFixture(t, "ws48900002"), "WS48900002", "E")
	nodes, _ := decodeNodes(m, "E")
	if len(nodes) != 53 {
		t.Errorf("nodes = %d, want 53", len(nodes))
	}
	if e := rawEdges(m); len(e) != 56 {
		t.Errorf("edges = %d, want 56", len(e))
	}
	n := nodeByID(t, nodes, "36")
	want := Node{ID: "36", Type: "IF", Kind: "condition", Label: "Already Revised Once?",
		Condition: "&COUNTER& > 1", BlockID: "36", ParentBlockID: "23"}
	if n.ID != want.ID || n.Type != want.Type || n.Kind != want.Kind || n.Label != want.Label ||
		n.Condition != want.Condition || n.BlockID != want.BlockID || n.ParentBlockID != want.ParentBlockID {
		t.Errorf("node 36 = %+v, want %+v", n, want)
	}
}

func TestDecode_RawEdgeIDs(t *testing.T) {
	m := loadModel(t, loadFixture(t, "ws48900002"), "WS48900002", "E")
	found := false
	for _, e := range rawEdges(m) {
		if e.From == "36" && e.To == "37" {
			found = true
		}
		if strings.HasPrefix(e.From, "0") || strings.HasPrefix(e.To, "0") {
			t.Errorf("edge id with leading zero: %+v", e)
		}
	}
	if !found {
		t.Error("edge 36 -> 37 missing")
	}
}

func TestDecode_LoopCondition(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	n := nodeByID(t, nodes, "10")
	if n.Kind != "loop_until" || n.Condition != "&FLAG& = X" {
		t.Errorf("node 10 kind=%q condition=%q", n.Kind, n.Condition)
	}
}

func TestDecode_Operations(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	for id, want := range map[string]string{
		"33": "counter := &counter& + 1",
		"30": "flag := X",
		"15": "flag := ",
	} {
		n := nodeByID(t, nodes, id)
		if n.Operation != want || n.Kind != "container_operation" {
			t.Errorf("node %s operation=%q kind=%q, want %q", id, n.Operation, n.Kind, want)
		}
	}
}

func TestDecode_ProcessControl(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	n := nodeByID(t, nodes, "47")
	if n.Kind != "process_control" || n.Operation != "complete workflow" {
		t.Errorf("node 47 kind=%q operation=%q", n.Kind, n.Operation)
	}
}

func TestDecode_MultipleCondition(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	n := nodeByID(t, nodes, "23")
	if n.Kind != "multiple_condition" || n.Condition != "&RESULT&" {
		t.Errorf("node 23 kind=%q condition=%q", n.Kind, n.Condition)
	}
}

func TestDecode_StepAndSubworkflow(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	n := nodeByID(t, nodes, "4")
	if n.Kind != "step" || n.Task != "TS30200106" || n.Agent != "&_WF_INITIATOR&" {
		t.Errorf("node 4 = %+v", n)
	}
	n = nodeByID(t, nodes, "18")
	if n.Kind != "subworkflow" || n.Task != "WS48900024" {
		t.Errorf("node 18 = %+v", n)
	}
}

func TestDecode_Event(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	if n := nodeByID(t, nodes, "45"); n.Event != "FORMABSENC.DELETED" {
		t.Errorf("node 45 event = %q", n.Event)
	}
	if n := nodeByID(t, nodes, "64"); n.Kind != "trigger_event" || n.Event != "FLOWITEM.DELETED" {
		t.Errorf("node 64 kind=%q event=%q", n.Kind, n.Event)
	}
}

func TestDecode_KindsOfHelperNodes(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	for id, want := range map[string]string{
		"1": "start", "999999": "end", "5": "other", "55": "fork", "56": "join", "999998": "other",
	} {
		if n := nodeByID(t, nodes, id); n.Kind != want {
			t.Errorf("node %s kind = %q, want %q", id, n.Kind, want)
		}
	}
}

func TestDecode_Bindings(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	n := nodeByID(t, nodes, "18")
	want := []Binding{
		{Direction: "export", StepElement: "FORMABSENC", Expression: "&ABSENCFORM&", BindType: "WW"},
		{Direction: "import", StepElement: "RESULT", Expression: "&RESULT&", BindType: "WW"},
	}
	if len(n.Bindings) != len(want) {
		t.Fatalf("node 18 bindings = %+v", n.Bindings)
	}
	for i, w := range want {
		b := n.Bindings[i]
		if b.Direction != w.Direction || b.StepElement != w.StepElement || b.Expression != w.Expression || b.BindType != w.BindType {
			t.Errorf("binding %d = %+v, want %+v", i, b, w)
		}
	}
	if n := nodeByID(t, nodes, "36"); n.Bindings == nil {
		t.Error("bindings must be an empty slice, not nil")
	}
}

func TestDecode_LabelFallbacks(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	if n := nodeByID(t, nodes, "4"); n.Label != "Create Notification of Absence" {
		t.Errorf("node 4 label = %q", n.Label)
	}
	if n := nodeByID(t, nodes, "1"); n.Label != "STRT" {
		t.Errorf("node 1 label = %q, want raw type", n.Label)
	}
	// Node without ND text but with a task: task short text.
	tables := loadFixture(t, "ws48900002")
	var texts []Row
	for _, r := range tables["SWDSTEXT"] {
		if r["NODEID"] != "0000000042" {
			texts = append(texts, r)
		}
	}
	tables["SWDSTEXT"] = texts
	nodes = decodeNodesOf(t, tables, "E")
	if n := nodeByID(t, nodes, "42"); n.Label != "Delete notification of absence" {
		t.Errorf("node 42 label = %q, want HRS1000 short text", n.Label)
	}
}

func decodeNodesOf(t *testing.T, tables map[string][]Row, language string) []Node {
	t.Helper()
	nodes, _ := decodeNodes(loadModel(t, tables, "WS48900002", language), language)
	return nodes
}

func TestDecode_LanguageFallback(t *testing.T) {
	nodes, _ := decodeFixture(t, "D")
	if n := nodeByID(t, nodes, "36"); n.Label != "Bereits einmal überarbeitet ?" {
		t.Errorf("node 36 label (D) = %q", n.Label)
	}
	tables := loadFixture(t, "ws48900002")
	var texts []Row
	for _, r := range tables["SWDSTEXT"] {
		if r["LANGUAGE"] != "D" {
			texts = append(texts, r)
		}
	}
	tables["SWDSTEXT"] = texts
	nodes = decodeNodesOf(t, tables, "D")
	if n := nodeByID(t, nodes, "36"); n.Label != "Already Revised Once?" {
		t.Errorf("node 36 label (no D rows) = %q", n.Label)
	}
}

func TestDecode_TextQueryRestrictsLanguage(t *testing.T) {
	f := &fakeQuery{tables: loadFixture(t, "ws48900002")}
	if _, err := loadSWDS(context.Background(), f, "WS48900002", "0001", "E", false); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, sql := range f.calls {
		if strings.Contains(sql, "swdstext") {
			n++
			if !strings.Contains(sql, "language IN ('E', 'D')") {
				t.Errorf("text query not restricted: %s", sql)
			}
		}
	}
	if n != 1 {
		t.Errorf("text queries = %d, want 1", n)
	}
}

func TestDecode_NoLogonIDInOutput(t *testing.T) {
	nodes, _ := decodeFixture(t, "E")
	b, err := json.Marshal(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SAPUSER") || regexp.MustCompile(`\d{14}`).Match(b) {
		t.Errorf("output leaks change info: %s", b)
	}
}

func TestDecode_OrphanTextsIgnored(t *testing.T) {
	nodes, warnings := decodeFixture(t, "E")
	for _, n := range nodes {
		if n.ID == "70" {
			t.Error("orphan text produced node 70")
		}
	}
	n := 0
	for _, w := range warnings {
		if strings.Contains(w, "SWDSTEXT") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("text warnings = %d, want 1 (%v)", n, warnings)
	}
}

func TestDecode_UnknownContCmd(t *testing.T) {
	tables := loadFixture(t, "ws48900002")
	for _, r := range tables["SWDSMNODES"] {
		if r["WFD_ID"] == "WS48900002" && r["NODEID"] == "0000000033" {
			r["CONT_CMD"] = "99"
		}
	}
	m := loadModel(t, tables, "WS48900002", "E")
	nodes, warnings := decodeNodes(m, "E")
	if n := nodeByID(t, nodes, "33"); !strings.HasPrefix(n.Operation, "cmd 99") {
		t.Errorf("node 33 operation = %q", n.Operation)
	}
	found := false
	for _, w := range warnings {
		found = found || strings.Contains(w, "99")
	}
	if !found {
		t.Errorf("no warning for command 99: %v", warnings)
	}
}

func TestDecode_ChangeInfoVariants(t *testing.T) {
	m := &rawModel{
		Model: "SWDS", WfdID: "WS1", Version: "0001", MasterLanguage: "E",
		Nodes: []rawNode{{ID: "0000000001", Type: "IF"}, {ID: "0000000002", Type: "IF"}},
		Conds: []Row{
			{"NODEID": "0000000001", "LINE_ID": "000", "LINE_TYP": "", "LEFTEXPR": "620 SAPUSER     20020122162144", "COMPOP": ""},
			{"NODEID": "0000000001", "LINE_ID": "001", "LINE_TYP": "-", "LEFTEXPR": "&A&", "COMPOP": "=", "RIGHTEXPR": "'312'{TYPE=C}"},
			{"NODEID": "0000000002", "LINE_ID": "000", "LINE_TYP": "E", "LEFTEXPR": "001EXP"},
			{"NODEID": "0000000002", "LINE_ID": "001", "LINE_TYP": "", "LEFTEXPR": "620 SAPUSER     20020122162144", "COMPOP": ""},
			{"NODEID": "0000000002", "LINE_ID": "001", "LINE_TYP": "-", "LEFTEXPR": "&B&", "COMPOP": "NX"},
		},
	}
	nodes, _ := decodeNodes(m, "E")
	if c := nodeByID(t, nodes, "1").Condition; c != "&A& = '312'" {
		t.Errorf("node 1 condition = %q", c)
	}
	if c := nodeByID(t, nodes, "2").Condition; c != "&B& is initial" {
		t.Errorf("node 2 condition = %q", c)
	}
}

func TestDecode_ConditionJoin(t *testing.T) {
	m := &rawModel{
		Nodes: []rawNode{{ID: "0000000001", Type: "IF"}},
		Conds: []Row{
			{"NODEID": "0000000001", "LINE_ID": "002", "LINE_TYP": "-", "LEFTEXPR": "&B&", "COMPOP": "EX", "LOGOP": "or", "NOTFLAG": "X"},
			{"NODEID": "0000000001", "LINE_ID": "001", "LINE_TYP": "-", "LEFTEXPR": "&A&", "COMPOP": "=", "RIGHTEXPR": "1", "LOGOP": "and"},
		},
	}
	nodes, _ := decodeNodes(m, "E")
	if c := nodes[0].Condition; c != "&A& = 1 OR NOT &B& is filled" {
		t.Errorf("condition = %q", c)
	}
}

func TestDecode_RejectsBadKeys(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{}}
	for _, c := range [][3]string{{"WS1' OR '1", "0001", "E"}, {"WS48900002", "1", "E"}, {"WS48900002", "0001", "EN"}} {
		if _, err := loadSWDS(context.Background(), f, c[0], c[1], c[2], false); err == nil {
			t.Errorf("loadSWDS(%v) accepted bad input", c)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("queries sent: %v", f.calls)
	}
}
