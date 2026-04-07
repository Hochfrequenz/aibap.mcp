package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// listToolInputSchemas drives the MCP server through a `tools/list` call and
// returns each tool's input schema as the raw map[string]any wire form. The
// schema validation tests in schema_client_validation_test.go feed these
// maps into a real JSON Schema 2020-12 compiler, which is the source of
// truth for "would a real client accept this?". This helper is shared by
// both files so the tools are exercised through the same code path the
// model uses to discover them.
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

// arrayItems looks up the items schema for an array property on a tool.
// Fails the test if any step is missing.
func arrayItems(t *testing.T, schemas map[string]map[string]any, toolName, propertyName string) map[string]any {
	t.Helper()
	schema, ok := schemas[toolName]
	if !ok {
		t.Fatalf("tool %q not found", toolName)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q: properties missing or wrong type", toolName)
	}
	prop, ok := props[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("tool %q: property %q missing or wrong type", toolName, propertyName)
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q: property %q missing items schema", toolName, propertyName)
	}
	return items
}

// TestKnownArrayItemShapes pins the item shapes for the four arrays that
// originally triggered issue #261. The generic walker only checks that
// `items` exists; this test asserts the *contents* of those items so a
// future regression cannot quietly loosen them back to bare schemas (the
// problem #263 fixed).
func TestKnownArrayItemShapes(t *testing.T) {
	schemas := listToolInputSchemas(t)

	t.Run("activate_objects.object_uris", func(t *testing.T) {
		assertStringItems(t, arrayItems(t, schemas, "activate_objects", "object_uris"))
	})
	t.Run("run_atc_check.object_uris", func(t *testing.T) {
		assertStringItems(t, arrayItems(t, schemas, "run_atc_check", "object_uris"))
	})
	t.Run("update_customizing.entries", func(t *testing.T) {
		assertCustomizingEntriesItems(t, arrayItems(t, schemas, "update_customizing", "entries"))
	})
	t.Run("patch_source.operations", func(t *testing.T) {
		assertPatchOperationsItems(t, arrayItems(t, schemas, "patch_source", "operations"))
	})
}

// assertStringItems asserts an items schema is a primitive string.
func assertStringItems(t *testing.T, items map[string]any) {
	t.Helper()
	if got := items["type"]; got != "string" {
		t.Fatalf("items.type=%v want \"string\"", got)
	}
}

// stringSet returns the set of string values in a []any (e.g. a JSON Schema
// `required` array). Non-string entries are ignored.
func stringSet(raw []any) map[string]bool {
	out := make(map[string]bool, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out[s] = true
		}
	}
	return out
}

// assertClosedObjectShape verifies a JSON Schema fragment is a closed object
// (additionalProperties: false) and that every name in `wantFields` appears
// in BOTH `properties` and `required`. Asymmetric coverage would let a
// contributor drop a field from one but not the other, leaving a malformed
// schema. label is used in error messages.
func assertClosedObjectShape(t *testing.T, label string, schema map[string]any, wantFields []string) {
	t.Helper()
	if schema["type"] != "object" {
		t.Errorf("%s: type=%v want \"object\"", label, schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties=%v want false", label, schema["additionalProperties"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		t.Fatalf("%s: properties missing or empty (would re-introduce #263): %v", label, schema["properties"])
	}
	req, ok := schema["required"].([]any)
	if !ok || len(req) == 0 {
		t.Fatalf("%s: required missing or empty (would re-introduce #263): %v", label, schema["required"])
	}
	gotRequired := stringSet(req)
	for _, want := range wantFields {
		if _, ok := props[want]; !ok {
			t.Errorf("%s: properties missing %q", label, want)
		}
		if !gotRequired[want] {
			t.Errorf("%s: required missing %q", label, want)
		}
	}
}

// assertCustomizingEntriesItems asserts the update_customizing.entries items
// schema describes the CustomizingEntry runtime shape (keys + values, both
// required, both string-to-string maps).
func assertCustomizingEntriesItems(t *testing.T, items map[string]any) {
	t.Helper()
	assertClosedObjectShape(t, "items", items, []string{"keys", "values"})
}

// patchOpWantBranches enumerates the per-discriminator required-field sets
// the patch_source.operations schema must contain. Mirrors the runtime
// dispatch in adt/patch.go (applyLineOp + the search_replace block).
var patchOpWantBranches = map[string][]string{
	"insert":         {"type", "after_line", "content"},
	"replace":        {"type", "from_line", "to_line", "content"},
	"delete":         {"type", "from_line", "to_line"},
	"search_replace": {"type", "search", "replace"},
}

// assertPatchOperationsItems asserts the patch_source.operations items schema
// is a discriminated `oneOf` with one closed-object branch per PatchOp variant.
func assertPatchOperationsItems(t *testing.T, items map[string]any) {
	t.Helper()
	oneOf, ok := items["oneOf"].([]any)
	if !ok {
		t.Fatalf("items.oneOf missing or wrong type (would re-introduce #263): %v", items["oneOf"])
	}
	if len(oneOf) != len(patchOpWantBranches) {
		t.Fatalf("oneOf has %d branches, want %d", len(oneOf), len(patchOpWantBranches))
	}
	seen := map[string]bool{}
	for i, raw := range oneOf {
		branch, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("oneOf[%d]: not an object", i)
			continue
		}
		discriminator := patchOpDiscriminator(t, i, branch)
		if discriminator == "" {
			continue
		}
		wantReq, known := patchOpWantBranches[discriminator]
		if !known {
			t.Errorf("oneOf[%d]: unknown discriminator %q", i, discriminator)
			continue
		}
		if seen[discriminator] {
			t.Errorf("oneOf[%d]: duplicate discriminator %q", i, discriminator)
		}
		seen[discriminator] = true
		assertClosedObjectShape(t, fmt.Sprintf("oneOf[%d] (%s)", i, discriminator), branch, wantReq)
	}
	for d := range patchOpWantBranches {
		if !seen[d] {
			t.Errorf("oneOf missing branch for discriminator %q", d)
		}
	}
}

// patchOpDiscriminator extracts and returns the single enum value pinned on
// branch.properties.type. Reports per-branch errors via t and returns "" if
// the discriminator is malformed.
func patchOpDiscriminator(t *testing.T, i int, branch map[string]any) string {
	t.Helper()
	props, ok := branch["properties"].(map[string]any)
	if !ok {
		t.Errorf("oneOf[%d]: properties missing", i)
		return ""
	}
	typeProp, ok := props["type"].(map[string]any)
	if !ok {
		t.Errorf("oneOf[%d]: type discriminator missing", i)
		return ""
	}
	enum, ok := typeProp["enum"].([]any)
	if !ok || len(enum) != 1 {
		t.Errorf("oneOf[%d].type.enum=%v want single-value enum", i, typeProp["enum"])
		return ""
	}
	d, _ := enum[0].(string)
	return d
}
