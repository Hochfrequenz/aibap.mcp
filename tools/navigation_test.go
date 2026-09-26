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

// TestNavigateToDefinition_EmptySource verifies that an empty or
// whitespace-only "source" is rejected too, not just an absent one — the
// same foot-gun #386 targets, just without the trimming requireString does
// for other parameters (which would corrupt source_uri's position fragment).
func TestNavigateToDefinition_EmptySource(t *testing.T) {
	for _, source := range []string{"", "   "} {
		t.Run("q_"+source, func(t *testing.T) {
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
				"source":     source,
			})
			if !result.IsError {
				t.Fatalf("expected an empty source %q to be rejected", source)
			}
			if called {
				t.Errorf("the call should not have reached adtler with source %q", source)
			}
		})
	}
}

func TestNavigateToDefinition_NavigatesThroughToTheClient(t *testing.T) {
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

// TestNavigateToDefinition_SourceReachesTheClientUntrimmed guards against a
// regression to requireString (which trims): source_uri's #start=line,col
// fragment indexes positions in "source", so leading whitespace must survive
// unchanged all the way to the client call.
func TestNavigateToDefinition_SourceReachesTheClientUntrimmed(t *testing.T) {
	const padded = "  REPORT z_report.  "
	var gotSource string
	mock := &mockClient{
		navigateToDefinitionFn: func(_ context.Context, _, source string) (string, error) {
			gotSource = source
			return "", nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "navigate_to_definition", map[string]interface{}{
		"source_uri": testNavURI,
		"source":     padded,
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if gotSource != padded {
		t.Errorf("source reached the client as %q, want the untrimmed %q", gotSource, padded)
	}
}
