package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// callTool invokes a tool via HandleMessage using JSON-RPC protocol.
// Calls t.Fatalf on any JSON-RPC-level error; to assert on tool-level
// errors, inspect result.IsError. Shared by unit and integration tests.
func callTool(t *testing.T, s *server.MCPServer, toolName string, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		toolName, string(argsJSON))

	resp := s.HandleMessage(context.Background(), []byte(msg))

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var envelope struct {
		Result *mcp.CallToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("unmarshal response envelope: %v\nraw: %s", err, string(respBytes))
	}
	if envelope.Error != nil {
		t.Fatalf("JSON-RPC error calling %q: code=%d msg=%s", toolName, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		t.Fatalf("nil result for tool %q\nraw: %s", toolName, string(respBytes))
	}
	return envelope.Result
}
