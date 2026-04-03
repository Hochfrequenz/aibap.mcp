package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerActivateTools(s toolAdder, client adt.ObjectClient) {
	s.AddTool(mcp.NewTool("activate_objects",
		mcp.WithTitleAnnotation("Activate Objects"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Activate one or more ABAP objects in SAP. Returns activation result with per-object messages (E=error, W=warning, I=info)."),
		mcp.WithArray("object_uris",
			mcp.Required(),
			mcp.Description("List of ADT object URIs to activate"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice("object_uris", nil)
		result, err := client.ActivateObjects(ctx, uris)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
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
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objects, err := client.GetInactiveObjects(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(objects)
		return mcp.NewToolResultText(string(out)), nil
	})

	// Backward-compatible alias: activate a single object by URI string.
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithTitleAnnotation("Activate Object"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Activate a single ABAP object."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.ActivateObjects(ctx, []string{uri})
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
