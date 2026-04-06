package tools_test

import (
	"testing"
)

func TestSetTextElements_NoInput_ReturnsError(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
	})
	if !result.IsError {
		t.Fatal("expected error when neither symbols nor selections provided")
	}
}

func TestSetTextElements_WithSymbols_Succeeds(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
		"symbols": []map[string]interface{}{
			{"key": "001", "text": "Hello World", "max_length": 50},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestSetTextElements_WithSelections_Succeeds(t *testing.T) {
	s := newTestServer(&mockClient{})
	result := callTool(t, s, "set_text_elements", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
		"selections": []map[string]interface{}{
			{"name": "P_PARAM", "text": "My parameter"},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}
