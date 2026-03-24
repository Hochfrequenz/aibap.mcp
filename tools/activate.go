package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerActivateTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("activate_objects",
		mcp.WithDescription("Activate one or more ABAP objects in SAP."),
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

	// Backward-compatible alias: activate a single object by URI string.
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithDescription("Activate a single ABAP object (alias for activate_objects)."),
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
