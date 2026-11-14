package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockPackageFallback is a BlackMagicClient that also implements the optional
// tools.PackageFallback extension.
type mockPackageFallback struct {
	mockBlackMagicObj
	createFn func(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error
}

func (m *mockPackageFallback) CreatePackageFallback(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error {
	if m.createFn != nil {
		return m.createFn(ctx, name, description, responsible, softwareComponent, transportLayer, transport)
	}
	return nil
}

var _ tools.PackageFallback = (*mockPackageFallback)(nil)

// noPackageEndpoint answers like a system without /sap/bc/adt/packages (ECC):
// 404 on the package POST, success with a CSRF token on everything else.
type noPackageEndpoint struct{}

func (noPackageEndpoint) RoundTrip(req *http.Request) (*http.Response, error) {
	status := http.StatusOK
	if req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, "/sap/bc/adt/packages") {
		status = http.StatusNotFound
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"X-Csrf-Token": {"token"}},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

// packageEndpointMissing returns the error the real adtler client gives when
// the system has no package endpoint, so a change of adtler's wording breaks
// this test instead of silently disabling the fallback.
func packageEndpointMissing(t *testing.T) error {
	t.Helper()
	c := adt.NewClientWithTransport(sapmcpconfig.SAPSystem{Host: "http://sap-a.test", User: "u", Password: "p"}, noPackageEndpoint{})
	err := c.CreatePackage(context.Background(), "ZPACKAGE", "d", "DEVELOPER", "", "", "", "")
	if err == nil {
		t.Fatal("expected adtler to fail on a 404 from the package endpoint")
	}
	return err
}

func failingPackageClient(err error) *mockClient {
	return &mockClient{createPackageFn: func(context.Context, string, string, string, string, string, string) error { return err }}
}

var transportablePackageArgs = map[string]interface{}{
	"name":               "zpackage",
	"description":        "Transportable package",
	"responsible":        "developer",
	"software_component": "HOME",
	"transport_layer":    "ZDEV",
	"transport":          "EXAMPLE_REQUEST",
}

func packageText(r *mcp.CallToolResult) string { return r.Content[0].(mcp.TextContent).Text }

func TestCreatePackage_EndpointMissing_UsesFallback(t *testing.T) {
	var got [6]string
	fb := &mockPackageFallback{createFn: func(_ context.Context, name, desc, responsible, sc, tl, transport string) error {
		got = [6]string{name, desc, responsible, sc, tl, transport}
		return nil
	}}
	r := callTool(t, newTestServerWithObjFallback(failingPackageClient(packageEndpointMissing(t)), fb), "create_package", transportablePackageArgs)
	if r.IsError {
		t.Fatalf("expected success via fallback: %s", packageText(r))
	}
	// Name and responsible are upper-cased as adtler does for the ADT call, so
	// both routes hand SAP the same values.
	if want := [6]string{"ZPACKAGE", "Transportable package", "DEVELOPER", "HOME", "ZDEV", "EXAMPLE_REQUEST"}; got != want {
		t.Errorf("fallback got %v, want %v", got, want)
	}
	var out tools.PackageCreateResult
	if err := json.Unmarshal([]byte(packageText(r)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if want := (tools.PackageCreateResult{Name: "ZPACKAGE", Created: true, ViaFallback: true}); out != want {
		t.Errorf("got %+v, want %+v", out, want)
	}
}

// Should adtler classify the missing endpoint as not supported later, the
// fallback still applies.
func TestCreatePackage_NotSupported_UsesFallback(t *testing.T) {
	called := false
	err := &adt.ADTError{Type: adt.ExceptionTypeAddObjectUnsupported, Message: "not supported"}
	fb := &mockPackageFallback{createFn: func(context.Context, string, string, string, string, string, string) error { called = true; return nil }}
	r := callTool(t, newTestServerWithObjFallback(failingPackageClient(err), fb), "create_package", transportablePackageArgs)
	if r.IsError || !called {
		t.Fatalf("IsError=%v, fallback called=%v; want success via fallback: %s", r.IsError, called, packageText(r))
	}
}

func TestCreatePackage_FallbackError_IsReported(t *testing.T) {
	fb := &mockPackageFallback{createFn: func(context.Context, string, string, string, string, string, string) error {
		return errors.New("package ZPACKAGE already exists")
	}}
	r := callTool(t, newTestServerWithObjFallback(failingPackageClient(packageEndpointMissing(t)), fb), "create_package", transportablePackageArgs)
	if !r.IsError || !strings.Contains(packageText(r), "already exists") {
		t.Fatalf("expected the fallback error, got IsError=%v %s", r.IsError, packageText(r))
	}
}

// A BlackMagicClient without the extension, or none at all, keeps adtler's error.
func TestCreatePackage_EndpointMissing_NoExtension_KeepsError(t *testing.T) {
	for name, fb := range map[string]tools.BlackMagicClient{"without extension": &mockBlackMagicObj{}, "no fallback": nil} {
		t.Run(name, func(t *testing.T) {
			r := callTool(t, newTestServerWithObjFallback(failingPackageClient(packageEndpointMissing(t)), fb), "create_package", transportablePackageArgs)
			if !r.IsError || !strings.Contains(packageText(r), "SE21") {
				t.Fatalf("expected adtler's endpoint error, got IsError=%v %s", r.IsError, packageText(r))
			}
		})
	}
}

// An error SAP returned (the endpoint exists) is reported as is: the fallback
// is only for systems without the endpoint.
func TestCreatePackage_OtherError_NoFallback(t *testing.T) {
	called := false
	err := &adt.ADTError{StatusCode: 400, Message: "package ZPACKAGE already exists"}
	fb := &mockPackageFallback{createFn: func(context.Context, string, string, string, string, string, string) error { called = true; return nil }}
	r := callTool(t, newTestServerWithObjFallback(failingPackageClient(err), fb), "create_package", transportablePackageArgs)
	if !r.IsError || called {
		t.Errorf("IsError=%v, fallback called=%v; want the ADT error and no fallback", r.IsError, called)
	}
}

func TestCreatePackage_ADTSuccess_NotViaFallback(t *testing.T) {
	ok := &mockClient{createPackageFn: func(context.Context, string, string, string, string, string, string) error { return nil }}
	r := callTool(t, newTestServerWithObjFallback(ok, &mockPackageFallback{}), "create_package", transportablePackageArgs)
	var out tools.PackageCreateResult
	_ = json.Unmarshal([]byte(packageText(r)), &out)
	if r.IsError || out.ViaFallback || !out.Created {
		t.Errorf("IsError=%v, got %+v; want created over ADT", r.IsError, out)
	}
}
