package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerCompletionTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("get_completions",
		mcp.WithDescription("Get ABAP code completion proposals at a specific cursor position."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Current ABAP source code"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Cursor line number (1-based)"),
		),
		mcp.WithNumber("column",
			mcp.Required(),
			mcp.Description("Cursor column number (1-based)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		source := req.GetString("source", "")
		line := int(req.GetFloat("line", 0))
		column := int(req.GetFloat("column", 0))
		items, err := client.GetCompletions(ctx, uri, source, line, column)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(items)
		return mcp.NewToolResultText(string(out)), nil
	})
}
