package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

func textOfTE(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

const testProgURI = "/sap/bc/adt/programs/programs/zfoo"
const testTextElementsURI = "/sap/bc/adt/textelements/programs/zfoo"

func TestSetTextElementsTool_SymbolsHappy(t *testing.T) {
	var lockedURI, writtenURI, gotLock, gotTransport string
	var gotSymbols []adt.TextSymbol
	var gotSelections []adt.SelectionText

	mock := &mockClient{
		lockObjectFn: func(_ context.Context, uri string) (string, error) {
			lockedURI = uri
			return "lock-handle-X", nil
		},
		setTextElementsFn: func(_ context.Context, uri string, sym []adt.TextSymbol, sel []adt.SelectionText, lock, tr string) error {
			writtenURI, gotLock, gotTransport = uri, lock, tr
			gotSymbols, gotSelections = sym, sel
			return nil
		},
	}
	s := newTestServer(mock)

	res := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": testProgURI,
		"symbols":    `[{"key":"001","text":"hello","max_length":5}]`,
		"transport":  "DEVK900001",
	})
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", textOfTE(res))
	}

	if lockedURI != testTextElementsURI {
		t.Errorf("lock URI: got %q, want %q (auto-lock must target the textelements resource, not the program)", lockedURI, testTextElementsURI)
	}
	if writtenURI != testProgURI {
		t.Errorf("write URI: got %q, want %q", writtenURI, testProgURI)
	}
	if gotLock != "lock-handle-X" {
		t.Errorf("lock handle passed to SetTextElements: got %q", gotLock)
	}
	if gotTransport != "DEVK900001" {
		t.Errorf("transport: got %q", gotTransport)
	}
	if len(gotSymbols) != 1 || gotSymbols[0].Key != "001" || gotSymbols[0].Text != "hello" || gotSymbols[0].MaxLength != 5 {
		t.Errorf("symbols: got %+v", gotSymbols)
	}
	if len(gotSelections) != 0 {
		t.Errorf("selections: got %+v, want empty", gotSelections)
	}

	var out tools.SetTextElementsResult
	if err := json.Unmarshal([]byte(textOfTE(res)), &out); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, textOfTE(res))
	}
	if !out.Success || out.SymbolsCount != 1 || out.SelectionsCount != 0 || out.LockHandle != "lock-handle-X" {
		t.Errorf("result: %+v", out)
	}
}

func TestSetTextElementsTool_ExplicitLockHandleSkipsAutoLock(t *testing.T) {
	var locked bool
	var gotLock string

	mock := &mockClient{
		lockObjectFn: func(_ context.Context, _ string) (string, error) {
			locked = true
			return "auto-lock", nil
		},
		setTextElementsFn: func(_ context.Context, _ string, _ []adt.TextSymbol, _ []adt.SelectionText, lock, _ string) error {
			gotLock = lock
			return nil
		},
	}
	s := newTestServer(mock)

	res := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri":  testProgURI,
		"selections":  `[{"name":"P_X","text":"label"}]`,
		"lock_handle": "explicit-lock",
	})
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", textOfTE(res))
	}
	if locked {
		t.Errorf("LockObject should not be called when lock_handle is provided")
	}
	if gotLock != "explicit-lock" {
		t.Errorf("explicit lock_handle not passed through: got %q", gotLock)
	}
}

func TestSetTextElementsTool_RequiresAtLeastOnePayload(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)

	res := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": testProgURI,
	})
	if !res.IsError {
		t.Fatalf("expected IsError; got success: %s", textOfTE(res))
	}
	if !strings.Contains(textOfTE(res), "at least one of symbols or selections") {
		t.Errorf("error text mismatch: %s", textOfTE(res))
	}
}

func TestSetTextElementsTool_RejectsUnsupportedObjectType(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)

	res := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": "/sap/bc/adt/ddic/tables/ztable",
		"symbols":    `[{"key":"001","text":"x"}]`,
	})
	if !res.IsError {
		t.Fatalf("expected IsError on unsupported type")
	}
	if !strings.Contains(textOfTE(res), "text elements not supported") {
		t.Errorf("error text mismatch: %s", textOfTE(res))
	}
}

func TestSetTextElementsTool_InvalidSymbolsJSON(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)

	res := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": testProgURI,
		"symbols":    `[{"key":"001",`, // truncated
	})
	if !res.IsError {
		t.Fatalf("expected IsError on invalid JSON")
	}
	if !strings.Contains(textOfTE(res), "invalid symbols JSON") {
		t.Errorf("error text: %s", textOfTE(res))
	}
}
