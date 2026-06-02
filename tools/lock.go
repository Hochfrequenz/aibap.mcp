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
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Lock an ABAP object for editing. Returns a lock handle stored in the server lock map. Usually not needed — patch_source and set_source_from_file auto-lock."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithOutputSchema[LockResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		handle, err := client.LockObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.Set(adt.LockKey(selector.ActiveName(), uri), handle, "")
		return mcp.NewToolResultJSON(LockResult{Handle: handle})
	})

	s.AddTool(mcp.NewTool("unlock_object",
		mcp.WithTitleAnnotation("Unlock Object"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Unlock a previously locked ABAP object. Validates the supplied handle against the session lock map; rejects mismatched or untracked handles without contacting SAP."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle returned by lock_object (optional, looked up automatically from the session lock map)"),
		),
		mcp.WithOutputSchema[UnlockResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		lockHandle := req.GetString("lock_handle", "")
		key := adt.LockKey(selector.ActiveName(), uri)
		// SAP's UNLOCK endpoint returns 2xx regardless of whether the lock
		// existed, the handle was valid, or the URI matched (see #383).
		// Validate against the session lock map first so we never propagate
		// that lie and never wipe a real entry on a no-op call.
		state, tracked := lockMap.Get(key)
		if !tracked {
			return errorResult(fmt.Errorf("no lock tracked for %s in this session", uri)), nil
		}
		if lockHandle == "" {
			lockHandle = state.LockHandle
		} else if lockHandle != state.LockHandle {
			return errorResult(fmt.Errorf("lock_handle does not match the handle tracked for %s in this session", uri)), nil
		}
		if err := client.UnlockObject(ctx, uri, lockHandle); err != nil {
			return errorResult(err), nil
		}
		lockMap.Delete(key)
		return mcp.NewToolResultJSON(UnlockResult{URI: uri, Unlocked: true})
	})
}
