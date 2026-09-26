package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestVerifySourceTool(t *testing.T) {
	t.Run("valid source", func(t *testing.T) {
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
				return true, nil, nil
			},
		}
		s := newTestServer(mock)
		result := callTool(t, s, "verify_source", map[string]interface{}{"source": "REPORT zx."})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		var out struct {
			Valid    bool                `json:"valid"`
			Messages []adt.SyntaxMessage `json:"messages"`
		}
		_ = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &out)
		if !out.Valid {
			t.Errorf("expected valid=true, got %+v", out)
		}
	})

	t.Run("invalid source surfaces messages", func(t *testing.T) {
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
				return false, []adt.SyntaxMessage{{Type: "E", Text: "Field FOO is unknown"}}, nil
			},
		}
		s := newTestServer(mock)
		result := callTool(t, s, "verify_source", map[string]interface{}{"source": "REPORT zx. DATA x TYPE foo."})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		var out struct {
			Valid    bool                `json:"valid"`
			Messages []adt.SyntaxMessage `json:"messages"`
		}
		_ = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &out)
		if out.Valid {
			t.Errorf("expected valid=false, got %+v", out)
		}
		if len(out.Messages) != 1 || out.Messages[0].Type != "E" {
			t.Errorf("expected one E message, got %+v", out.Messages)
		}
	})

	t.Run("empty or whitespace-only source rejected before adtler call", func(t *testing.T) {
		for _, source := range []string{"", "   "} {
			t.Run("q_"+source, func(t *testing.T) {
				called := false
				mock := &mockClient{
					verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
						called = true
						return true, nil, nil
					},
				}
				s := newTestServer(mock)
				result := callTool(t, s, "verify_source", map[string]interface{}{"source": source})
				if !result.IsError {
					t.Fatalf("expected error for source %q", source)
				}
				if called {
					t.Errorf("VerifySource must not be called for source %q", source)
				}
			})
		}
	})

	t.Run("missing source rejected before adtler call", func(t *testing.T) {
		called := false
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
				called = true
				return true, nil, nil
			},
		}
		s := newTestServer(mock)
		result := callTool(t, s, "verify_source", map[string]interface{}{})
		if !result.IsError {
			t.Fatal("expected error for missing source")
		}
		if called {
			t.Error("VerifySource must not be called when source is missing")
		}
	})

	// TestVerifySourceTool/source_reaches_client_untrimmed guards against a
	// regression to requireString (which trims): leading/trailing whitespace
	// in ABAP source is significant and must survive unchanged.
	t.Run("source reaches client untrimmed", func(t *testing.T) {
		const padded = "  REPORT zx.  "
		var gotSource string
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, source string) (bool, []adt.SyntaxMessage, error) {
				gotSource = source
				return true, nil, nil
			},
		}
		s := newTestServer(mock)
		result := callTool(t, s, "verify_source", map[string]interface{}{"source": padded})
		if result.IsError {
			t.Fatalf("expected success, got error: %v", result.Content)
		}
		if gotSource != padded {
			t.Errorf("source reached the client as %q, want the untrimmed %q", gotSource, padded)
		}
	})
}
