package bwf

import (
	"reflect"
	"testing"
)

// collapseFixture runs a fixture through loadSWDS → decodeNodes → rawEdges → collapse.
func collapseFixture(t *testing.T, tables map[string][]Row, wfdID, version string) ([]Node, []Edge) {
	t.Helper()
	m := loadModelVersion(t, tables, wfdID, version)
	nodes, _ := decodeNodes(m, "E")
	return collapse(m, nodes, rawEdges(m))
}

func loadModelVersion(t *testing.T, tables map[string][]Row, wfdID, version string) *rawModel {
	t.Helper()
	m, err := loadSWDS(t.Context(), &fakeQuery{tables: tables}, wfdID, version, "E", false)
	if err != nil {
		t.Fatalf("loadSWDS: %v", err)
	}
	return m
}

func collapseWS48900002(t *testing.T) ([]Node, []Edge) {
	t.Helper()
	return collapseFixture(t, loadFixture(t, "ws48900002"), testWfdMain, "0001")
}

// collapseWhile uses WS00400012 version 0004 (the active version on the S/4 system).
func collapseWhile(t *testing.T) ([]Node, []Edge) {
	t.Helper()
	return collapseFixture(t, loadFixture(t, "ws00400012_while"), "WS00400012", "0004")
}

func requireEdge(t *testing.T, edges []Edge, from, to, label string) {
	t.Helper()
	for _, e := range edges {
		if e.From == from && e.To == to {
			if e.Label != label {
				t.Errorf("edge %s→%s: label %q, want %q", from, to, e.Label, label)
			}
			return
		}
	}
	t.Errorf("edge %s→%s %q missing; edges: %v", from, to, label, edges)
}

func edgesFrom(edges []Edge, from string) []Edge {
	var out []Edge
	for _, e := range edges {
		if e.From == from {
			out = append(out, e)
		}
	}
	return out
}

func TestCollapse_NoHelperNodes(t *testing.T) {
	nodes, edges := collapseWS48900002(t)
	helpers := map[string]bool{}
	for _, typ := range []string{"XOR", "ECAS", "ENDI", "UNJO", "IFRS", "UNRS", "CRES", "COTH",
		"EXCP", "TEVT", "CEVT", "EEVT", "EFUN", "EVTG", "OFF"} {
		helpers[typ] = true
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		if helpers[n.Type] {
			t.Errorf("helper node %s (%s) remains", n.ID, n.Type)
		}
		ids[n.ID] = true
	}
	if n := nodeByID(t, nodes, "999999"); n.Kind != "end" {
		t.Errorf("END node kind %q, want end", n.Kind)
	}
	for _, e := range edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("edge %s→%s references a removed node", e.From, e.To)
		}
	}
}

func TestCollapse_ConditionEdges(t *testing.T) {
	_, edges := collapseWS48900002(t)
	requireEdge(t, edges, "36", "42", "true")
	requireEdge(t, edges, "36", "50", "false")
}

func TestCollapse_CaseEdges(t *testing.T) {
	_, edges := collapseWS48900002(t)
	requireEdge(t, edges, "23", "30", "A")
	requireEdge(t, edges, "23", "33", "R")
	requireEdge(t, edges, "23", "10", "others")
	if got := len(edgesFrom(edges, "23")); got != 3 {
		t.Errorf("case node 23 has %d outgoing edges, want 3", got)
	}
}

func TestCollapse_StepEdges(t *testing.T) {
	_, edges := collapseWS48900002(t)
	requireEdge(t, edges, "4", "15", "")  // step → (UNJO) → first loop body node
	requireEdge(t, edges, "33", "36", "") // container operation, single "done" outcome
	requireEdge(t, edges, "56", "999999", "")
}

func TestCollapse_UntilLoop(t *testing.T) {
	_, edges := collapseWS48900002(t)
	requireEdge(t, edges, "10", "55", "true")
	requireEdge(t, edges, "10", "15", "false")
	for _, e := range edges {
		if e.To == "9" || e.From == "9" {
			t.Errorf("edge %s→%s touches the UNJO node 9", e.From, e.To)
		}
	}
}

func TestCollapse_ForkJoin(t *testing.T) {
	nodes, edges := collapseWS48900002(t)
	j := nodeByID(t, nodes, "56")
	if j.Kind != "join" || j.Label != "2 of 2" {
		t.Errorf("join node 56: kind %q label %q, want join / \"2 of 2\"", j.Kind, j.Label)
	}
	requireEdge(t, edges, "55", "60", "")
	requireEdge(t, edges, "55", "64", "")
}

func TestCollapse_TerminatingEvent(t *testing.T) {
	_, edges := collapseWS48900002(t)
	requireEdge(t, edges, "42", "47", "Form deleted")

	tables := loadFixture(t, "ws48900002")
	for i, r := range tables["SWDSMNODES"] {
		if r["WFD_ID"] == testWfdMain && r["NODEID"] == "0000000045" {
			c := Row{}
			for k, v := range r {
				c[k] = v
			}
			c["EVT_NAME"] = "DELETED"
			tables["SWDSMNODES"][i] = c
		}
	}
	_, edges = collapseFixture(t, tables, testWfdMain, "0001")
	requireEdge(t, edges, "42", "47", "event DELETED")
}

func TestCollapse_ExceptionLabel(t *testing.T) {
	tables := loadFixture(t, "ws48900002")
	for i, r := range tables["SWDSMNODES"] {
		if r["WFD_ID"] == testWfdMain && r["NODEID"] == "0000000053" {
			c := Row{}
			for k, v := range r {
				c[k] = v
			}
			c["EXC_CLSNAM"] = "CX_EXAMPLE"
			tables["SWDSMNODES"][i] = c
		}
	}
	_, edges := collapseFixture(t, tables, testWfdMain, "0001")
	requireEdge(t, edges, "50", "10", "exception CX_EXAMPLE")
}

func TestCollapse_WhileLoop(t *testing.T) {
	nodes, edges := collapseWhile(t)
	w := nodeByID(t, nodes, "17")
	if w.Type != "WHIL" || w.Kind != "loop_while" {
		t.Errorf("node 17: type %q kind %q, want WHIL / loop_while", w.Type, w.Kind)
	}
	requireEdge(t, edges, "17", "42", "true") // WHRS "Continue" → body fork
	requireEdge(t, edges, "17", "5", "false") // WOTH "Exit" → join after the loop
	requireEdge(t, edges, "43", "17", "")     // body end → EWHI → WHJO → WHIL
	requireEdge(t, edges, "4", "17", "")      // loop entry: edges into WHJO point to WHIL
	if got := len(edgesFrom(edges, "17")); got != 2 {
		t.Errorf("WHIL node has %d outgoing edges, want 2", got)
	}
	for _, n := range nodes {
		switch n.Type {
		case "WHJO", "WHRS", "WOTH", "EWHI":
			t.Errorf("helper node %s (%s) remains", n.ID, n.Type)
		}
	}
}

func TestCollapse_RawModeUntouched(t *testing.T) {
	m := loadModelVersion(t, loadFixture(t, "ws48900002"), testWfdMain, "0001")
	nodes, _ := decodeNodes(m, "E")
	edges := rawEdges(m)
	wantNodes := append([]Node(nil), nodes...)
	wantEdges := append([]Edge(nil), edges...)
	collapse(m, nodes, edges)
	if !reflect.DeepEqual(nodes, wantNodes) {
		t.Error("collapse modified its input nodes")
	}
	if !reflect.DeepEqual(edges, wantEdges) {
		t.Error("collapse modified its input edges")
	}
}

func TestCollapse_ConvergingOutcomes(t *testing.T) {
	// FUNC 1 → XOR 2 → EXCP 3 (exception) / EXCP 4 (completed) → ECAS 5 → FUNC 6
	m := &rawModel{Nodes: []rawNode{
		{ID: "1", Type: "FUNC"}, {ID: "2", Type: "XOR"},
		{ID: "3", Type: "EXCP", ExcClass: "CX_EXAMPLE"}, {ID: "4", Type: "EXCP"},
		{ID: "5", Type: "ECAS"}, {ID: "6", Type: "FUNC"},
	}}
	var nodes []Node
	for _, rn := range m.Nodes {
		nodes = append(nodes, Node{ID: rn.ID, Type: rn.Type, Label: rn.Type})
	}
	edges := []Edge{{From: "1", To: "2"}, {From: "2", To: "3"}, {From: "2", To: "4"},
		{From: "3", To: "5"}, {From: "4", To: "5"}, {From: "5", To: "6"}}
	_, got := collapse(m, nodes, edges)
	want := []Edge{{From: "1", To: "6", Label: "exception CX_EXAMPLE"}, {From: "1", To: "6"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("edges %v, want %v", got, want)
	}
}
