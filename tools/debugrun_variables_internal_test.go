package tools

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestChildrenOfFollowsTheLinksOfTheParent(t *testing.T) {
	kids := &adt.DebugChildVariables{
		Variables: []adt.DebugVariable{
			{ID: "LS-B", Name: "B", MetaType: "simple", Value: "2"},
			{ID: "LS-A", Name: "A", MetaType: "simple", Value: "1"},
			{ID: "OTHER", Name: "OTHER"},
		},
		Links: []adt.DebugVariableLink{{ParentID: "LS", ChildID: "LS-A"}, {ParentID: "LS", ChildID: "LS-B"}},
	}
	got := childrenOf(kids, "LS")
	if len(got) != 2 || got[0].ID != "LS-A" || got[1].Value != "2" {
		t.Errorf("got %+v", got)
	}
	if got := childrenOf(&adt.DebugChildVariables{Variables: kids.Variables}, "LS"); len(got) != 3 {
		t.Errorf("without links every variable is a child: %+v", got)
	}
}

func TestParseVariableQueryDefaults(t *testing.T) {
	q, err := parseVariableQuery(map[string]any{"variable_name": " lt_rows ", "offset": float64(5)})
	if err != nil || !q.table || q.offset != 5 || q.limit != defaultTableRows || q.name != "lt_rows" {
		t.Errorf("got %+v, %v", q, err)
	}
}
