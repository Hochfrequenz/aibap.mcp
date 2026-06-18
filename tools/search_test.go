package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestGetObjectDependenciesTool verifies the tool wiring: args are forwarded to
// adt.GetObjectDependencies and its result is returned. The DDIC/OO engine
// itself is tested in adtler.
func TestGetObjectDependenciesTool(t *testing.T) {
	var gotType, gotName string
	var gotMaxResults, gotMaxDepth int
	mock := &mockClient{
		getObjectDepsFn: func(_ context.Context, objType, objName string, maxResults, maxDepth int) (*adt.DependencyResult, error) {
			gotType, gotName, gotMaxResults, gotMaxDepth = objType, objName, maxResults, maxDepth
			return &adt.DependencyResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        1,
				Dependencies: []adt.ObjectDependency{{Name: "ZORDERS", UseType: adt.UseTypeTable}},
			}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_MY_REPORT",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// Default args forwarded.
	if gotType != "PROG" || gotName != "Z_MY_REPORT" || gotMaxResults != 200 || gotMaxDepth != 3 {
		t.Errorf("forwarded args: type=%q name=%q maxResults=%d maxDepth=%d", gotType, gotName, gotMaxResults, gotMaxDepth)
	}
	var out adt.DependencyResult
	_ = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &out)
	if out.Count != 1 || len(out.Dependencies) != 1 || out.Dependencies[0].Name != "ZORDERS" {
		t.Errorf("result: got %+v", out)
	}
}

func TestGetObjectDependenciesTool_Error(t *testing.T) {
	mock := &mockClient{
		getObjectDepsFn: func(_ context.Context, _, _ string, _, _ int) (*adt.DependencyResult, error) {
			return nil, &adt.ADTError{StatusCode: 400, Message: "unsupported object type"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "BOGUS",
		"object_name": "X",
	})
	if !result.IsError {
		t.Fatal("expected error result")
	}
}
