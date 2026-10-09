package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

const argsProgURI = "/sap/bc/adt/programs/programs/zprog/source/main"

func bpArg(uri string, line any) map[string]any {
	return map[string]any{"object_uri": uri, "line": line}
}

func TestParseDebugRunArgs(t *testing.T) {
	manual := map[string]any{"kind": "manual"}
	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
		check   func(t *testing.T, a debugRunArgs)
	}{
		{
			name: "manual with defaults",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "user": " alice "},
			check: func(t *testing.T, a debugRunArgs) {
				if a.kind != triggerManual || a.user != "alice" || a.timeout != 300*time.Second {
					t.Errorf("got %+v", a)
				}
				want := adt.LineBreakpoint{ObjectURI: argsProgURI, Line: 3, ObjectType: "PROG/P", ObjectName: "ZPROG"}
				if len(a.breakpoints) != 1 || a.breakpoints[0] != want {
					t.Errorf("breakpoints = %+v, want %+v", a.breakpoints, want)
				}
			},
		},
		{
			name: "unit tests default to the object of the first breakpoint",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "unit_tests"}},
			check: func(t *testing.T, a debugRunArgs) {
				if a.unitObjectURI != "/sap/bc/adt/programs/programs/zprog" {
					t.Errorf("unitObjectURI = %q", a.unitObjectURI)
				}
			},
		},
		{
			name: "gui target with inputs",
			args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "timeout_seconds": float64(600),
				"trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "report", "name": "zprog", "inputs": map[string]any{"P_X": "1"}}}},
			check: func(t *testing.T, a debugRunArgs) {
				if a.target == nil || a.target.Type != "report" || a.target.Inputs["P_X"] != "1" || a.timeout != 600*time.Second {
					t.Errorf("got %+v / %+v", a, a.target)
				}
			},
		},
		{name: "no breakpoints", args: map[string]any{"breakpoints": []any{}, "trigger": manual}, wantErr: `"breakpoints" needs 1 to 30`},
		{name: "31 breakpoints", args: map[string]any{"breakpoints": thirtyOne(), "trigger": manual}, wantErr: `"breakpoints" needs 1 to 30`},
		{name: "not a source URI", args: map[string]any{"breakpoints": []any{bpArg("/sap/bc/adt/programs/programs/zprog", float64(3))}, "trigger": manual}, wantErr: "not a source URI"},
		{name: "line zero", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(0))}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "fractional line", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, 2.5)}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "line as string", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, "3")}, "trigger": manual}, wantErr: "line must be a whole number"},
		{name: "missing trigger", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}}, wantErr: `"trigger" is required`},
		{name: "unknown kind", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "rfc"}}, wantErr: "trigger.kind must be one of"},
		{name: "gui without target", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui"}}, wantErr: "needs trigger.target"},
		{name: "gui bad target type", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "bsp", "name": "X"}}}, wantErr: "trigger.target.type must be one of"},
		{name: "class method without arrow", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "class_method", "name": "ZCL_X"}}}, wantErr: "CLASS=>METHOD"},
		{name: "non-string input", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "gui", "target": map[string]any{"type": "report", "name": "ZPROG", "inputs": map[string]any{"P_X": float64(1)}}}}, wantErr: `inputs["P_X"] must be a string`},
		{name: "target with manual", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": map[string]any{"kind": "manual", "target": map[string]any{"type": "report", "name": "ZPROG"}}}, wantErr: `used only with kind "gui"`},
		{name: "timeout too large", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "timeout_seconds": float64(601)}, wantErr: `"timeout_seconds" must be a whole number from 1 to 600`},
		{name: "timeout as string", args: map[string]any{"breakpoints": []any{bpArg(argsProgURI, float64(3))}, "trigger": manual, "timeout_seconds": "300"}, wantErr: `"timeout_seconds" must be a whole number`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := parseDebugRunArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, a)
		})
	}
}

func thirtyOne() []any {
	out := make([]any, 31)
	for i := range out {
		out[i] = bpArg(argsProgURI, float64(i+1))
	}
	return out
}

func TestDeriveBreakpointObject(t *testing.T) {
	cases := []struct{ uri, typ, name string }{
		{"/sap/bc/adt/programs/programs/zprog/source/main", "PROG/P", "ZPROG"},
		{"/sap/bc/adt/programs/programs/ZPROG/source/main", "PROG/P", "ZPROG"},
		{"/sap/bc/adt/programs/includes/zprog_f01/source/main", "PROG/I", "ZPROG_F01"},
		{"/sap/bc/adt/oo/classes/zcl_x/source/main", "CLAS/OC", "ZCL_X"},
		{"/sap/bc/adt/oo/classes/zcl_x/includes/testclasses", "CLAS/OC", "ZCL_X"},
		{"/sap/bc/adt/oo/classes/%2fabc%2fcl_x/source/main", "CLAS/OC", "/ABC/CL_X"},
		{"/sap/bc/adt/oo/interfaces/zif_x/source/main", "INTF/OI", "ZIF_X"},
		{"/sap/bc/adt/functions/groups/zfg/fmodules/z_fm/source/main", "FUGR/FF", "Z_FM"},
		{"/sap/bc/adt/functions/groups/zfg/includes/lzfgf01/source/main", "", ""},
		{"/sap/bc/adt/ddic/ddl/sources/zddl/source/main", "", ""},
	}
	for _, c := range cases {
		typ, name := deriveBreakpointObject(c.uri)
		if typ != c.typ || name != c.name {
			t.Errorf("deriveBreakpointObject(%q) = %q, %q; want %q, %q", c.uri, typ, name, c.typ, c.name)
		}
	}
}

func TestCheckBreakpointResults(t *testing.T) {
	req := []adt.LineBreakpoint{{ObjectURI: argsProgURI, Line: 3}, {ObjectURI: argsProgURI, Line: 9}, {ObjectURI: argsProgURI, Line: 12}}
	res := []adt.BreakpointResult{
		{ID: "BP1"},
		{ID: "BP2", ErrorKind: "existing"},
		{ErrorKind: "invalidPosition", ErrorMessage: "no statement"},
	}
	set, err := checkBreakpointResults(req, res)
	if len(set) != 2 || set[0].ID != "BP1" || set[1].ID != "BP2" || set[1].Scope != "external" {
		t.Errorf("set = %+v (existing counts as set)", set)
	}
	if err == nil || !strings.Contains(err.Error(), "line 12") || !strings.Contains(err.Error(), "invalidPosition") {
		t.Errorf("error must name the rejected breakpoint, got %v", err)
	}
	if _, err := checkBreakpointResults(req[:1], res[:1]); err != nil {
		t.Errorf("all set: %v", err)
	}
}
