package bwf

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

func getOpts(task string) Options {
	return Options{Task: task, Source: "definition", Language: "E", Container: true,
		Today: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)}
}

func getFake(t *testing.T) *fakeQuery {
	t.Helper()
	return &fakeQuery{tables: loadFixture(t, "ws48900002")}
}

func TestGet_Root(t *testing.T) {
	res, err := Get(t.Context(), getFake(t), getOpts(testWfdMain))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.Version != "0001" || res.VersionSource != testSourceHRS || res.Title == "" {
		t.Errorf("version=%q source=%q title=%q", res.Version, res.VersionSource, res.Title)
	}
	if res.Task != testWfdMain || res.WfdID != testWfdMain || res.SourceTables != modelSWDS || res.Language != "E" {
		t.Errorf("header fields = %+v", res.Definition)
	}
	if res.ActivatedOn != "2002-01-22" {
		t.Errorf("ActivatedOn = %q", res.ActivatedOn)
	}
	if len(res.Calls) != 1 || res.Calls[0] != (Call{Node: "18", WfdID: testWfdSub}) {
		t.Errorf("Calls = %+v", res.Calls)
	}
	if len(res.Subworkflows) != 0 {
		t.Errorf("Subworkflows = %+v, want none at depth 0", res.Subworkflows)
	}
	if len(res.Container) == 0 {
		t.Errorf("container empty")
	}
}

func TestGet_Depth1(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Depth = 1
	res, err := Get(t.Context(), getFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 {
		t.Fatalf("Subworkflows = %+v", res.Subworkflows)
	}
	s := res.Subworkflows[0]
	if s.WfdID != testWfdSub || !s.Found || s.Depth != 1 || strings.Join(s.CalledBy, ",") != testWfdMain {
		t.Errorf("sub = %+v", s)
	}
	if s.Definition == nil || len(s.Definition.Nodes) == 0 {
		t.Errorf("sub definition missing nodes")
	}
}

func cyclicFake(t *testing.T) *fakeQuery {
	t.Helper()
	f := getFake(t)
	rows := make([]Row, len(f.tables["SWDSMNODES"]))
	for i, r := range f.tables["SWDSMNODES"] {
		c := Row{}
		for k, v := range r {
			c[k] = v
		}
		if c["WFD_ID"] == testWfdSub && c["NODEID"] == "0000000004" {
			c["TASK"] = testWfdMain
		}
		rows[i] = c
	}
	f.tables["SWDSMNODES"] = rows
	return f
}

func TestGet_Cycle(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Depth = 5
	res, err := Get(t.Context(), cyclicFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 {
		t.Fatalf("Subworkflows = %d, want 1", len(res.Subworkflows))
	}
	if c := res.Subworkflows[0].Definition.Calls; len(c) != 1 || c[0].WfdID != testWfdMain {
		t.Errorf("sub calls = %+v, want call back to root", c)
	}
}

func TestGet_MissingSub(t *testing.T) {
	f := getFake(t)
	for _, tbl := range []string{"SWDSHEADER", testSourceHRS} {
		var keep []Row
		for _, r := range f.tables[tbl] {
			if r["WFD_ID"] != testWfdSub {
				keep = append(keep, r)
			}
		}
		f.tables[tbl] = keep
	}
	o := getOpts(testWfdMain)
	o.Depth = 1
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 || res.Subworkflows[0].Found || res.Subworkflows[0].Definition != nil {
		t.Fatalf("Subworkflows = %+v", res.Subworkflows)
	}
	found := false
	for _, w := range res.Warnings {
		found = found || strings.Contains(w, testWfdSub)
	}
	if !found {
		t.Errorf("warnings = %v, want one naming WS48900024", res.Warnings)
	}
}

func TestGet_Raw(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Raw = true
	res, err := Get(t.Context(), getFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Nodes) != 53 {
		t.Errorf("raw nodes = %d, want 53", len(res.Nodes))
	}
}

func TestGet_ContainerOff(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Container = false
	f := getFake(t)
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.Container == nil || len(res.Container) != 0 {
		t.Errorf("Container = %#v, want empty slice", res.Container)
	}
	for _, c := range f.calls {
		if strings.Contains(strings.ToLower(c), "swdscont") {
			t.Errorf("unexpected container query: %s", c)
		}
	}
}

func TestGet_Mermaid(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Mermaid = true
	res, err := Get(t.Context(), getFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.HasPrefix(res.Mermaid, "flowchart TD") || !strings.Contains(res.Mermaid, "-->|true|") {
		t.Errorf("mermaid = %q", res.Mermaid)
	}
	o.Mermaid = false
	res, _ = Get(t.Context(), getFake(t), o)
	if res.Mermaid != "" {
		t.Errorf("mermaid not empty when off")
	}
}

func TestRenderMermaid_Escapes(t *testing.T) {
	d := &Definition{Nodes: []Node{{ID: "1", Label: `say "hi"`}, {ID: "2", Label: "b"}},
		Edges: []Edge{{From: "1", To: "2", Label: "x"}, {From: "2", To: "1"}}}
	got := renderMermaid(d)
	for _, want := range []string{"flowchart TD", `n1["say #quot;hi#quot;"]`, "n1 -->|x| n2", "n2 --> n1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestClampDepth(t *testing.T) {
	for in, want := range map[int]int{99: maxDepth, 6: maxDepth, 5: 5, 3: 3, 0: 0, -1: 0} {
		if got := clampDepth(in); got != want {
			t.Errorf("clampDepth(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestGet_ContainerOffStillResolvesConstants(t *testing.T) {
	f := getFake(t)
	conds := make([]Row, 0, len(f.tables["SWDSCONDEF"]))
	for _, r := range f.tables["SWDSCONDEF"] {
		c := Row{}
		for k, v := range r {
			c[k] = v
		}
		if c["NODEID"] == "0000000010" && c["LINE_TYP"] == "-" {
			c["RIGHTEXPR"] = "&CONSTANTS.GC_ABAP_TRUE&"
		}
		conds = append(conds, c)
	}
	f.tables["SWDSCONDEF"] = conds
	f.tables["SWDSCNTIDX"] = append(f.tables["SWDSCNTIDX"], Row{"OTYPE": "WS", "OBJID": "48900002",
		"WFD_ID": testWfdMain, "VERSION": "0001", "ELEMENT": "CONSTANTS", "TYPEID": "/ABC/CL_EXAMPLE",
		"CATID": "CL", "TYPEKIND": "r", "TYPENAME": ""})
	cls := loadFixture(t, "constants_class")
	f.tables["SEOCOMPODF"], f.tables["SEOMETAREL"] = cls["SEOCOMPODF"], cls["SEOMETAREL"]

	o := getOpts(testWfdMain)
	o.Container = false
	o.Raw = true
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var n *Node
	for i := range res.Nodes {
		if res.Nodes[i].ID == "10" {
			n = &res.Nodes[i]
		}
	}
	if n == nil {
		t.Fatalf("node 10 missing")
	}
	if n.Condition != "&FLAG& = &CONSTANTS.GC_ABAP_TRUE&" || n.ConditionResolved != "&FLAG& = 'X'" {
		t.Errorf("condition = %q, resolved = %q", n.Condition, n.ConditionResolved)
	}
	if len(res.UnresolvedConstants) != 0 {
		t.Errorf("unresolved = %v", res.UnresolvedConstants)
	}
	if len(res.Container) != 0 {
		t.Errorf("Container = %+v, want empty", res.Container)
	}
}

func TestGet_CalleePointerToMissingVersion(t *testing.T) {
	f := getFake(t)
	ptr := make([]Row, 0, len(f.tables[testSourceHRS]))
	for _, r := range f.tables[testSourceHRS] {
		c := Row{}
		for k, v := range r {
			c[k] = v
		}
		if c["WFD_ID"] == testWfdSub {
			c["VERSION"] = "0007"
		}
		ptr = append(ptr, c)
	}
	f.tables[testSourceHRS] = ptr
	requireUnresolvedSub(t, f, testWfdSub)
}

func TestGet_EmptySlicesSerialiseAsArrays(t *testing.T) {
	res := &Result{}
	res.normalise()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"calls":[]`, `"subworkflows":[]`, `"nodes":[]`, `"edges":[]`,
		`"container":[]`, `"warnings":[]`, `"unresolved_constants":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	res = &Result{Subworkflows: []Subworkflow{{Definition: &Definition{Nodes: []Node{{}}}}}}
	res.normalise()
	b, _ = json.Marshal(res)
	for _, want := range []string{`"bindings":[]`, `"called_by":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("null in %s", b)
	}
}

func TestGet_RealResultHasNoNull(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Depth = 1
	res, err := Get(t.Context(), getFake(t), o)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "null") {
		t.Errorf("null in output")
	}
}

func withCallee(t *testing.T, task string) *fakeQuery {
	t.Helper()
	f := getFake(t)
	rows := make([]Row, len(f.tables["SWDSMNODES"]))
	for i, r := range f.tables["SWDSMNODES"] {
		c := Row{}
		for k, v := range r {
			c[k] = v
		}
		if c["WFD_ID"] == testWfdMain && c["NODEID"] == "0000000018" {
			c["TASK"] = task
		}
		rows[i] = c
	}
	f.tables["SWDSMNODES"] = rows
	return f
}

func requireUnresolvedSub(t *testing.T, f *fakeQuery, callee string) {
	t.Helper()
	o := getOpts(testWfdMain)
	o.Depth = 1
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.Task != testWfdMain || len(res.Nodes) == 0 {
		t.Errorf("root definition missing")
	}
	if len(res.Subworkflows) != 1 || res.Subworkflows[0].WfdID != callee || res.Subworkflows[0].Found {
		t.Fatalf("Subworkflows = %+v", res.Subworkflows)
	}
	ok := false
	for _, w := range res.Warnings {
		ok = ok || strings.Contains(w, callee)
	}
	if !ok {
		t.Errorf("warnings = %v, want one naming %s", res.Warnings, callee)
	}
}

func TestGet_InvalidCalleeID(t *testing.T) {
	requireUnresolvedSub(t, withCallee(t, "WS123"), "WS123")
}

func TestGet_LegacyOnlyCallee(t *testing.T) {
	f := withCallee(t, "WS00000024")
	for k, v := range loadFixture(t, "legacy") {
		f.tables[k] = append(f.tables[k], v...)
	}
	o := getOpts(testWfdMain)
	o.Depth = 1
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 {
		t.Fatalf("Subworkflows = %+v", res.Subworkflows)
	}
	s := res.Subworkflows[0]
	if s.WfdID != "WS00000024" || !s.Found || s.Definition == nil {
		t.Fatalf("sub = %+v", s)
	}
	if s.Definition.VersionSource != sourceLegacy || s.Definition.SourceTables != modelSWD || len(s.Definition.Nodes) == 0 {
		t.Errorf("sub definition = %+v", s.Definition)
	}
}

type failingQuery struct {
	*fakeQuery
	on string
}

func (f failingQuery) RunQuery(ctx context.Context, sql string, n int) (*adt.QueryResult, error) {
	if strings.Contains(sql, f.on) {
		return nil, errors.New("boom")
	}
	return f.fakeQuery.RunQuery(ctx, sql, n)
}

func TestGet_SubFetchErrorIsFatal(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Depth = 1
	_, err := Get(t.Context(), failingQuery{getFake(t), testWfdSub}, o)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want fetch failure", err)
	}
}

// runtimeFake holds the client-independent definition and the runtime copy.
func runtimeFake(t *testing.T) *fakeQuery {
	t.Helper()
	f := getFake(t)
	for k, v := range loadFixture(t, "ws48900002_runtime") {
		f.tables[k] = append(f.tables[k], v...)
	}
	return f
}

func TestGet_SourceRuntime(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Source = sourceRuntime
	f := runtimeFake(t)
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.VersionSource != sourceRuntime || res.SourceTables != modelSWD || res.Version != "0000" ||
		res.RuntimeVersion != "0000" || res.WfdID != testWfdMain || res.Task != testWfdMain {
		t.Errorf("header = %+v", res.Definition)
	}
	if res.ActivatedOn != "2005-08-25" || res.Title != "Workflow for Testing the BPML Export" {
		t.Errorf("activated=%q title=%q", res.ActivatedOn, res.Title)
	}
	if len(res.Nodes) != 26 || len(res.Container) != 4 {
		t.Errorf("nodes=%d container=%d", len(res.Nodes), len(res.Container))
	}
	if got := edgeLabel(t, res.Edges, "36", "42"); got != "true" {
		t.Errorf("36 -> 42 = %q", got)
	}
	for _, e := range res.Edges {
		if e.To == "999502" {
			t.Errorf("edge to a node that does not exist: %+v", e)
		}
	}
	if j := nodeByID(t, res.Nodes, "56"); j.Label != "2 of 2" {
		t.Errorf("join label = %q", j.Label)
	}
	if len(res.Calls) != 1 || res.Calls[0] != (Call{Node: "18", WfdID: testWfdSub}) {
		t.Errorf("calls = %+v", res.Calls)
	}
	for _, c := range f.calls {
		lc := strings.ToLower(c)
		if strings.Contains(lc, "swdsmnodes") || strings.Contains(lc, "swd_mnodes") {
			t.Errorf("runtime read the block-model node table: %s", c)
		}
	}
	o.Raw = true
	res, err = Get(t.Context(), runtimeFake(t), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edges) != 31 {
		t.Errorf("raw edges = %d, want 31", len(res.Edges))
	}
}

func TestGet_SourceRuntimeBlockMarkersDropped(t *testing.T) {
	f := runtimeFake(t)
	// Insert an X/Y block between nodes 4 and 6: 4 -> 5(X) -> 6.
	f.tables["SWD_NODES"] = append(f.tables["SWD_NODES"], Row{"WFD_ID": testWfdMain, "VERSION": "0000",
		"EXETYP": "E", "NODEID": "0000000005", "NODETYPE": "X"})
	var lines []Row
	for _, r := range f.tables["SWD_LINES"] {
		if r["PRED_NODE"] == "0000000004" && r["SUCC_NODE"] == "0000000006" {
			lines = append(lines,
				Row{"WFD_ID": testWfdMain, "VERSION": "0000", "EXETYP": "E", "PRED_NODE": "0000000004", "SUCC_NODE": "0000000005", "LINE_TYPE": "A", "RETURNCODE": "0000"},
				Row{"WFD_ID": testWfdMain, "VERSION": "0000", "EXETYP": "E", "PRED_NODE": "0000000005", "SUCC_NODE": "0000000006", "LINE_TYPE": "A", "RETURNCODE": "0000"})
			continue
		}
		lines = append(lines, r)
	}
	f.tables["SWD_LINES"] = lines
	o := getOpts(testWfdMain)
	o.Source = sourceRuntime
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes {
		if n.ID == "5" {
			t.Errorf("block marker kept: %+v", n)
		}
	}
	edgeLabel(t, res.Edges, "4", "6")
	o.Raw = true
	res, err = Get(t.Context(), f, o)
	if err != nil {
		t.Fatal(err)
	}
	nodeByID(t, res.Nodes, "5")
}

func TestGet_SourceRuntimeMissing(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Source = sourceRuntime
	_, err := Get(t.Context(), getFake(t), o)
	if err == nil || !strings.Contains(err.Error(), "no runtime copy of WS48900002 in the logon client") {
		t.Errorf("err = %v", err)
	}
}

func TestGet_SourceRuntimeOtherVersionRequested(t *testing.T) {
	o := getOpts(testWfdMain)
	o.Source = sourceRuntime
	o.Version = "0001"
	_, err := Get(t.Context(), runtimeFake(t), o)
	if err == nil || !strings.Contains(err.Error(), "0000") {
		t.Errorf("err = %v", err)
	}
}

func TestGet_LegacyClient(t *testing.T) {
	f := &fakeQuery{tables: loadFixture(t, "legacy")}
	o := getOpts("WS00000024")
	o.Depth = 1
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.VersionSource != sourceLegacy || res.SourceTables != modelSWD || res.WfdID != "0000002303" ||
		res.Task != "WS00000024" || res.Version != "0000" || len(res.Nodes) == 0 {
		t.Errorf("header = %+v", res.Definition)
	}
	if n := nodeByID(t, res.Nodes, "4"); n.Kind != "loop_while" || n.Label != "Release DO" {
		t.Errorf("node 4 = %+v", n)
	}
	if len(res.Calls) != 1 || res.Calls[0] != (Call{Node: "60", WfdID: "WS00000023"}) {
		t.Errorf("calls = %+v", res.Calls)
	}
	if len(res.Subworkflows) != 1 || !res.Subworkflows[0].Found {
		t.Errorf("subworkflows = %+v", res.Subworkflows)
	}
}

func TestGet_RuntimeVersionReported(t *testing.T) {
	res, err := Get(t.Context(), runtimeFake(t), getOpts(testWfdMain))
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "0001" || res.VersionSource != testSourceHRS || res.SourceTables != modelSWDS || res.RuntimeVersion != "0000" {
		t.Errorf("version=%q source=%q tables=%q runtime=%q", res.Version, res.VersionSource, res.SourceTables, res.RuntimeVersion)
	}
	if res.HasConfiguration {
		t.Error("HasConfiguration set without a C row")
	}
}

func TestGet_HasConfiguration(t *testing.T) {
	f := runtimeFake(t)
	f.tables["SWD_HEADER"] = append(f.tables["SWD_HEADER"],
		Row{"WFD_ID": testWfdMain, "TASK": testWfdMain, "VERSION": "0000", "EXETYP": "C"})
	for _, src := range []string{"definition", sourceRuntime} {
		o := getOpts(testWfdMain)
		o.Source = src
		res, err := Get(t.Context(), f, o)
		if err != nil {
			t.Fatal(err)
		}
		if !res.HasConfiguration {
			t.Errorf("source %s: HasConfiguration = false", src)
		}
	}
}
