package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerUnitTestTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("run_unit_tests",
		mcp.WithDescription("Run ABAP Unit Tests for an object. Returns test results with pass/fail counts."),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithNumber("timeout_seconds", mcp.Description("Test execution timeout in seconds (default: 30)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		timeout := req.GetInt("timeout_seconds", 30)
		result, err := client.RunUnitTests(ctx, uri, timeout)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
