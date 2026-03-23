package tools

import (
	"context"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerLockTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("lock_object",
		mcp.WithDescription("Lock an ABAP object for editing. Returns a lock handle that must be passed to set_source."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		handle, err := client.LockObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(handle), nil
	})

	s.AddTool(mcp.NewTool("unlock_object",
		mcp.WithDescription("Unlock a previously locked ABAP object."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if err := client.UnlockObject(ctx, uri); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object unlocked"), nil
	})
}
