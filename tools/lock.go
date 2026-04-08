package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerLockTools(s toolAdder, client adt.LockClient, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("lock_object",
		mcp.WithTitleAnnotation("Lock Object"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Lock an ABAP object for editing. Returns a lock handle stored in the server lock map. Usually not needed — patch_source and set_source_from_file auto-lock."),
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
		lockMap.Set(adt.LockKey(selector.ActiveName(), uri), handle, "")
		return mcp.NewToolResultText(handle), nil
	})

	s.AddTool(mcp.NewTool("unlock_object",
		mcp.WithTitleAnnotation("Unlock Object"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
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
		key := adt.LockKey(selector.ActiveName(), uri)
		if lockHandle == "" {
			if state, ok := lockMap.Get(key); ok {
				lockHandle = state.LockHandle
			}
		}
		if lockHandle == "" {
			return errorResult(fmt.Errorf("no lock handle: object not locked")), nil
		}
		if err := client.UnlockObject(ctx, uri, lockHandle); err != nil {
			return errorResult(err), nil
		}
		lockMap.Delete(key)
		return mcp.NewToolResultText("Object unlocked"), nil
	})
}
