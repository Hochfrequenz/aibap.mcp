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
