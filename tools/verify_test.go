package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	t.Run("failed cleanup keeps the result and names the program", func(t *testing.T) {
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
				return true, nil, &adt.VerifyCleanupError{Program: "Z_ADTLER_VERIFY_000001", Err: errors.New("delete failed")}
			},
		}
		result := callTool(t, newTestServer(mock), "verify_source", map[string]interface{}{"source": "REPORT zx."})
		if result.IsError {
			t.Fatalf("unexpected error: %v", result.Content)
		}
		var out struct {
			Valid        bool   `json:"valid"`
			CleanupError string `json:"cleanup_error"`
		}
		_ = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &out)
		if !out.Valid || !strings.Contains(out.CleanupError, "Z_ADTLER_VERIFY_000001") {
			t.Errorf("want valid=true and the program named in cleanup_error, got %+v", out)
		}
	})

	t.Run("failed check with failed cleanup is an error", func(t *testing.T) {
		mock := &mockClient{
			verifySourceFn: func(_ context.Context, _ string) (bool, []adt.SyntaxMessage, error) {
				leak := &adt.VerifyCleanupError{Program: "Z_ADTLER_VERIFY_000002", Err: errors.New("delete failed")}
				checkErr := &adt.ADTError{StatusCode: 500, Message: "internal server error"}
				return false, nil, fmt.Errorf("VerifySource: set source: %w; additionally: %w", checkErr, leak)
			},
		}
		result := callTool(t, newTestServer(mock), "verify_source", map[string]interface{}{"source": "REPORT zx."})
		if !result.IsError {
			t.Fatalf("want a tool error, got %v", result.Content)
		}
		text := result.Content[0].(mcp.TextContent).Text
		if strings.Contains(text, "Retry once") {
			t.Errorf("a leaked temporary program must not get a retry hint, got: %s", text)
		}
		if !strings.Contains(text, "Z_ADTLER_VERIFY_000002") || !strings.Contains(text, "Do not call verify_source again") {
			t.Errorf("want the leftover program named with no-retry guidance, got: %s", text)
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
