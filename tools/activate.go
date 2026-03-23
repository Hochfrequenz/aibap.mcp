package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerActivateTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithDescription("Activate an ABAP object in SAP. Returns success status and any activation messages."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.ActivateObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
