package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

const testRenameURI = "/sap/bc/adt/programs/programs/Z/source/main#start=5,7"

func TestRename_RenamesWithoutAskingTheClient(t *testing.T) {
	called := false
	mock := &mockClient{
		renameFn: func(_ context.Context, _, _, _ string) (*adt.RenameResult, error) {
			called = true
			return &adt.RenameResult{}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)
	result := callTool(t, s, "rename", map[string]interface{}{
		"source_uri": testRenameURI,
		"new_name":   "NEW_SYM",
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor (backwards compat), got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected renameFn to be called with nil elicitor (backwards compat)")
	}
}
