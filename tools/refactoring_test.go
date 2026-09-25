package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

const testRenameURI = "/sap/bc/adt/programs/programs/Z/source/main#start=5,7"

// TestRename_MissingRequiredParams verifies that an omitted required
// parameter is rejected locally with a clear message instead of forwarding an
// empty string to adtler. See #386.
func TestRename_MissingRequiredParams(t *testing.T) {
	fullArgs := map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	}
	for _, missing := range []string{"source_uri", "new_name"} {
		t.Run(missing, func(t *testing.T) {
			called := false
			mock := &mockClient{
				renameFn: func(_ context.Context, _, _, _ string) (*adt.RenameResult, error) {
					called = true
					return &adt.RenameResult{}, nil
				},
			}
			args := map[string]interface{}{}
			for k, v := range fullArgs {
				if k != missing {
					args[k] = v
				}
			}
			s := newTestServerWithFallback(mock, nil)
			result := callTool(t, s, "rename", args)
			if !result.IsError {
				t.Fatalf("expected a missing %q to be rejected", missing)
			}
			if called {
				t.Errorf("the call should not have reached adtler with %q missing", missing)
			}
		})
	}
}

func TestRename_RenamesWithoutAskingTheClient(t *testing.T) {
	called := false
	var gotURI, gotNewName string
	mock := &mockClient{
		renameFn: func(_ context.Context, uri, newName, _ string) (*adt.RenameResult, error) {
			called = true
			gotURI, gotNewName = uri, newName
			return &adt.RenameResult{}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)
	result := callTool(t, s, "rename", map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected renameFn to be called")
	}
	// Three strings in a row: asserting which is which is what catches a
	// swapped argument, since either order compiles.
	if gotURI != testRenameURI {
		t.Errorf("source_uri reached the client as %q, want %q", gotURI, testRenameURI)
	}
	if gotNewName != "NEW_SYM" {
		t.Errorf("new_name reached the client as %q, want NEW_SYM", gotNewName)
	}
}
