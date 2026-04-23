package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestStructuredContentIsObject pins the MCP 2025-06-18 contract that
// CallToolResult.structuredContent MUST be a JSON object — a slice, null,
// or scalar trips Zod record() validation on Claude's client side (see
// issue #351 for the original report, #353 for why this is reflective).
//
// The test walks every tool the server exposes via tools/list, synthesises
// minimum-required args from the input schema, invokes the handler via
// JSON-RPC, and asserts the resulting StructuredContent serialises to a
// JSON object. The assertion runs on both success and error paths — both
// errorResult (typed ToolError) and every success wrapper are expected to
// be objects, so the invariant is identical either way. A bug like "return
// mcp.NewToolResultJSON(slice)" in a success branch fails the check because
// the slice serialises to [...] (or null for a nil slice), not {...}.
//
// A completeness guard asserts every registered tool is either exercised
// by this walk or listed in knownOptOuts — so a new slice-returning tool
// added later can't silently slip past without a human consciously
// opting out.

// knownOptOuts lists tools that cannot be exercised by a blind reflective
// call even for the purpose of shape-checking. Each entry needs a reason
// so a future contributor can tell whether the exemption still applies.
//
// errorResult guarantees the error path also produces an object, so a
// handler returning an adt error on canned args is fine — the invariant
// is still checked on the error branch. Opt-outs are reserved for
// failure modes no amount of arg synthesis can dodge (panics, infinite
// hangs, real side effects).
var knownOptOuts = map[string]string{
	// adtler's NewDebugSession requires a concrete *httpClient or
	// *ClientRegistry and panics when given the test mockClient — the
	// debug_* handlers construct a session on first use and crash the
	// test goroutine before StructuredContent is ever produced. The
	// debugger handlers' result shapes are still guarded by the
	// convention-wide rule in CLAUDE.md ("structuredContent must be a
	// JSON object"); reviewers check by eye.
	"debug_set_breakpoint": "adtler debug session panics on mockClient",
	"debug_start":          "adtler debug session panics on mockClient",
	"debug_stop":           "adtler debug session panics on mockClient",
	"debug_get_sessions":   "adtler debug session panics on mockClient",
	"debug_attach":         "adtler debug session panics on mockClient",
	"debug_step":           "adtler debug session panics on mockClient",
	"debug_get_variable":   "adtler debug session panics on mockClient",
	"debug_get_stack":      "adtler debug session panics on mockClient",
	"debug_set_watchpoint": "adtler debug session panics on mockClient",
}

// TestStructuredContentIsObject is the reflective guardrail — one test,
// every tool, both paths.
func TestStructuredContentIsObject(t *testing.T) {
	s := newTestServer(&mockClient{})

	listResp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	listBytes, err := json.Marshal(listResp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}

	var listEnvelope struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listBytes, &listEnvelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v\nraw: %s", err, string(listBytes))
	}
	if len(listEnvelope.Result.Tools) == 0 {
		t.Fatal("tools/list returned zero tools — test server misconfigured")
	}

	enumerated := make(map[string]bool, len(listEnvelope.Result.Tools))
	for _, tool := range listEnvelope.Result.Tools {
		enumerated[tool.Name] = true
	}

	// Completeness: opt-outs must all refer to tools that actually exist,
	// otherwise the list grows stale as tools are renamed or removed.
	for name := range knownOptOuts {
		if !enumerated[name] {
			t.Errorf("knownOptOuts[%q]: tool no longer registered — remove the opt-out", name)
		}
	}

	for _, tool := range listEnvelope.Result.Tools {
		tool := tool
		if reason, ok := knownOptOuts[tool.Name]; ok {
			t.Run(tool.Name+"_opted_out", func(t *testing.T) {
				t.Skipf("opted out: %s", reason)
			})
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			args := synthesizeArgs(tool.InputSchema)
			rawSC, present := wireStructuredContent(t, s, tool.Name, args)
			assertWireStructuredContentIsObject(t, tool.Name, rawSC, present)
		})
	}
}

// wireStructuredContent invokes the tool via JSON-RPC and extracts the
// raw bytes of result.structuredContent from the response envelope.
// Returns (bytes, true) when the field is present on the wire and
// (nil, false) when it's absent. Notably distinguishes "null" (present,
// violating) from truly absent (omitempty skipped it).
func wireStructuredContent(t *testing.T, s *server.MCPServer, toolName string, args map[string]any) (json.RawMessage, bool) {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("%s: marshal args: %v", toolName, err)
	}
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		toolName, string(argsJSON),
	)
	resp := s.HandleMessage(context.Background(), []byte(msg))
	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("%s: marshal response: %v", toolName, err)
	}
	var envelope struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("%s: unmarshal envelope: %v\nraw: %s", toolName, err, string(respBytes))
	}
	if envelope.Error != nil {
		t.Fatalf("%s: JSON-RPC error code=%d msg=%s", toolName, envelope.Error.Code, envelope.Error.Message)
	}
	raw, ok := envelope.Result["structuredContent"]
	return raw, ok
}

// assertWireStructuredContentIsObject checks the wire form. Absence is
// fine (mcp-go's omitempty skipped the field); presence must be a JSON
// object. This is strictly stronger than inspecting the unmarshalled
// *CallToolResult, because mcp-go's UnmarshalJSON collapses
// `"structuredContent": null` and a missing field into the same
// Go-level nil — but null on the wire is a spec violation (record()
// rejects it), while omitempty-absence is legal.
func assertWireStructuredContentIsObject(t shapeCheckerT, toolName string, raw json.RawMessage, present bool) {
	t.Helper()
	if !present {
		return
	}
	if len(raw) == 0 || raw[0] != '{' {
		t.Fatalf(
			"%s: structuredContent on the wire must be a JSON object per MCP 2025-06-18; "+
				"got %s.\n"+
				"This is the bug class from #351: arrays/nulls/scalars in structuredContent "+
				"trip Zod record() validation on the client side.",
			toolName, string(raw),
		)
	}
}

// shapeCheckerT is the narrow slice of *testing.T that
// assertWireStructuredContentIsObject needs.
// TestWireStructuredContentChecker substitutes a capturingT implementing
// this interface so the checker's pass/fail decisions can be asserted on
// without failing the outer test.
type shapeCheckerT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// synthesizeArgs builds a minimum-required argument map from a tool's
// input schema. Produces type-appropriate defaults ("x" for string, 1 for
// number, ["x"] for required arrays, etc.). If the handler rejects the
// canned args and emits an errorResult, that's fine — errorResult is still
// object-shaped, so the invariant is exercised either way. The aim is not
// to produce a semantically valid request but to drive every handler to
// emit *some* CallToolResult whose StructuredContent shape can be checked.
func synthesizeArgs(schema map[string]any) map[string]any {
	args := map[string]any{}
	required, _ := schema["required"].([]any)
	props, _ := schema["properties"].(map[string]any)
	// Sort required names for deterministic iteration (keeps test output
	// predictable across Go map-iteration-order flakiness).
	names := make([]string, 0, len(required))
	for _, r := range required {
		if name, ok := r.(string); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		propSchema, _ := props[name].(map[string]any)
		args[name] = defaultValueFromSchema(propSchema)
	}
	return args
}

func defaultValueFromSchema(schema map[string]any) any {
	if schema == nil {
		return "x"
	}
	// oneOf: pick the first branch's default.
	if oneOf, ok := schema["oneOf"].([]any); ok && len(oneOf) > 0 {
		if first, ok := oneOf[0].(map[string]any); ok {
			return defaultValueFromSchema(first)
		}
	}
	switch t, _ := schema["type"].(string); t {
	case "string":
		return "x"
	case "number", "integer":
		return float64(1)
	case "boolean":
		return false
	case "array":
		items, _ := schema["items"].(map[string]any)
		return []any{defaultValueFromSchema(items)}
	case "object":
		return map[string]any{}
	default:
		return "x"
	}
}

// TestWireStructuredContentChecker pins the wire-level checker's own
// behaviour so a future refactor cannot silently weaken it. Safety net for
// the "test the test" axis: if someone changes
// assertWireStructuredContentIsObject to e.g. accept arrays, or treat null
// as absent, these subtests fail immediately.
func TestWireStructuredContentChecker(t *testing.T) {
	cases := []struct {
		name       string
		raw        json.RawMessage
		present    bool
		shouldFail bool
	}{
		{"object_passes", json.RawMessage(`{"k":"v"}`), true, false},
		{"absent_passes", nil, false, false},
		{"null_on_wire_fails", json.RawMessage(`null`), true, true},
		{"array_fails", json.RawMessage(`[1,2]`), true, true},
		{"empty_array_fails", json.RawMessage(`[]`), true, true},
		{"string_scalar_fails", json.RawMessage(`"hi"`), true, true},
		{"number_scalar_fails", json.RawMessage(`42`), true, true},
		{"true_scalar_fails", json.RawMessage(`true`), true, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeT := &capturingT{}
			assertWireStructuredContentIsObject(fakeT, "_", tc.raw, tc.present)
			if fakeT.failed != tc.shouldFail {
				t.Fatalf("shouldFail=%v but checker failed=%v (msg: %s)", tc.shouldFail, fakeT.failed, fakeT.msg)
			}
		})
	}
}

// capturingT implements shapeCheckerT and records whether Fatalf was
// called. Used by TestWireStructuredContentChecker to assert the
// checker's pass/fail decisions without failing the outer test.
type capturingT struct {
	failed bool
	msg    string
}

func (c *capturingT) Helper() {}
func (c *capturingT) Fatalf(format string, args ...any) {
	c.failed = true
	c.msg = format
	_ = args
}
