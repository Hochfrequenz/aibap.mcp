package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type stubElicitor struct {
	result *mcp.ElicitationResult
	err    error
	called int
}

func (s *stubElicitor) RequestElicitation(_ context.Context, _ mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	s.called++
	return s.result, s.err
}

func TestConfirmDestructive_Accepted(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	proceed, reason := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if !proceed {
		t.Fatalf("expected proceed=true, got false (reason=%q)", reason)
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitation call, got %d", el.called)
	}
}

func TestConfirmDestructive_AcceptedButFalse(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": false},
		},
	}}
	proceed, reason := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if proceed {
		t.Fatal("expected proceed=false when user accepts with confirm=false")
	}
	if reason == "" {
		t.Error("expected a non-empty decline reason")
	}
}

func TestConfirmDestructive_Declined(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	proceed, _ := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if proceed {
		t.Fatal("expected proceed=false on decline")
	}
}

func TestConfirmDestructive_Cancelled(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionCancel},
	}}
	proceed, _ := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if proceed {
		t.Fatal("expected proceed=false on cancel")
	}
}

func TestConfirmDestructive_ElicitationNotSupported(t *testing.T) {
	el := &stubElicitor{err: server.ErrElicitationNotSupported}
	proceed, reason := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if !proceed {
		t.Fatalf("expected proceed=true when client does not support elicitation (backwards compat), got false (%q)", reason)
	}
}

func TestConfirmDestructive_NoActiveSession(t *testing.T) {
	el := &stubElicitor{err: server.ErrNoActiveSession}
	proceed, _ := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if !proceed {
		t.Fatal("expected proceed=true when no active session (backwards compat)")
	}
}

func TestConfirmDestructive_NilElicitor(t *testing.T) {
	proceed, _ := tools.ConfirmDestructive(context.Background(), nil, "delete X?")
	if !proceed {
		t.Fatal("expected proceed=true when elicitor is nil (backwards compat for tests)")
	}
}

func TestConfirmDestructive_TransportError(t *testing.T) {
	el := &stubElicitor{err: errors.New("transport crashed")}
	proceed, reason := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if proceed {
		t.Fatal("expected proceed=false on unexpected transport error")
	}
	if reason == "" {
		t.Error("expected a non-empty reason on transport error")
	}
}

func TestConfirmDestructive_NilResult(t *testing.T) {
	// Some transports return (nil, nil) occasionally. Don't panic; treat as "do not proceed".
	el := &stubElicitor{result: nil, err: nil}
	proceed, reason := tools.ConfirmDestructive(context.Background(), el, "delete X?")
	if proceed {
		t.Fatal("expected proceed=false when elicitation returns nil result without error")
	}
	if reason == "" {
		t.Error("expected a non-empty reason on nil result")
	}
}
