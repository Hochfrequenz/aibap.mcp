package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func reqWithArgs(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func resultText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestRequireString(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		wantVal  string
		wantErr  bool
		errParts []string // substrings the error text must contain
	}{
		{name: "present", args: map[string]any{"transport": "DEVK900123"}, key: "transport", wantVal: "DEVK900123"},
		{name: "trimmed", args: map[string]any{"transport": "  DEVK900123 "}, key: "transport", wantVal: "DEVK900123"},
		// Assert only on this repo's stable wrapper wording + the param name,
		// never on mcp-go's internal phrasing (which could change on a bump).
		{name: "absent", args: map[string]any{}, key: "transport", wantErr: true, errParts: []string{"invalid required parameter", "transport"}},
		{name: "nil args", args: nil, key: "transport", wantErr: true, errParts: []string{"invalid required parameter", "transport"}},
		{name: "wrong key", args: map[string]any{"transprt": "x"}, key: "transport", wantErr: true, errParts: []string{"invalid required parameter", "transport"}},
		{name: "empty", args: map[string]any{"transport": ""}, key: "transport", wantErr: true, errParts: []string{"must not be empty", "transport"}},
		{name: "whitespace only", args: map[string]any{"transport": "   "}, key: "transport", wantErr: true, errParts: []string{"must not be empty", "transport"}},
		{name: "wrong type", args: map[string]any{"transport": 42}, key: "transport", wantErr: true, errParts: []string{"invalid required parameter", "transport"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, errRes := requireString(reqWithArgs(tt.args), tt.key)
			if tt.wantErr {
				if errRes == nil {
					t.Fatalf("expected an error result, got value %q", val)
				}
				if !errRes.IsError {
					t.Error("error result should have IsError=true")
				}
				text := resultText(errRes)
				for _, part := range tt.errParts {
					if !strings.Contains(text, part) {
						t.Errorf("error text %q should contain %q", text, part)
					}
				}
				if val != "" {
					t.Errorf("value should be empty on error, got %q", val)
				}
				return
			}
			if errRes != nil {
				t.Fatalf("unexpected error result: %s", resultText(errRes))
			}
			if val != tt.wantVal {
				t.Errorf("got %q, want %q", val, tt.wantVal)
			}
		})
	}
}

// TestRequireRawString covers the untrimmed variant used for parameters
// where whitespace is significant (e.g. navigate_to_definition's "source"):
// unlike requireString, a present-but-whitespace-only value is returned
// unchanged rather than rejected — callers needing an emptiness check do it
// themselves on a trimmed copy, without altering what they forward.
func TestRequireRawString(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		key      string
		wantVal  string
		wantErr  bool
		errParts []string
	}{
		{name: "present", args: map[string]any{"source": "REPORT z."}, key: "source", wantVal: "REPORT z."},
		{name: "not trimmed", args: map[string]any{"source": "  REPORT z.  "}, key: "source", wantVal: "  REPORT z.  "},
		{name: "whitespace only is returned, not rejected", args: map[string]any{"source": "   "}, key: "source", wantVal: "   "},
		{name: "absent", args: map[string]any{}, key: "source", wantErr: true, errParts: []string{"invalid required parameter", "source"}},
		{name: "wrong type", args: map[string]any{"source": 42}, key: "source", wantErr: true, errParts: []string{"invalid required parameter", "source"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, errRes := requireRawString(reqWithArgs(tt.args), tt.key)
			if tt.wantErr {
				if errRes == nil {
					t.Fatalf("expected an error result, got value %q", val)
				}
				text := resultText(errRes)
				for _, part := range tt.errParts {
					if !strings.Contains(text, part) {
						t.Errorf("error text %q should contain %q", text, part)
					}
				}
				return
			}
			if errRes != nil {
				t.Fatalf("unexpected error result: %s", resultText(errRes))
			}
			if val != tt.wantVal {
				t.Errorf("got %q, want %q", val, tt.wantVal)
			}
		})
	}
}
