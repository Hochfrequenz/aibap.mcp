package tools_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestCreateObject_MissingRequiredParams verifies that an omitted required
// parameter is rejected locally with a clear message instead of forwarding an
// empty string to adtler. See #386.
func TestCreateObject_MissingRequiredParams(t *testing.T) {
	fullArgs := map[string]interface{}{
		"object_type": "PROG",
		"name":        "ZTEST_PROG",
		"package":     "$TMP",
		"description": "Test",
	}
	for _, missing := range []string{"object_type", "name", "package", "description"} {
		t.Run(missing, func(t *testing.T) {
			called := false
			client := &mockClient{
				createObjectFn: func(_ context.Context, _, _, _, _, _ string) error {
					called = true
					return nil
				},
			}
			args := map[string]interface{}{}
			for k, v := range fullArgs {
				if k != missing {
					args[k] = v
				}
			}
			s := newTestServer(client)
			result := callTool(t, s, "create_object", args)
			if !result.IsError {
				t.Fatalf("expected a missing %q to be rejected", missing)
			}
			if called {
				t.Errorf("the call should not have reached adtler with %q missing", missing)
			}
			if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, missing) {
				t.Errorf("error text should name the missing parameter %q, got: %s", missing, text)
			}
		})
	}
}

func TestCreateObject_DDIC_404_NoFallback_ReturnsError(t *testing.T) {
	client := &mockClient{
		createObjectFn: func(_ context.Context, _, _, _, _, _ string) error {
			return fmt.Errorf("SAP ADT error 404: not found")
		},
	}
	s := newTestServer(client)
	for _, objType := range []string{"TABL", "DTEL", "DOMA"} {
		result := callTool(t, s, "create_object", map[string]interface{}{
			"object_type": objType,
			"name":        "ZTEST_OBJ",
			"package":     "$TMP",
			"description": "Test",
		})
		if !result.IsError {
			t.Fatalf("%s: expected error for DDIC 404 without fallback", objType)
		}
		text := result.Content[0].(mcp.TextContent).Text
		for _, want := range []string{"S4-only", "SE11", "BlackMagic"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: error should contain %q, got: %s", objType, want, text)
			}
		}
	}
}

func TestCreateObject_DDIC_404_WithFallback_UsesFallback(t *testing.T) {
	client := &mockClient{
		createObjectFn: func(_ context.Context, _, _, _, _, _ string) error {
			return fmt.Errorf("SAP ADT error 404: not found")
		},
	}
	called := false
	fb := &mockBlackMagicObj{
		createObjectFallbackFn: func(_ context.Context, objectType, name, pkg, desc, transport string) error {
			called = true
			if objectType != "TABL" {
				t.Errorf("expected TABL, got %s", objectType)
			}
			if name != "ZTEST_TABLE" {
				t.Errorf("expected ZTEST_TABLE, got %s", name)
			}
			return nil
		},
	}
	s := newTestServerWithObjFallback(client, fb)
	result := callTool(t, s, "create_object", map[string]interface{}{
		"object_type": "TABL",
		"name":        "ZTEST_TABLE",
		"package":     "$TMP",
		"description": "Test table",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected BlackMagic fallback to be called")
	}
}

func TestCreateObject_DDIC_NonError_NoFallback(t *testing.T) {
	// On S4 systems, DDIC creation works via ADT — no fallback needed.
	client := &mockClient{
		createObjectFn: func(_ context.Context, _, _, _, _, _ string) error {
			return nil
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_object", map[string]interface{}{
		"object_type": "TABL",
		"name":        "ZTEST_TABLE",
		"package":     "$TMP",
		"description": "Test table",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestCreateObject_NonDDIC_404_NoFallback(t *testing.T) {
	// Non-DDIC types should NOT trigger fallback logic, just return the error.
	client := &mockClient{
		createObjectFn: func(_ context.Context, _, _, _, _, _ string) error {
			return fmt.Errorf("SAP ADT error 404: not found")
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_object", map[string]interface{}{
		"object_type": "PROG",
		"name":        "ZTEST_PROG",
		"package":     "$TMP",
		"description": "Test",
	})
	if !result.IsError {
		t.Fatal("expected error for PROG 404")
	}
	text := result.Content[0].(mcp.TextContent).Text
	// Should NOT mention SE11 — this is not a DDIC fallback scenario.
	if strings.Contains(text, "SE11") {
		t.Errorf("non-DDIC 404 should not mention SE11, got: %s", text)
	}
}

// mockBlackMagicObj implements tools.BlackMagicClient for object creation tests.
type mockBlackMagicObj struct {
	createObjectFallbackFn func(ctx context.Context, objectType, name, pkg, description, transport string) error
}

func (m *mockBlackMagicObj) ReleaseTransportFallback(context.Context, string) error {
	return nil
}

func (m *mockBlackMagicObj) CreateTransportFallback(context.Context, string, string, string, string) (string, error) {
	return "", nil
}

func (m *mockBlackMagicObj) UpdateCustomizing(context.Context, string, []tools.CustomizingEntry, string) error {
	return nil
}

func (m *mockBlackMagicObj) CreateObjectFallback(ctx context.Context, objectType, name, pkg, description, transport string) error {
	if m.createObjectFallbackFn != nil {
		return m.createObjectFallbackFn(ctx, objectType, name, pkg, description, transport)
	}
	return nil
}

func newTestServerWithObjFallback(client *mockClient, fallback tools.BlackMagicClient) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback)
	return s
}

func TestDeleteObject_DeletesWithoutAskingTheClient(t *testing.T) {
	called := false
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithObjFallback(mock, nil)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteObjectFn to be called: this server no longer gates the call itself")
	}
}

// createPackageArgs records what reached adtler's CreatePackage, so the tests
// can assert the tool passes parameters through rather than reshaping them.
type createPackageArgs struct {
	name, desc, responsible, softwareComponent, transportLayer, transport string
}

func recordingPackageClient(got *createPackageArgs) *mockClient {
	return &mockClient{
		createPackageFn: func(_ context.Context, name, desc, responsible, sc, tl, transport string) error {
			*got = createPackageArgs{name, desc, responsible, sc, tl, transport}
			return nil
		},
	}
}

func TestCreatePackage_LocalPackagePassesParametersThroughUnchanged(t *testing.T) {
	// SAP files a '$' package under software component LOCAL on its own, so the
	// tool sends what the caller gave it and invents nothing.
	var got createPackageArgs
	s := newTestServer(recordingPackageClient(&got))
	result := callTool(t, s, "create_package", map[string]interface{}{
		"name":        "$ZLOCAL_TEST",
		"description": "Local test package",
		"responsible": "DEVELOPER",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	want := createPackageArgs{"$ZLOCAL_TEST", "Local test package", "DEVELOPER", "", "", ""}
	if got != want {
		t.Errorf("parameters reshaped:\n got %+v\nwant %+v", got, want)
	}
}

func TestCreatePackage_TransportablePackagePassesParametersThroughUnchanged(t *testing.T) {
	var got createPackageArgs
	s := newTestServer(recordingPackageClient(&got))
	result := callTool(t, s, "create_package", map[string]interface{}{
		"name":               "ZTRANSPORTABLE",
		"description":        "Transportable package",
		"responsible":        "DEVELOPER",
		"software_component": "HOME",
		"transport_layer":    "ZDEV",
		"transport":          "EXAMPLE_REQUEST",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	want := createPackageArgs{"ZTRANSPORTABLE", "Transportable package", "DEVELOPER", "HOME", "ZDEV", "EXAMPLE_REQUEST"}
	if got != want {
		t.Errorf("parameters reshaped:\n got %+v\nwant %+v", got, want)
	}
}

func TestCreatePackage_ResponsibleIsRequired(t *testing.T) {
	// Measured on S/4 (SAP_BASIS 816, S4CORE 109): a package POST with an empty
	// adtcore:responsible is rejected with 400 ExceptionInvalidData, so the
	// parameter is declared required rather than left to fail at the server.
	called := false
	client := &mockClient{
		createPackageFn: func(_ context.Context, _, _, _, _, _, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_package", map[string]interface{}{
		"name":        "$ZLOCAL_TEST",
		"description": "Local test package",
	})
	if !result.IsError {
		t.Fatal("expected a missing responsible to be rejected")
	}
	if called {
		t.Error("the call should not have reached adtler")
	}
}

func TestCreatePackage_ResultReportsTheUpperCasedName(t *testing.T) {
	client := &mockClient{
		createPackageFn: func(_ context.Context, _, _, _, _, _, _ string) error { return nil },
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_package", map[string]interface{}{
		"name":        "$zlocal_test",
		"description": "Local test package",
		"responsible": "DEVELOPER",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// adtler upper-cases the name before sending it; the result must say what
	// SAP actually holds, not what the caller typed.
	if text := textOfTE(result); !strings.Contains(text, "$ZLOCAL_TEST") {
		t.Errorf("result should report the upper-cased name, got: %s", text)
	}
}

func TestCreatePackage_SurfacesClientError(t *testing.T) {
	client := &mockClient{
		createPackageFn: func(_ context.Context, _, _, _, _, _, _ string) error {
			return fmt.Errorf("CreatePackage: the /sap/bc/adt/packages endpoint is not available on this SAP system")
		},
	}
	s := newTestServer(client)
	result := callTool(t, s, "create_package", map[string]interface{}{
		"name":        "$ZLOCAL_TEST",
		"description": "Local test package",
		"responsible": "DEVELOPER",
	})
	if !result.IsError {
		t.Fatal("expected the adtler error to be surfaced")
	}
	if text := textOfTE(result); !strings.Contains(text, "not available on this SAP system") {
		t.Errorf("error text should be preserved, got: %s", text)
	}
}
