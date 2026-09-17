package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// rollbackGateMock records whether `rollback_transport` reached the
// adt.RollbackTransport call — enough to exercise the elicitation gate without
// driving the (now adtler-side) rollback pipeline.
func rollbackGateMock(calledGate *bool) *mockClient {
	return &mockClient{
		rollbackTransportFn: func(_ context.Context, _ string) (*adt.RollbackResult, error) {
			if calledGate != nil {
				*calledGate = true
			}
			return &adt.RollbackResult{}, nil
		},
	}
}

func TestRollbackTransport_RollsBackWithoutAskingTheClient(t *testing.T) {
	called := false
	mock := rollbackGateMock(&called)
	s := newTestServerWithFallback(mock, nil)
	result := callTool(t, s, "rollback_transport", map[string]interface{}{
		"transport": testTransportNum,
	})
	if result.IsError {
		t.Fatalf("expected success with nil elicitor, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected rollbackTransportFn to be called with nil elicitor (backwards compat)")
	}
}
