package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// This file holds the shared machinery for the reflective guards that read the
// server's own tools/list output — TestStructuredContentIsObject and the
// consent guards in consent_test.go. Reading the wire form rather than the Go
// values that produced it is deliberate: it is the only representation a
// client ever sees.

// confirmProbeFallback is a no-op BlackMagicClient. update_customizing refuses
// outright when no fallback is configured, which would hide the behaviour
// under test; every method succeeds silently so the handler reaches its end.
type confirmProbeFallback struct{}

func (c *confirmProbeFallback) ReleaseTransportFallback(context.Context, string) error { return nil }

func (c *confirmProbeFallback) CreateTransportFallback(
	_ context.Context, _, _, _, _ string,
) (string, error) {
	return "", nil
}

func (c *confirmProbeFallback) UpdateCustomizing(
	_ context.Context, _ string, _ []tools.CustomizingEntry, _ string,
) error {
	return nil
}

func (c *confirmProbeFallback) CreateObjectFallback(
	_ context.Context, _, _, _, _, _ string,
) error {
	return nil
}

// listedTool is one entry of a tools/list result, as a client receives it.
type listedTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Meta         map[string]any `json:"_meta,omitempty"`
	Annotations  struct {
		ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
		DestructiveHint *bool `json:"destructiveHint,omitempty"`
	} `json:"annotations"`
}

// listRegisteredTools enumerates the tools a server exposes via tools/list.
// Reflective, so a tool added later is covered without touching the callers.
// Shared between the guards above so the two cannot drift in how they
// enumerate tools.
func listRegisteredTools(t *testing.T, s *server.MCPServer) []listedTool {
	t.Helper()
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var envelope struct {
		Result struct {
			Tools []listedTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v\nraw: %s", err, string(raw))
	}
	if len(envelope.Result.Tools) == 0 {
		t.Fatal("tools/list returned zero tools — test server misconfigured")
	}
	return envelope.Result.Tools
}

// toolResultText concatenates the text content of a tool result, so assertions
// do not depend on how many content blocks a handler emitted.
func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
