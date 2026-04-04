package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestWithStringOrArray(t *testing.T) {
	tool := mcp.NewTool("test_tool",
		withStringOrArray("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	)
	prop, ok := tool.InputSchema.Properties["object_uri"].(map[string]any)
	if !ok {
		t.Fatal("object_uri property not found or wrong type")
	}
	oneOf, ok := prop["oneOf"].([]any)
	if !ok {
		t.Fatal("expected oneOf in schema")
	}
	if len(oneOf) != 2 {
		t.Fatalf("expected 2 oneOf entries, got %d", len(oneOf))
	}
	found := false
	for _, r := range tool.InputSchema.Required {
		if r == "object_uri" {
			found = true
		}
	}
	if !found {
		t.Error("object_uri should be required")
	}
}

func TestGetStringOrSlice_String(t *testing.T) {
	args := map[string]any{"object_uri": "/sap/bc/adt/programs/programs/ZTEST"}
	single, multi := getStringOrSlice(args, "object_uri")
	if single != "/sap/bc/adt/programs/programs/ZTEST" {
		t.Errorf("single: got %q", single)
	}
	if multi != nil {
		t.Errorf("multi should be nil, got %v", multi)
	}
}

func TestGetStringOrSlice_Array(t *testing.T) {
	args := map[string]any{
		"object_uri": []any{"/sap/bc/adt/programs/programs/ZA", "/sap/bc/adt/programs/programs/ZB"},
	}
	single, multi := getStringOrSlice(args, "object_uri")
	if single != "" {
		t.Errorf("single should be empty, got %q", single)
	}
	if len(multi) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(multi))
	}
	if multi[0] != "/sap/bc/adt/programs/programs/ZA" {
		t.Errorf("multi[0]: got %q", multi[0])
	}
}
