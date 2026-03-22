package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSyntaxCheckTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("syntax_check",
		mcp.WithDescription("Run ABAP syntax check on an object. Returns list of syntax messages with line/column info."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("object_uri", "")
		msgs, err := client.SyntaxCheck(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(msgs)
		return mcp.NewToolResultText(string(out)), nil
	})
}
