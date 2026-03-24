package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerSystemTools(s toolAdder, selector SystemSelector) {
	s.AddTool(mcp.NewTool("select_system",
		mcp.WithDescription("Switch the active SAP system for all subsequent tool calls. Returns the active system name and host."),
		mcp.WithString("system",
			mcp.Required(),
			mcp.Description("Name of the system to activate, as defined in config.yaml (e.g. \"dev\", \"prod\")"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("system", "")
		msg, err := selector.Select(name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(msg), nil
	})
}
