package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestBuildDebugSessionsResult guards the regression from issue #433: an empty
// SAP response body (no active debuggee sessions) must not crash. Previously the
// handler forwarded the raw bytes as json.RawMessage, and NewToolResultJSON's
// json.Marshal rejected the non-JSON payload (empty body: "unexpected end of JSON input"),
// surfacing as an MCP -32603 critical failure.
func TestBuildDebugSessionsResult(t *testing.T) {
	// Empty body (no active sessions) must not crash and must report no sessions.
	if got := buildDebugSessionsResult([]byte("")); got.HasSessions || got.Raw != "" {
		t.Errorf("empty: got %+v, want {HasSessions:false Raw:\"\"}", got)
	}

	// Whitespace-only body is also "empty".
	if got := buildDebugSessionsResult([]byte("  \r\n ")); got.HasSessions || got.Raw != "" {
		t.Errorf("whitespace: got %+v, want {HasSessions:false Raw:\"\"}", got)
	}

	// Non-empty (XML) body is forwarded verbatim as Raw.
	xml := `<sessions><session id="123"/></sessions>`
	got := buildDebugSessionsResult([]byte(xml))
	if !got.HasSessions || got.Raw != xml {
		t.Errorf("xml: got %+v, want HasSessions=true and Raw=%q", got, xml)
	}
}

// TestDebugSessionsResultMarshalsToObject closes the loop the reflective
// structured_content_shape_test guardrail can't reach (debug_get_sessions is in
// knownOptOuts): the value the handler feeds to NewToolResultJSON must round-trip
// to a JSON object, per the MCP 2025-06-18 structuredContent requirement. Covers
// both the empty and non-empty branches.
func TestDebugSessionsResultMarshalsToObject(t *testing.T) {
	for _, body := range [][]byte{[]byte(""), []byte(`<sessions><session id="123"/></sessions>`)} {
		res, err := mcp.NewToolResultJSON(buildDebugSessionsResult(body))
		if err != nil {
			t.Fatalf("body %q: NewToolResultJSON returned error: %v", body, err)
		}
		out, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("body %q: marshal StructuredContent: %v", body, err)
		}
		if !strings.HasPrefix(string(out), "{") {
			t.Errorf("body %q: StructuredContent is not a JSON object: %s", body, out)
		}
	}
}

// TestDebugStepGetVariableGetStackSetWatchpointBuildersHandleNonJSONBody
// exercises the builder functions issue #501 introduced (debug_step,
// debug_get_variable, debug_get_stack, and debug_set_watchpoint used to
// forward the ADT debugger endpoints' XML/text bodies as json.RawMessage to
// NewToolResultJSON, which fails JSON validation for any body that isn't
// valid JSON — none of these ever are — the same bug class as #433). The
// builders wrap the raw bytes in a typed struct instead, so json.Marshal
// always succeeds.
//
// This test calls the builders directly, not the registered debug_* tool
// handlers — adtler's DebugSession panics against the mockClient used
// elsewhere in this package's tests (see TestStructuredContentIsObject's
// knownOptOuts comment), so there is no unit-test-level way to pin that a
// handler actually calls the right builder with the right bytes. That
// wiring is proven live instead — see issue #501's linked 2026-09-18
// end-to-end verification. TestDebugStepActionEnum and
// TestDebugToolsDeclareOutputSchema (debugger_schema_test.go) cover what
// unit tests safely can: the declared tool schema, without invoking a
// handler.
func TestDebugStepGetVariableGetStackSetWatchpointBuildersHandleNonJSONBody(t *testing.T) {
	stepXML := `<PROGATTR><DEBUGGEE_STATE/></PROGATTR>`
	if got := buildDebugStepResult([]byte(stepXML)); got.Raw != stepXML {
		t.Errorf("buildDebugStepResult: got %+v, want Raw=%q", got, stepXML)
	}

	if got := buildDebugVariableResult("LV_FLAG", []byte("X")); got.VariableName != "LV_FLAG" || got.Value != "X" {
		t.Errorf("buildDebugVariableResult: got %+v, want {VariableName:LV_FLAG Value:X}", got)
	}

	stackXML := `<dbg:stack xmlns:dbg="..."><stackEntry/></dbg:stack>`
	if got := buildDebugStackResult([]byte(stackXML)); got.Raw != stackXML {
		t.Errorf("buildDebugStackResult: got %+v, want Raw=%q", got, stackXML)
	}

	watchpointXML := `<watchpoint id="1"/>`
	if got := buildDebugWatchpointResult([]byte(watchpointXML)); got.Raw != watchpointXML {
		t.Errorf("buildDebugWatchpointResult: got %+v, want Raw=%q", got, watchpointXML)
	}
}

// TestDebugStepGetVariableGetStackSetWatchpointResultsMarshalToObject checks
// that each of the four #501 result types round-trips to a JSON object
// through NewToolResultJSON, per the MCP 2025-06-18 structuredContent
// requirement — a raw XML/text body forwarded via json.RawMessage (the
// #501 bug) would fail this. Like the test above, this exercises the
// result types directly, not the registered handlers (see that test's
// comment for why); a named Go struct always marshals to a JSON object,
// so this specifically guards against a future field type change breaking
// that shape (e.g. an added field whose type can't marshal), not against
// the handler-wiring class of regression #501 was.
func TestDebugStepGetVariableGetStackSetWatchpointResultsMarshalToObject(t *testing.T) {
	results := []any{
		buildDebugStepResult([]byte(`<x/>`)),
		buildDebugVariableResult("LV_FLAG", []byte("X")),
		buildDebugStackResult([]byte(`<dbg:stack/>`)),
		buildDebugWatchpointResult([]byte(`<watchpoint/>`)),
	}
	for _, r := range results {
		res, err := mcp.NewToolResultJSON(r)
		if err != nil {
			t.Fatalf("%T: NewToolResultJSON returned error: %v", r, err)
		}
		out, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("%T: marshal StructuredContent: %v", r, err)
		}
		if !strings.HasPrefix(string(out), "{") {
			t.Errorf("%T: StructuredContent is not a JSON object: %s", r, out)
		}
	}
}
