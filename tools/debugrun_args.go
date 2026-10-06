package tools

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// paramUser is the optional `user` argument of every debug tool.
const paramUser = "user"

// adtPrograms is the first path segment of program and include URIs.
const adtPrograms = "programs"

const (
	maxRunBreakpoints        = 30
	defaultRunTimeoutSeconds = 300
	// maxRunTimeoutSeconds is also the per-poll cap of one listening window
	// (600 s, read in the SAP_BASIS 816 listener source).
	maxRunTimeoutSeconds = 600
)

// debugRunArgs are the validated arguments of debug_run.
type debugRunArgs struct {
	breakpoints   []adt.LineBreakpoint
	user          string // as given, may be empty
	kind          string
	unitObjectURI string
	target        *DebugTarget
	timeout       time.Duration
}

func parseDebugRunArgs(args map[string]any) (debugRunArgs, error) {
	var a debugRunArgs
	bps, err := parseRunBreakpoints(args["breakpoints"])
	if err != nil {
		return a, err
	}
	a.breakpoints = bps
	if u, ok := args[paramUser].(string); ok {
		a.user = strings.TrimSpace(u)
	}
	if err := parseRunTrigger(args["trigger"], &a); err != nil {
		return a, err
	}
	secs := defaultRunTimeoutSeconds
	if v, ok := args["timeout_seconds"]; ok && v != nil {
		n, ok := intArg(v)
		if !ok || n < 1 || n > maxRunTimeoutSeconds {
			return a, fmt.Errorf("debug_run: \"timeout_seconds\" must be a whole number from 1 to %d", maxRunTimeoutSeconds)
		}
		secs = n
	}
	a.timeout = time.Duration(secs) * time.Second
	return a, nil
}

func parseRunBreakpoints(v any) ([]adt.LineBreakpoint, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 || len(list) > maxRunBreakpoints {
		return nil, fmt.Errorf("debug_run: \"breakpoints\" needs 1 to %d entries of {object_uri, line}", maxRunBreakpoints)
	}
	out := make([]adt.LineBreakpoint, 0, len(list))
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("debug_run: breakpoint %d is not an object {object_uri, line}", i)
		}
		uri, _ := m[paramObjectURI].(string)
		uri = strings.TrimSpace(uri)
		if _, ok := sourceObjectURI(uri); !ok {
			return nil, fmt.Errorf("debug_run: breakpoint %d: object_uri %q is not a source URI (…/source/main or …/includes/…)", i, uri)
		}
		line, ok := intArg(m["line"])
		if !ok || line < 1 {
			return nil, fmt.Errorf("debug_run: breakpoint %d: line must be a whole number of at least 1", i)
		}
		typ, name := deriveBreakpointObject(uri)
		out = append(out, adt.LineBreakpoint{ObjectURI: uri, Line: line, ObjectType: typ, ObjectName: name})
	}
	return out, nil
}

func parseRunTrigger(v any, a *debugRunArgs) error {
	t, ok := v.(map[string]any)
	if !ok {
		return errors.New("debug_run: \"trigger\" is required: {kind: unit_tests | manual | gui, ...}")
	}
	kind, _ := t["kind"].(string)
	a.kind = strings.TrimSpace(kind)
	rawTarget := t["target"]
	switch a.kind {
	case triggerUnitTests:
		uri, _ := t[paramObjectURI].(string)
		a.unitObjectURI = strings.TrimSpace(uri)
		if a.unitObjectURI == "" {
			a.unitObjectURI, _ = sourceObjectURI(a.breakpoints[0].ObjectURI)
		}
	case triggerManual:
	case triggerGUI:
		target, err := parseDebugTarget(rawTarget)
		if err != nil {
			return err
		}
		a.target = target
		return nil
	default:
		return fmt.Errorf("debug_run: trigger.kind must be one of unit_tests, manual, gui (got %q)", kind)
	}
	if rawTarget != nil {
		return errors.New(`debug_run: trigger.target is used only with kind "gui"`)
	}
	return nil
}

func parseDebugTarget(v any) (*DebugTarget, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New(`debug_run: trigger kind "gui" needs trigger.target {type, name, inputs?}`)
	}
	typ, _ := m["type"].(string)
	name, _ := m["name"].(string)
	t := &DebugTarget{Type: strings.TrimSpace(typ), Name: strings.TrimSpace(name)}
	if !validDebugTargetType(t.Type) {
		return nil, fmt.Errorf("debug_run: trigger.target.type must be one of %s (got %q)", strings.Join(validDebugTargetTypes, ", "), typ)
	}
	if t.Name == "" {
		return nil, errors.New("debug_run: trigger.target.name is required")
	}
	if t.Type == targetClassMethod && !strings.Contains(t.Name, "=>") {
		return nil, errors.New("debug_run: a class_method target is named CLASS=>METHOD")
	}
	if raw, ok := m["inputs"]; ok && raw != nil {
		in, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("debug_run: trigger.target.inputs must be an object of strings")
		}
		t.Inputs = make(map[string]string, len(in))
		for k, v := range in {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("debug_run: trigger.target.inputs[%q] must be a string", k)
			}
			t.Inputs[k] = s
		}
	}
	return t, nil
}

// intArg reads a whole JSON number. JSON numbers arrive as float64; a fraction
// or another type is rejected rather than truncated.
func intArg(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || math.Abs(n) > 1<<53 {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

// sourceObjectURI returns the object URI of a source URI: the part before
// /source/ or /includes/, whichever comes first.
func sourceObjectURI(uri string) (string, bool) {
	// Skip the "/sap/bc/adt/<category>/<kind>" prefix: "includes" is itself a
	// kind there (/programs/includes/<name>).
	start := 0
	if rest, ok := strings.CutPrefix(uri, "/sap/bc/adt/"); ok {
		for i, skipped := 0, 0; i < len(rest) && skipped < 2; i++ {
			if rest[i] == '/' {
				skipped++
				start = len(uri) - len(rest) + i
			}
		}
	}
	cut := -1
	for _, sep := range []string{"/source/", "/includes/"} {
		if i := strings.Index(uri[start:], sep); i >= 0 && (cut < 0 || start+i < cut) {
			cut = start + i
		}
	}
	if cut <= 0 {
		return "", false
	}
	return uri[:cut], true
}

// deriveBreakpointObject derives adtcore:type and adtcore:name from a source
// URI (spec: derived until adtler#200 confirms they may be omitted). An
// unknown shape returns "", "" and the breakpoint is sent without them.
func deriveBreakpointObject(sourceURI string) (objectType, objectName string) {
	obj, ok := sourceObjectURI(sourceURI)
	if !ok {
		return "", ""
	}
	segs := strings.Split(strings.TrimPrefix(obj, "/sap/bc/adt/"), "/")
	name := func(s string) string {
		if u, err := url.PathUnescape(s); err == nil {
			s = u
		}
		return strings.ToUpper(s)
	}
	switch {
	case len(segs) == 3 && segs[0] == adtPrograms && segs[1] == "programs":
		return "PROG/P", name(segs[2])
	case len(segs) == 3 && segs[0] == adtPrograms && segs[1] == "includes":
		return "PROG/I", name(segs[2])
	case len(segs) == 3 && segs[0] == "oo" && segs[1] == "classes":
		return "CLAS/OC", name(segs[2])
	case len(segs) == 3 && segs[0] == "oo" && segs[1] == "interfaces":
		return "INTF/OI", name(segs[2])
	case len(segs) == 5 && segs[0] == "functions" && segs[1] == "groups" && segs[3] == "fmodules":
		return "FUGR/FF", name(segs[4])
	}
	return "", ""
}

// checkBreakpointResults pairs SAP's results (adtler returns them in request
// order) with the requested breakpoints. A breakpoint counts as set when SAP
// set it or answered errorKind "existing". It returns the set ones and, when
// any was rejected, an error naming each rejected breakpoint.
func checkBreakpointResults(req []adt.LineBreakpoint, res []adt.BreakpointResult) ([]DebugRunBreakpoint, error) {
	var set []DebugRunBreakpoint
	var rejected []string
	for i, bp := range req {
		var r adt.BreakpointResult
		if i < len(res) {
			r = res[i]
		} else {
			r.ErrorMessage = "SAP returned no result for this breakpoint"
		}
		if r.IsSet() || r.ErrorKind == "existing" {
			set = append(set, DebugRunBreakpoint{ObjectURI: bp.ObjectURI, Line: bp.Line, ID: r.ID, Scope: string(adt.BreakpointScopeExternal)})
			continue
		}
		rejected = append(rejected, fmt.Sprintf("%s line %d: %s", bp.ObjectURI, bp.Line, describeRejection(r)))
	}
	if len(rejected) > 0 {
		return set, fmt.Errorf("SAP rejected %d breakpoint(s): %s", len(rejected), strings.Join(rejected, "; "))
	}
	return set, nil
}

func describeRejection(r adt.BreakpointResult) string {
	switch {
	case r.ErrorKind != "" && r.ErrorMessage != "":
		return r.ErrorKind + ": " + r.ErrorMessage
	case r.ErrorKind != "":
		return r.ErrorKind
	case r.ErrorMessage != "":
		return r.ErrorMessage
	}
	return "not set"
}
