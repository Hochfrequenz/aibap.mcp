package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockEntryFallback is a BlackMagicClient that also implements the optional
// tools.TransportEntryFallback extension (#616).
type mockEntryFallback struct {
	mockBlackMagic
	addFn    func(ctx context.Context, pgmID, objectType, objectName, transport string) error
	removeFn func(ctx context.Context, task, parent, pgmID, objectType, objectName string) error
}

func (m *mockEntryFallback) AddToTransportFallback(ctx context.Context, pgmID, objectType, objectName, transport string) error {
	if m.addFn != nil {
		return m.addFn(ctx, pgmID, objectType, objectName, transport)
	}
	return nil
}

func (m *mockEntryFallback) RemoveFromTransportFallback(ctx context.Context, task, parent, pgmID, objectType, objectName string) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, task, parent, pgmID, objectType, objectName)
	}
	return nil
}

var _ tools.TransportEntryFallback = (*mockEntryFallback)(nil)

func addUnsupported() error {
	return &adt.ADTError{Type: adt.ExceptionTypeAddObjectUnsupported, Message: "transport request does not advertise adding an object; add the entry in SE09 instead"}
}

func removeUnsupported() error {
	return &adt.ADTError{Type: adt.ExceptionTypeRemoveObjectUnsupported, Message: "transport request does not advertise removing an object"}
}

var addArgs = map[string]interface{}{"object_type": "PROG", "object_name": "ZFOO", "transport": testTransportNum}

func entryText(r *mcp.CallToolResult) string { return r.Content[0].(mcp.TextContent).Text }

func TestAddToTransport_NotSupported_UsesFallback(t *testing.T) {
	var got [4]string
	client := &mockClient{addTransportFn: func(context.Context, string, string, string, string) error { return addUnsupported() }}
	fb := &mockEntryFallback{addFn: func(_ context.Context, pgmID, objectType, objectName, transport string) error {
		got = [4]string{pgmID, objectType, objectName, transport}
		return nil
	}}
	r := callTool(t, newTestServerWithFallback(client, fb), "add_to_transport", addArgs)
	if r.IsError {
		t.Fatalf("expected success via fallback: %s", entryText(r))
	}
	if want := [4]string{"R3TR", "PROG", "ZFOO", testTransportNum}; got != want {
		t.Errorf("fallback got %v, want %v", got, want)
	}
	var out tools.AddToTransportResult
	_ = json.Unmarshal([]byte(entryText(r)), &out)
	if !out.Added || !out.ViaFallback {
		t.Errorf("got %+v, want added via fallback", out)
	}
}

// A BlackMagicClient without the extension keeps today's error.
func TestAddToTransport_NotSupported_FallbackWithoutExtension(t *testing.T) {
	client := &mockClient{addTransportFn: func(context.Context, string, string, string, string) error { return addUnsupported() }}
	r := callTool(t, newTestServerWithFallback(client, &mockBlackMagic{}), "add_to_transport", addArgs)
	if !r.IsError || !strings.Contains(entryText(r), "SE09") {
		t.Fatalf("expected the ADT not-supported error, got IsError=%v %s", r.IsError, entryText(r))
	}
}

func TestAddToTransport_OtherError_NoFallback(t *testing.T) {
	called := false
	client := &mockClient{addTransportFn: func(context.Context, string, string, string, string) error {
		return &adt.ADTError{StatusCode: 400, Message: "object ZFOO does not exist"}
	}}
	fb := &mockEntryFallback{addFn: func(context.Context, string, string, string, string) error { called = true; return nil }}
	r := callTool(t, newTestServerWithFallback(client, fb), "add_to_transport", addArgs)
	if !r.IsError || called {
		t.Errorf("IsError=%v, fallback called=%v; want the ADT error and no fallback", r.IsError, called)
	}
}

func TestAddToTransport_FallbackFails_ReportsBoth(t *testing.T) {
	client := &mockClient{addTransportFn: func(context.Context, string, string, string, string) error { return addUnsupported() }}
	fb := &mockEntryFallback{addFn: func(context.Context, string, string, string, string) error {
		return errors.New("request is released")
	}}
	r := callTool(t, newTestServerWithFallback(client, fb), "add_to_transport", addArgs)
	text := entryText(r)
	if !r.IsError || !strings.Contains(text, "request is released") || !strings.Contains(text, "does not advertise adding") {
		t.Errorf("want both errors, got IsError=%v %s", r.IsError, text)
	}
}

func TestRemoveFromTransport_NotSupported_UsesFallback(t *testing.T) {
	var got [5]string
	client := &mockClient{removeFromTransportFn: func(context.Context, string, string, string, string, string, string, string) error {
		return removeUnsupported()
	}}
	fb := &mockEntryFallback{removeFn: func(_ context.Context, task, parent, pgmID, objectType, objectName string) error {
		got = [5]string{task, parent, pgmID, objectType, objectName}
		return nil
	}}
	r := callTool(t, newTestServerWithFallback(client, fb), "remove_from_transport", removeFromTransportArgs())
	if r.IsError {
		t.Fatalf("expected success via fallback: %s", entryText(r))
	}
	if want := [5]string{"S4UK902001", testTransportNum, "R3TR", "PROG", "ZFOO"}; got != want {
		t.Errorf("fallback got %v, want %v", got, want)
	}
	var out tools.RemoveFromTransportResult
	_ = json.Unmarshal([]byte(entryText(r)), &out)
	if !out.Removed || !out.ViaFallback {
		t.Errorf("got %+v, want removed via fallback", out)
	}
}

func TestRemoveFromTransport_OtherError_NoFallback(t *testing.T) {
	called := false
	client := &mockClient{removeFromTransportFn: func(context.Context, string, string, string, string, string, string, string) error {
		return &adt.ADTError{StatusCode: 400, Message: "no target user"}
	}}
	fb := &mockEntryFallback{removeFn: func(context.Context, string, string, string, string, string) error { called = true; return nil }}
	r := callTool(t, newTestServerWithFallback(client, fb), "remove_from_transport", removeFromTransportArgs())
	if !r.IsError || called {
		t.Errorf("IsError=%v, fallback called=%v; want the ADT error and no fallback", r.IsError, called)
	}
}

func TestRemoveFromTransport_NotSupported_NoFallback(t *testing.T) {
	client := &mockClient{removeFromTransportFn: func(context.Context, string, string, string, string, string, string, string) error {
		return removeUnsupported()
	}}
	r := callTool(t, newTestServerWithFallback(client, nil), "remove_from_transport", removeFromTransportArgs())
	if !r.IsError {
		t.Fatal("expected the ADT not-supported error without a fallback")
	}
}
