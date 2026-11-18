package bwf

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func constFake(t *testing.T) *fakeQuery {
	return &fakeQuery{tables: loadFixture(t, "constants_class")}
}

func constModel(f *fakeQuery) *rawModel { return &rawModel{CntIdx: f.tables["SWDSCNTIDX"]} }

func hasCall(f *fakeQuery, table string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.ToUpper(c), table) {
			return true
		}
	}
	return false
}

func TestResolveConstants_ViaInterface(t *testing.T) {
	f := constFake(t)
	nodes := []Node{
		{ID: "1", Condition: "&LV_END_DCR& = &CONSTANTS.GC_ABAP_TRUE&"},
		{ID: "2", Condition: "&LV_FIRST_MSG_TYPE& = &CONSTANTS.GC_PROC_VAL_UTILMD&"},
	}
	got, unres, err := resolveConstants(context.Background(), f, constModel(f), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ConditionResolved != "&LV_END_DCR& = 'X'" || got[1].ConditionResolved != "&LV_FIRST_MSG_TYPE& = 'UTILMD'" {
		t.Errorf("resolved: %q / %q", got[0].ConditionResolved, got[1].ConditionResolved)
	}
	if got[0].Condition != nodes[0].Condition {
		t.Errorf("original modified: %q", got[0].Condition)
	}
	if len(unres) != 0 {
		t.Errorf("unresolved = %v", unres)
	}
}

func TestResolveConstants_Unresolved(t *testing.T) {
	f := constFake(t)
	nodes := []Node{{ID: "1", Condition: "&A& = &CONSTANTS.GC_DOES_NOT_EXIST&"}, {ID: "2", Condition: "&CONSTANTS.GC_DOES_NOT_EXIST&"}}
	got, unres, err := resolveConstants(context.Background(), f, constModel(f), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ConditionResolved != nodes[0].Condition {
		t.Errorf("not verbatim: %q", got[0].ConditionResolved)
	}
	if !reflect.DeepEqual(unres, []string{"&CONSTANTS.GC_DOES_NOT_EXIST&"}) {
		t.Errorf("unresolved = %v", unres)
	}
}

func TestResolveConstants_NonClassElementIgnored(t *testing.T) {
	f := constFake(t)
	m := constModel(f)
	m.CntIdx = append(m.CntIdx, Row{"OTYPE": "WS", "ELEMENT": "ABSENCFORM", "TYPEID": "ABSENCFORM", "CATID": "BO"})
	nodes := []Node{{ID: "1", Condition: "&ABSENCFORM.CREATOR& = &X&"}}
	got, unres, err := resolveConstants(context.Background(), f, m, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ConditionResolved != nodes[0].Condition || len(unres) != 0 {
		t.Errorf("got %q unres %v", got[0].ConditionResolved, unres)
	}
	if hasCall(f, "SEOCOMPODF") {
		t.Error("SEOCOMPODF queried for non-class element")
	}
}

func TestResolveConstants_BindingsAndOperations(t *testing.T) {
	f := constFake(t)
	nodes := []Node{{ID: "1", Operation: "lv := &CONSTANTS.GC_ABAP_TRUE&",
		Bindings: []Binding{{Expression: "&CONSTANTS.GC_PROC_VAL_UTILMD&"}}}}
	got, unres, err := resolveConstants(context.Background(), f, constModel(f), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].OperationResolved != "lv := 'X'" || got[0].Bindings[0].ExpressionResolved != "'UTILMD'" {
		t.Errorf("got %q / %q", got[0].OperationResolved, got[0].Bindings[0].ExpressionResolved)
	}
	if got[0].Bindings[0].Expression != "&CONSTANTS.GC_PROC_VAL_UTILMD&" || len(unres) != 0 {
		t.Errorf("expression %q unres %v", got[0].Bindings[0].Expression, unres)
	}
}

func TestResolveConstants_CycleInHierarchy(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"SEOMETAREL": {
			{"CLSNAME": "ZA", "REFCLSNAME": "ZB", "VERSION": "1", "RELTYPE": "1"},
			{"CLSNAME": "ZB", "REFCLSNAME": "ZA", "VERSION": "1", "RELTYPE": "2"},
		},
	}}
	m := &rawModel{CntIdx: []Row{{"OTYPE": "WS", "ELEMENT": "C", "TYPEID": "ZA", "CATID": "CL"}}}
	_, unres, err := resolveConstants(context.Background(), f, m, []Node{{ID: "1", Condition: "&C.X&"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unres, []string{"&C.X&"}) {
		t.Errorf("unresolved = %v", unres)
	}
}

func TestResolveConstants_InvalidNameNotQueried(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{}}
	m := &rawModel{CntIdx: []Row{{"OTYPE": "WS", "ELEMENT": "C", "TYPEID": "bad name", "CATID": "CL"}}}
	_, unres, err := resolveConstants(context.Background(), f, m, []Node{{ID: "1", Condition: "&C.X&"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || !reflect.DeepEqual(unres, []string{"&C.X&"}) {
		t.Errorf("calls %v unres %v", f.calls, unres)
	}
}

func hierarchyFake(rel []Row, comp []Row) (*fakeQuery, *rawModel) {
	f := &fakeQuery{tables: map[string][]Row{"SEOMETAREL": rel, "SEOCOMPODF": comp}}
	m := &rawModel{CntIdx: []Row{{"OTYPE": "WS", "ELEMENT": "C", "TYPEID": "ZCL", "CATID": "CL"}}}
	return f, m
}

func TestResolveConstants_ViaComposedInterface(t *testing.T) {
	f, m := hierarchyFake(
		[]Row{
			{"CLSNAME": "ZCL", "REFCLSNAME": "ZIF_A", "VERSION": "1", "RELTYPE": "1"},
			{"CLSNAME": "ZIF_A", "REFCLSNAME": "ZIF_B", "VERSION": "1", "RELTYPE": "0"},
		},
		[]Row{{"CLSNAME": "ZIF_B", "CMPNAME": "GC_X", "VERSION": "1", "ATTDECLTYP": "2", "ATTVALUE": "'B'"}})
	got, unres, err := resolveConstants(context.Background(), f, m, []Node{{ID: "1", Condition: "&C.GC_X&"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ConditionResolved != "'B'" || len(unres) != 0 {
		t.Errorf("got %q unres %v", got[0].ConditionResolved, unres)
	}
}

func TestResolveConstants_SuperclassLevel2(t *testing.T) {
	f, m := hierarchyFake(
		[]Row{
			{"CLSNAME": "ZCL", "REFCLSNAME": "ZSUPER", "VERSION": "1", "RELTYPE": "2"},
			{"CLSNAME": "ZSUPER", "REFCLSNAME": "ZSUPER2", "VERSION": "1", "RELTYPE": "2"},
		},
		[]Row{{"CLSNAME": "ZSUPER2", "CMPNAME": "GC_X", "VERSION": "1", "ATTDECLTYP": "2", "ATTVALUE": "'S'"}})
	got, unres, err := resolveConstants(context.Background(), f, m, []Node{{ID: "1", Condition: "&C.GC_X&"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ConditionResolved != "'S'" || len(unres) != 0 {
		t.Errorf("got %q unres %v", got[0].ConditionResolved, unres)
	}
}
