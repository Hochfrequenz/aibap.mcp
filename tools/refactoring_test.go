package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

const testRenameURI = "/sap/bc/adt/programs/programs/Z/source/main#start=5,7"

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
