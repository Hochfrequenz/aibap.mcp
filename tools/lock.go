package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerLockTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("lock_object",
		mcp.WithDescription("Lock an ABAP object for editing. Returns a lock handle. The handle is stored in the server lock map automatically."),
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
		lockMap.Set(lockKey(selector, uri), handle, "")
		return mcp.NewToolResultText(handle), nil
	})

	s.AddTool(mcp.NewTool("unlock_object",
		mcp.WithDescription("Unlock a previously locked ABAP object."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle returned by lock_object (optional, looked up automatically)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		lockHandle := req.GetString("lock_handle", "")
		if lockHandle == "" {
			if state, ok := lockMap.Get(lockKey(selector, uri)); ok {
				lockHandle = state.LockHandle
			}
		}
		if lockHandle == "" {
			return errorResult(fmt.Errorf("no lock handle: object not locked")), nil
		}
		if err := client.UnlockObject(ctx, uri, lockHandle); err != nil {
			return errorResult(err), nil
		}
		lockMap.Delete(lockKey(selector, uri))
		return mcp.NewToolResultText("Object unlocked"), nil
	})
}
