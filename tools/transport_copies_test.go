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

// mockCopiesFallback is a BlackMagicClient that also implements the optional
// tools.TransportCopiesFallback extension (#602).
type mockCopiesFallback struct {
	mockBlackMagic
	copyFn func(ctx context.Context, source, target string, withDocumentation bool) error
	pkgFn  func(ctx context.Context, pkg, target string, includeSubpackages bool) error
}

func (m *mockCopiesFallback) CopyTransportObjectsFallback(ctx context.Context, source, target string, withDocumentation bool) error {
	if m.copyFn != nil {
		return m.copyFn(ctx, source, target, withDocumentation)
	}
	return nil
}

func (m *mockCopiesFallback) AddPackageToTransportFallback(ctx context.Context, pkg, target string, includeSubpackages bool) error {
	if m.pkgFn != nil {
		return m.pkgFn(ctx, pkg, target, includeSubpackages)
	}
	return nil
}

var _ tools.TransportCopiesFallback = (*mockCopiesFallback)(nil)

func addObjectsUnsupported() error {
	return &adt.ADTError{Type: adt.ExceptionTypeAddObjectsUnsupported, Message: "transport request does not advertise adding objects from a request or package"}
}

const copiesTarget = "DEVK900002"

func copiesText(r *mcp.CallToolResult) string { return r.Content[0].(mcp.TextContent).Text }

func TestCopyTransportObjects_PassesArguments(t *testing.T) {
	var gotSrc, gotTgt string
	gotDoc := false
	client := &mockClient{copyTransportObjectsFn: func(_ context.Context, src, tgt string, withDoc bool) error {
		gotSrc, gotTgt, gotDoc = src, tgt, withDoc
		return nil
	}}
	r := callTool(t, newTestServer(client), "copy_transport_objects", map[string]interface{}{
		"source_request": testTransportDEVK1, "target_request": copiesTarget,
	})
	if r.IsError {
		t.Fatalf("unexpected error: %s", copiesText(r))
	}
	if gotSrc != testTransportDEVK1 || gotTgt != copiesTarget || !gotDoc {
		t.Errorf("got (%q, %q, %v), want (DEVK900001, DEVK900002, true): documentation is copied by default", gotSrc, gotTgt, gotDoc)
	}
	var out tools.CopyTransportObjectsResult
	_ = json.Unmarshal([]byte(copiesText(r)), &out)
	if !out.Copied || out.ViaFallback || out.SourceRequest != testTransportDEVK1 || out.TargetRequest != copiesTarget {
		t.Errorf("result %+v", out)
	}
}

func TestCopyTransportObjects_WithoutDocumentation(t *testing.T) {
	gotDoc := true
	client := &mockClient{copyTransportObjectsFn: func(_ context.Context, _, _ string, withDoc bool) error {
		gotDoc = withDoc
		return nil
	}}
	callTool(t, newTestServer(client), "copy_transport_objects", map[string]interface{}{
		"source_request": testTransportDEVK1, "target_request": copiesTarget, "with_documentation": false,
	})
	if gotDoc {
		t.Error("with_documentation=false did not reach adtler")
	}
}

func TestCopyTransportObjects_MissingParams(t *testing.T) {
	for _, missing := range []string{"source_request", "target_request"} {
		called := false
		client := &mockClient{copyTransportObjectsFn: func(context.Context, string, string, bool) error { called = true; return nil }}
		args := map[string]interface{}{"source_request": testTransportDEVK1, "target_request": copiesTarget}
		delete(args, missing)
		r := callTool(t, newTestServer(client), "copy_transport_objects", args)
		if !r.IsError || called {
			t.Errorf("missing %s: IsError=%v, reached adtler=%v", missing, r.IsError, called)
		}
	}
}

func TestCopyTransportObjects_NotSupported_UsesFallback(t *testing.T) {
	var got string
	client := &mockClient{copyTransportObjectsFn: func(context.Context, string, string, bool) error { return addObjectsUnsupported() }}
	fb := &mockCopiesFallback{copyFn: func(_ context.Context, src, tgt string, withDoc bool) error {
		got = src + ">" + tgt
		return nil
	}}
	r := callTool(t, newTestServerWithFallback(client, fb), "copy_transport_objects", map[string]interface{}{
		"source_request": testTransportDEVK1, "target_request": copiesTarget,
	})
	if r.IsError {
		t.Fatalf("expected success via fallback: %s", copiesText(r))
	}
	if got != "DEVK900001>DEVK900002" {
		t.Errorf("fallback got %q", got)
	}
	var out tools.CopyTransportObjectsResult
	_ = json.Unmarshal([]byte(copiesText(r)), &out)
	if !out.ViaFallback {
		t.Errorf("result %+v, want via_fallback", out)
	}
}

func TestCopyTransportObjects_NotSupported_NoFallback(t *testing.T) {
	client := &mockClient{copyTransportObjectsFn: func(context.Context, string, string, bool) error { return addObjectsUnsupported() }}
	r := callTool(t, newTestServerWithFallback(client, &mockBlackMagic{}), "copy_transport_objects", map[string]interface{}{
		"source_request": testTransportDEVK1, "target_request": copiesTarget,
	})
	if !r.IsError {
		t.Fatal("a BlackMagicClient without the extension must leave the ADT error")
	}
}

func TestCopyTransportObjects_OtherError_NoFallback(t *testing.T) {
	called := false
	client := &mockClient{copyTransportObjectsFn: func(context.Context, string, string, bool) error {
		return &adt.ADTError{StatusCode: 404, Message: "request does not exist"}
	}}
	fb := &mockCopiesFallback{copyFn: func(context.Context, string, string, bool) error { called = true; return nil }}
	r := callTool(t, newTestServerWithFallback(client, fb), "copy_transport_objects", map[string]interface{}{
		"source_request": testTransportDEVK1, "target_request": copiesTarget,
	})
	if !r.IsError || called {
		t.Errorf("IsError=%v, fallback called=%v", r.IsError, called)
	}
}

func TestAddPackageToTransport_PassesArguments(t *testing.T) {
	var gotPkg, gotTgt string
	gotSub := false
	client := &mockClient{addPackageToTransportFn: func(_ context.Context, pkg, tgt string, sub bool) error {
		gotPkg, gotTgt, gotSub = pkg, tgt, sub
		return nil
	}}
	r := callTool(t, newTestServer(client), "add_package_to_transport", map[string]interface{}{
		"package": "ZPKG", "target_request": copiesTarget, "include_subpackages": true,
	})
	if r.IsError {
		t.Fatalf("unexpected error: %s", copiesText(r))
	}
	if gotPkg != "ZPKG" || gotTgt != copiesTarget || !gotSub {
		t.Errorf("got (%q, %q, %v)", gotPkg, gotTgt, gotSub)
	}
	var out tools.AddPackageToTransportResult
	_ = json.Unmarshal([]byte(copiesText(r)), &out)
	if !out.Added || out.Package != "ZPKG" || !out.IncludeSubpackages {
		t.Errorf("result %+v", out)
	}
}

func TestAddPackageToTransport_NotSupported_FallbackFails(t *testing.T) {
	client := &mockClient{addPackageToTransportFn: func(context.Context, string, string, bool) error { return addObjectsUnsupported() }}
	fb := &mockCopiesFallback{pkgFn: func(context.Context, string, string, bool) error { return errors.New("target is released") }}
	r := callTool(t, newTestServerWithFallback(client, fb), "add_package_to_transport", map[string]interface{}{
		"package": "ZPKG", "target_request": copiesTarget,
	})
	text := copiesText(r)
	if !r.IsError || !strings.Contains(text, "target is released") || !strings.Contains(text, "does not advertise") {
		t.Errorf("want both errors, got IsError=%v %s", r.IsError, text)
	}
}
