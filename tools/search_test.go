package tools_test

import (
	"context"
	"encoding/json"
	"strings"
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

// TestGetObjectDependenciesAdvertisesUITypes guards the tool surface: callers
// pick object types out of tools/list, so UIAC and UIAD have to be named both
// in the tool description and in the object_type parameter description. They
// also have to be named among the types max_depth is ignored for, since both
// are resolved by a single non-recursive query and max_depth has no effect on
// them. max_results is also not applied on the UIAD path — one app entry has
// at most four launch targets — so that exception has to be advertised too,
// the same way max_depth's exemption list is.
func TestGetObjectDependenciesAdvertisesUITypes(t *testing.T) {
	s := newTestServer(&mockClient{})
	var tool listedTool
	for _, lt := range listRegisteredTools(t, s) {
		if lt.Name == "get_object_dependencies" {
			tool = lt
			break
		}
	}
	if tool.Name == "" {
		t.Fatal("get_object_dependencies is not registered")
	}
	props, _ := tool.InputSchema["properties"].(map[string]any)
	objType, _ := props["object_type"].(map[string]any)
	paramDesc, _ := objType["description"].(string)
	maxDepth, _ := props["max_depth"].(map[string]any)
	maxDepthDesc, _ := maxDepth["description"].(string)
	maxResults, _ := props["max_results"].(map[string]any)
	maxResultsDesc, _ := maxResults["description"].(string)
	for _, want := range []string{"UIAC", "UIAD"} {
		if !strings.Contains(tool.Description, want) {
			t.Errorf("tool description does not mention %s", want)
		}
		if !strings.Contains(paramDesc, want) {
			t.Errorf("object_type description does not mention %s: %q", want, paramDesc)
		}
		if !strings.Contains(maxDepthDesc, want) {
			t.Errorf("max_depth description does not name %s as one of the types it's ignored for: %q", want, maxDepthDesc)
		}
	}
	if !strings.Contains(maxResultsDesc, "UIAD") {
		t.Errorf("max_results description does not name UIAD as a type it is not applied to: %q", maxResultsDesc)
	}
}
