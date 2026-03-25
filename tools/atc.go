package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerATCTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("get_atc_customizing",
		mcp.WithDescription("Get ATC (ABAP Test Cockpit) configuration including check variant and exemption reasons."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.GetATCCustomizing(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("run_atc_check",
		mcp.WithDescription("Run ATC (ABAP Test Cockpit) static analysis checks on one or more ABAP objects. Returns findings with priority, check title, and message."),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to check")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice("object_uris", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}

		result, err := client.RunATCCheck(ctx, uris)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
