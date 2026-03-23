package tools

import (
	"context"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerPrettyPrinterTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("pretty_print",
		mcp.WithDescription("Format ABAP source code using the SAP Pretty Printer."),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code to format"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source := req.GetString("source", "")
		formatted, err := client.PrettyPrint(ctx, source)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(formatted), nil
	})
}
