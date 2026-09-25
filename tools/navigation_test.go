package tools_test

import (
	"context"
	"testing"
)

const testNavURI = "/sap/bc/adt/programs/programs/z_report/source/main#start=15,4"

// TestNavigateToDefinition_MissingSourceURI verifies that an omitted
// "source_uri" is rejected locally with a clear message instead of
// forwarding an empty string to adtler. See #386.
func TestNavigateToDefinition_MissingSourceURI(t *testing.T) {
	called := false
	mock := &mockClient{
		navigateToDefinitionFn: func(_ context.Context, _, _ string) (string, error) {
			called = true
			return "", nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "navigate_to_definition", map[string]interface{}{
		"source": "REPORT z_report.",
	})
	if !result.IsError {
		t.Fatal("expected a missing source_uri to be rejected")
	}
	if called {
		t.Error("the call should not have reached adtler with source_uri missing")
	}
}

// TestNavigateToDefinition_MissingSource verifies the same for "source",
// which cannot use the trimming requireString helper (its whitespace is
// significant) but must still guard against a missing key. See #386.
func TestNavigateToDefinition_MissingSource(t *testing.T) {
	called := false
	mock := &mockClient{
		navigateToDefinitionFn: func(_ context.Context, _, _ string) (string, error) {
			called = true
			return "", nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "navigate_to_definition", map[string]interface{}{
		"source_uri": testNavURI,
	})
	if !result.IsError {
		t.Fatal("expected a missing source to be rejected")
	}
	if called {
		t.Error("the call should not have reached adtler with source missing")
	}
}

func TestNavigateToDefinition_NavigatesWithoutAskingTheClient(t *testing.T) {
	called := false
	var gotURI, gotSource string
	mock := &mockClient{
		navigateToDefinitionFn: func(_ context.Context, uri, source string) (string, error) {
			called = true
			gotURI, gotSource = uri, source
			return "/sap/bc/adt/programs/programs/z_dep", nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "navigate_to_definition", map[string]interface{}{
		"source_uri": testNavURI,
		"source":     "REPORT z_report.",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected navigateToDefinitionFn to be called")
	}
	if gotURI != testNavURI {
		t.Errorf("source_uri reached the client as %q, want %q", gotURI, testNavURI)
	}
	if gotSource != "REPORT z_report." {
		t.Errorf("source reached the client as %q, want %q", gotSource, "REPORT z_report.")
	}
}
