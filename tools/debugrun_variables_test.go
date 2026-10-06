package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func variableServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s, _, backend := newDebugServer(t)
	attachRun(t, s, backend)
	backend.set(func(f *fakeDebugBackend) {
		f.vars = map[string]fakeVar{
			"LV_X":    {id: "LV_X", name: "LV_X", meta: "simple", value: "42"},
			"LS_ROW":  {id: "LS_ROW", name: "LS_ROW", meta: "structure", value: "Structure"},
			"LT_ROWS": {id: "LT_ROWS", name: "LT_ROWS", meta: "table", value: "[250x1]", lines: 250},
			"LR_DATA": {id: "LR_DATA", name: "LR_DATA", meta: "dataref", value: "->"},
			"LO_ITEM": {id: "LO_ITEM", name: "LO_ITEM", meta: "objectref", value: "{O:1}"},
		}
		f.children = map[string][]fakeVar{
			"LS_ROW":     {{id: "LS_ROW-TEXT", name: "TEXT", meta: "simple", value: "seven"}},
			"LR_DATA->*": {{id: "LR_DATA->TEXT", name: "TEXT", meta: "simple", value: "deref"}},
			"LO_ITEM":    {{id: "LO_ITEM-MV_NAME", name: "MV_NAME", meta: "simple", value: "item"}},
		}
	})
	return s
}

func variable(t *testing.T, res *mcp.CallToolResult) tools.DebugVariableResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("debug_get_variable failed: %s", debugResultText(res))
	}
	var v tools.DebugVariableResult
	if err := json.Unmarshal([]byte(debugResultText(res)), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDebugGetVariable_ScalarAsToday(t *testing.T) {
	s := variableServer(t)
	if v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": "LV_X"})); v.Value != "42" || v.Children != nil || v.Table != nil {
		t.Errorf("got %+v", v)
	}
}

func TestDebugGetVariable_Expand(t *testing.T) {
	s := variableServer(t)
	cases := map[string]string{"LS_ROW": "seven", "LR_DATA": "deref", "LO_ITEM": "item"}
	for name, want := range cases {
		v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": name, "expand": true}))
		if len(v.Children) != 1 || v.Children[0].Value != want {
			t.Errorf("%s: children %+v", name, v.Children)
		}
	}
}

func TestDebugGetVariable_TablePage(t *testing.T) {
	s := variableServer(t)
	v := variable(t, callTool(t, s, "debug_get_variable", map[string]interface{}{"variable_name": "LT_ROWS", "offset": 2, "limit": 3}))
	if v.Table == nil || v.Table.TotalLines != 250 || len(v.Table.Rows) != 3 || v.Table.Rows[0].Index != 2 ||
		v.Table.Rows[0].Fields[0] != (tools.DebugTableField{Path: "TEXT", Value: "row 2"}) {
		t.Errorf("got %+v", v.Table)
	}
}

func TestDebugGetVariable_Refusals(t *testing.T) {
	s := variableServer(t)
	cases := []struct {
		args map[string]interface{}
		want string
	}{
		{map[string]interface{}{"variable_name": "LT_ROWS", "limit": 101}, "100"},
		{map[string]interface{}{"variable_name": "LT_ROWS", "offset": 0}, "1-based"},
		{map[string]interface{}{"variable_name": "LV_X", "offset": 1}, "not an internal table"},
		{map[string]interface{}{"variable_name": "LT_ROWS", "expand": true}, "offset/limit"},
		{map[string]interface{}{"variable_name": "LS_ROW", "expand": true, "limit": 5}, "either expand or offset/limit"},
	}
	for _, c := range cases {
		res := callTool(t, s, "debug_get_variable", c.args)
		if !res.IsError || !strings.Contains(debugResultText(res), c.want) {
			t.Errorf("%v: got %s", c.args, debugResultText(res))
		}
	}
}

func TestDebugGetVariableDescriptionStatesTheScope(t *testing.T) {
	for _, tl := range listRegisteredTools(t, newTestServer(&mockClient{})) {
		if tl.Name == "debug_get_variable" {
			if !strings.Contains(tl.Description, "not for retrieving data") || !strings.Contains(tl.Description, "100") {
				t.Errorf("description: %s", tl.Description)
			}
			return
		}
	}
	t.Fatal("debug_get_variable not registered")
}
