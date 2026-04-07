package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
)

func listToolInputSchemas(t *testing.T) map[string]map[string]any {
	t.Helper()

	s := newTestServer(&mockClient{})
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var envelope struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list response: %v\nraw: %s", err, string(respBytes))
	}

	schemas := make(map[string]map[string]any, len(envelope.Result.Tools))
	for _, tool := range envelope.Result.Tools {
		schemas[tool.Name] = tool.InputSchema
	}
	return schemas
}

// findArraysMissingItems walks a JSON Schema fragment and returns JSON-pointer-like
// paths for every `type: "array"` node that lacks an `items` schema. It recurses
// into `properties`, `items`, and the combinator keywords `oneOf`/`anyOf`/`allOf`
// so nested array definitions are also checked.
func findArraysMissingItems(schema any, path string) []string {
	var problems []string
	switch node := schema.(type) {
	case map[string]any:
		if t, _ := node["type"].(string); t == "array" {
			if _, hasItems := node["items"]; !hasItems {
				problems = append(problems, path)
			}
		}
		if props, ok := node["properties"].(map[string]any); ok {
			keys := make([]string, 0, len(props))
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				problems = append(problems, findArraysMissingItems(props[k], path+"/properties/"+k)...)
			}
		}
		if items, ok := node["items"]; ok {
			problems = append(problems, findArraysMissingItems(items, path+"/items")...)
		}
		for _, key := range []string{"oneOf", "anyOf", "allOf"} {
			if list, ok := node[key].([]any); ok {
				for i, sub := range list {
					problems = append(problems, findArraysMissingItems(sub, fmt.Sprintf("%s/%s/%d", path, key, i))...)
				}
			}
		}
	}
	return problems
}

// TestAllToolArraySchemasHaveItems is regression-proof: it walks every registered
// tool's input schema and fails if any `type: "array"` node is missing `items`.
// Adding a new tool with `mcp.WithArray(...)` but no items helper will trip this
// test, preventing the issue #261 class of bug from coming back.
func TestAllToolArraySchemasHaveItems(t *testing.T) {
	schemas := listToolInputSchemas(t)
	if len(schemas) == 0 {
		t.Fatal("no tools returned from tools/list")
	}

	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		problems := findArraysMissingItems(schemas[name], "")
		if len(problems) > 0 {
			t.Errorf("tool %q has array schemas missing items at: %v", name, problems)
		}
	}
}

// TestKnownArrayItemTypes pins the item types for the four arrays that originally
// triggered issue #261. Catches accidental type changes (e.g. switching from
// string items to object items) that the generic walker would not flag.
func TestKnownArrayItemTypes(t *testing.T) {
	schemas := listToolInputSchemas(t)

	cases := []struct {
		toolName      string
		propertyName  string
		wantItemsType string
	}{
		{toolName: "activate_objects", propertyName: "object_uris", wantItemsType: "string"},
		{toolName: "run_atc_check", propertyName: "object_uris", wantItemsType: "string"},
		{toolName: "patch_source", propertyName: "operations", wantItemsType: "object"},
		{toolName: "update_customizing", propertyName: "entries", wantItemsType: "object"},
	}

	for _, tc := range cases {
		t.Run(tc.toolName+"."+tc.propertyName, func(t *testing.T) {
			schema, ok := schemas[tc.toolName]
			if !ok {
				t.Fatalf("tool %q not found", tc.toolName)
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("tool %q: properties missing or wrong type", tc.toolName)
			}
			prop, ok := props[tc.propertyName].(map[string]any)
			if !ok {
				t.Fatalf("tool %q: property %q missing or wrong type", tc.toolName, tc.propertyName)
			}
			items, ok := prop["items"].(map[string]any)
			if !ok {
				t.Fatalf("tool %q: property %q missing items schema", tc.toolName, tc.propertyName)
			}
			if got := items["type"]; got != tc.wantItemsType {
				t.Fatalf("tool %q: property %q items.type=%v want %q", tc.toolName, tc.propertyName, got, tc.wantItemsType)
			}
		})
	}
}
