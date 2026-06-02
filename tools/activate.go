package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerActivateTools(s toolAdder, client interface {
	adt.ObjectClient
	adt.LockClient
}, lockMap *adt.LockMap, selector SystemSelector) {
	// unlockBeforeActivate releases locks held by the MCP session for the
	// given URIs. Activation replaces the inactive version — the lock from
	// set_source_from_file / patch_source is no longer needed and would
	// block activation with 403 "User is currently editing". See #301.
	unlockBeforeActivate := func(ctx context.Context, uris []string) {
		for _, uri := range uris {
			key := adt.LockKey(selector.ActiveName(), uri)
			if state, ok := lockMap.Get(key); ok && state.LockHandle != "" {
				_ = client.UnlockObject(ctx, uri, state.LockHandle)
				lockMap.Delete(key)
			}
		}
	}

	s.AddTool(mcp.NewTool("activate_objects",
		mcp.WithTitleAnnotation("Activate Objects"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Activate one or more ABAP objects in SAP. Returns activation result with per-object messages (E=error, W=warning, I=info)."),
		mcp.WithArray("object_uris",
			mcp.Required(),
			mcp.Description("List of ADT object URIs to activate"),
			mcp.WithStringItems(),
		),
		mcp.WithOutputSchema[adt.ActivationResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice("object_uris", nil)
		unlockBeforeActivate(ctx, uris)
		result, err := client.ActivateObjects(ctx, uris)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})

	s.AddTool(mcp.NewTool("get_inactive_objects",
		mcp.WithTitleAnnotation("Get Inactive Objects"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"List all inactive (not yet activated) ABAP objects for the current user. "+
				"Use this to check what needs activation before releasing a transport.",
		),
		mcp.WithOutputSchema[InactiveObjectsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objects, err := client.GetInactiveObjects(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(InactiveObjectsResult{Count: len(objects), Objects: objects})
	})

	// Backward-compatible alias: activate a single object by URI string.
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithTitleAnnotation("Activate Object"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Activate a single ABAP object."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithOutputSchema[adt.ActivationResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		unlockBeforeActivate(ctx, []string{uri})
		result, err := client.ActivateObjects(ctx, []string{uri})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
