package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerEnhancementTools(s toolAdder, client adt.EnhancementClient) {
	s.AddTool(mcp.NewTool("get_badi_definition",
		mcp.WithDescription(
			"Read a BAdI enhancement spot by name. Returns all BAdI definitions within the spot, "+
				"including the BAdI interface, sample class, filter definitions, and single-use / fallback flags. "+
				"Use this to understand the extension points provided by a BAdI before implementing one.",
		),
		mcp.WithString("spot_name", mcp.Required(), mcp.Description("Enhancement spot name, e.g. 'BADI_ACC_DOCUMENT'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("spot_name", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "spot_name is required"}), nil
		}
		result, err := client.GetEnhancementSpot(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_badi_implementation",
		mcp.WithDescription(
			"Read a BAdI enhancement implementation by name. Returns the implementing classes, "+
				"the referenced enhancement spot and BAdI definition, and active/default flags. "+
				"Use this to find which classes implement a given BAdI.",
		),
		mcp.WithString("implementation_name", mcp.Required(), mcp.Description("Enhancement implementation name, e.g. 'ZEI_BADI_BPEM'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("implementation_name", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "implementation_name is required"}), nil
		}
		result, err := client.GetEnhancementImplementation(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
