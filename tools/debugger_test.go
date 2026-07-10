package tools

import "testing"

// TestBuildDebugSessionsResult guards the regression from issue #433: an empty
// SAP response body (no active debuggee sessions) must not crash. Previously the
// handler forwarded the raw bytes as json.RawMessage, and NewToolResultJSON's
// json.Marshal rejected the empty (and, since the body is XML, any) payload with
// "unexpected end of JSON input", surfacing as an MCP -32603 critical failure.
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
