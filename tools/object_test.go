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
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, nil)
	return s
}

func newTestServerWithObjFallbackElicitor(client *mockClient, fallback tools.BlackMagicClient, elicitor tools.Elicitor) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, &mockSelector{}, adt.NewLockMap(), tools.ParseToolGroups([]string{"all"}), fallback, elicitor)
	return s
}

func TestDeleteObject_ElicitationAccepted(t *testing.T) {
	called := false
	var gotURI string
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, uri, _, _ string) error {
			called = true
			gotURI = uri
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithObjFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteObjectFn to be called after accept")
	}
	if gotURI != "/sap/bc/adt/programs/programs/ZDEAD" {
		t.Errorf("object_uri: got %q", gotURI)
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestDeleteObject_ElicitationDeclined(t *testing.T) {
	called := false
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error {
			called = true
			return nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithObjFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	if !result.IsError {
		t.Fatal("expected error result when user declines")
	}
	if called {
		t.Fatal("deleteObjectFn must NOT be called when user declines")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "delete_object aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
}

func TestDeleteObject_NilElicitorProceedsForBackwardsCompat(t *testing.T) {
	called := false
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error {
			called = true
			return nil
		},
	}
	s := newTestServerWithObjFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteObjectFn to be called with nil elicitor (backwards compat)")
	}
}

func TestDeleteObject_ElicitationIncludesMetadataWhenAvailable(t *testing.T) {
	called := false
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error {
			called = true
			return nil
		},
		getObjectFn: func(_ context.Context, _ string) (*adt.ObjectInfo, error) {
			return &adt.ObjectInfo{Name: "ZDEAD", Type: "PROG/P", PackageName: "$TMP"}, nil
		},
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "AUTHOR"}, {Name: "CREATED_ON"}},
				Rows:    [][]string{{"KLEINK", "20260614"}},
			}, nil
		},
	}
	s := newTestServerWithObjFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteObjectFn to be called")
	}
	for _, want := range []string{"PROG", "ZDEAD", "$TMP", "KLEINK", "2026-06-14"} {
		if !strings.Contains(el.lastMessage, want) {
			t.Errorf("elicitation message should contain %q, got: %s", want, el.lastMessage)
		}
	}
}

func TestDeleteObject_ElicitationFallsBackToURIWhenGetObjectInfoFails(t *testing.T) {
	called := false
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	const uri = "/sap/bc/adt/programs/programs/ZDEAD"
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error {
			called = true
			return nil
		},
		getObjectFn: func(_ context.Context, _ string) (*adt.ObjectInfo, error) {
			return nil, fmt.Errorf("404 not found")
		},
	}
	s := newTestServerWithObjFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": uri,
	})
	if result.IsError {
		t.Fatalf("expected success on fallback, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected deleteObjectFn to be called even when metadata fetch fails")
	}
	if !strings.Contains(el.lastMessage, uri) {
		t.Errorf("fallback message should contain URI %q, got: %s", uri, el.lastMessage)
	}
}

func TestDeleteObject_ElicitationShowsTypeAndPackageWhenTADIRQueryFails(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	mock := &mockClient{
		deleteObjectFn: func(_ context.Context, _, _, _ string) error { return nil },
		getObjectFn: func(_ context.Context, _ string) (*adt.ObjectInfo, error) {
			return &adt.ObjectInfo{Name: "ZDEAD", Type: "PROG/P", PackageName: "$TMP"}, nil
		},
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, fmt.Errorf("TADIR query failed")
		},
	}
	s := newTestServerWithObjFallbackElicitor(mock, nil, el)
	callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})
	for _, want := range []string{"PROG", "ZDEAD", "$TMP"} {
		if !strings.Contains(el.lastMessage, want) {
			t.Errorf("message should contain %q even without TADIR, got: %s", want, el.lastMessage)
		}
	}
	if strings.Contains(el.lastMessage, "KLEINK") {
		t.Error("message should not contain author when TADIR query fails")
	}
}
