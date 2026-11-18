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
	res, err := Get(t.Context(), getFake(t), getOpts("WS48900002"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.Version != "0001" || res.VersionSource != "HRS1205" || res.Title == "" {
		t.Errorf("version=%q source=%q title=%q", res.Version, res.VersionSource, res.Title)
	}
	if res.Task != "WS48900002" || res.WfdID != "WS48900002" || res.SourceTables != "SWDS" || res.Language != "E" {
		t.Errorf("header fields = %+v", res.Definition)
	}
	if res.ActivatedOn != "2002-01-22" {
		t.Errorf("ActivatedOn = %q", res.ActivatedOn)
	}
	if len(res.Calls) != 1 || res.Calls[0] != (Call{Node: "18", WfdID: "WS48900024"}) {
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
	o := getOpts("WS48900002")
	o.Depth = 1
	res, err := Get(t.Context(), getFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 {
		t.Fatalf("Subworkflows = %+v", res.Subworkflows)
	}
	s := res.Subworkflows[0]
	if s.WfdID != "WS48900024" || !s.Found || s.Depth != 1 || strings.Join(s.CalledBy, ",") != "WS48900002" {
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
		if c["WFD_ID"] == "WS48900024" && c["NODEID"] == "0000000004" {
			c["TASK"] = "WS48900002"
		}
		rows[i] = c
	}
	f.tables["SWDSMNODES"] = rows
	return f
}

func TestGet_Cycle(t *testing.T) {
	o := getOpts("WS48900002")
	o.Depth = 5
	res, err := Get(t.Context(), cyclicFake(t), o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(res.Subworkflows) != 1 {
		t.Fatalf("Subworkflows = %d, want 1", len(res.Subworkflows))
	}
	if c := res.Subworkflows[0].Definition.Calls; len(c) != 1 || c[0].WfdID != "WS48900002" {
		t.Errorf("sub calls = %+v, want call back to root", c)
	}
}

func TestGet_MissingSub(t *testing.T) {
	f := getFake(t)
	for _, tbl := range []string{"SWDSHEADER", "HRS1205"} {
		var keep []Row
		for _, r := range f.tables[tbl] {
			if r["WFD_ID"] != "WS48900024" {
				keep = append(keep, r)
			}
		}
		f.tables[tbl] = keep
	}
	o := getOpts("WS48900002")
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
		found = found || strings.Contains(w, "WS48900024")
	}
	if !found {
		t.Errorf("warnings = %v, want one naming WS48900024", res.Warnings)
	}
}

func TestGet_Raw(t *testing.T) {
	o := getOpts("WS48900002")
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
	o := getOpts("WS48900002")
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
	o := getOpts("WS48900002")
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

func TestGet_DepthClamped(t *testing.T) {
	o := getOpts("WS48900002")
	o.Depth = 99
	if _, err := Get(t.Context(), getFake(t), o); err != nil {
		t.Fatalf("Get: %v", err)
	}
}

func TestGet_Stage1Errors(t *testing.T) {
	o := getOpts("WS48900002")
	o.Source = "runtime"
	if _, err := Get(t.Context(), getFake(t), o); err == nil || !strings.Contains(err.Error(), "source=runtime not implemented yet") {
		t.Errorf("runtime err = %v", err)
	}
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
	o := getOpts("WS48900002")
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
		if c["WFD_ID"] == "WS48900002" && c["NODEID"] == "0000000018" {
			c["TASK"] = task
		}
		rows[i] = c
	}
	f.tables["SWDSMNODES"] = rows
	return f
}

func requireUnresolvedSub(t *testing.T, f *fakeQuery, callee string) {
	t.Helper()
	o := getOpts("WS48900002")
	o.Depth = 1
	res, err := Get(t.Context(), f, o)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if res.Task != "WS48900002" || len(res.Nodes) == 0 {
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
	f := withCallee(t, "WS99999999")
	f.tables["SWD_HEADER"] = append(f.tables["SWD_HEADER"],
		Row{"WFD_ID": "WS99999999", "TASK": "WS99999999", "VERSION": "0001", "EXETYP": "S"})
	requireUnresolvedSub(t, f, "WS99999999")
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
	o := getOpts("WS48900002")
	o.Depth = 1
	_, err := Get(t.Context(), failingQuery{getFake(t), "WS48900024"}, o)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want fetch failure", err)
	}
}
