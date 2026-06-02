package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func registerSystemTools(s toolAdder, selector SystemSelector) {
	s.AddTool(mcp.NewTool("select_system",
		mcp.WithTitleAnnotation("Select SAP System"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription("Switch the active SAP system for all subsequent tool calls. System names are defined in systems.json (see https://github.com/Hochfrequenz/sap-mcp-config). Returns the active system name and host URL."),
		mcp.WithString("system",
			mcp.Required(),
			mcp.Description("Name of the system to activate, as defined in config.json (e.g. \"dev\", \"prod\")"),
		),
		mcp.WithOutputSchema[SelectSystemResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("system", "")
		msg, err := selector.Select(name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(SelectSystemResult{System: name, Message: msg})
	})
}
