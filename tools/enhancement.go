package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerEnhancementTools(s toolAdder, client adt.EnhancementClient) {
	s.AddTool(mcp.NewTool("get_badi_definition",
		mcp.WithTitleAnnotation("Get BAdI Definition"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read a BAdI enhancement spot definition. Returns all BAdI definitions within the spot, "+
				"including the BAdI interface, sample class, filter definitions, and single-use / fallback flags. "+
				"Use this to understand the extension points before implementing a BAdI. "+
				"Note: creating enhancement implementations requires SAP GUI (transaction SE19).",
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
		mcp.WithTitleAnnotation("Get BAdI Implementation"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read a BAdI enhancement implementation by name. Returns the implementing classes, "+
				"the referenced enhancement spot and BAdI definition, and active/default flags. "+
				"Use this to find which classes implement a given BAdI. "+
				"Note: creating new enhancement implementations requires SAP GUI (transaction SE19) — "+
				"ADT REST does not support ENHO creation. Use set_badi_implementation to update existing ones.",
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
		out, _ := json.Marshal(struct {
			*adt.BAdIImplementationInfo
			RawXML string `json:"raw_xml"`
		}{result, result.RawXML})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("set_badi_implementation",
		mcp.WithTitleAnnotation("Update BAdI Implementation"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Update an existing BAdI enhancement implementation. Sends the full XML body via PUT. "+
				"Workflow: (1) get_badi_implementation to read current state, "+
				"(2) lock_object on /sap/bc/adt/enhancements/enhoxh/<name>, "+
				"(3) modify the raw_xml from the GET response, "+
				"(4) call this tool with the modified XML. "+
				"Note: creating new enhancement implementations requires SAP GUI (transaction SE19).",
		),
		mcp.WithString("implementation_name", mcp.Required(), mcp.Description("Enhancement implementation name")),
		mcp.WithString("xml_body", mcp.Required(), mcp.Description("Full XML body (modified from get_badi_implementation raw_xml)")),
		mcp.WithString("lock_handle", mcp.Required(), mcp.Description("Lock handle from lock_object")),
		mcp.WithString("transport", mcp.Description("Transport request number (required for non-local packages)")),
		mcp.WithString("etag", mcp.Required(), mcp.Description("ETag from get_badi_implementation")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("implementation_name", "")
		xmlBody := req.GetString("xml_body", "")
		lh := req.GetString("lock_handle", "")
		transport := req.GetString("transport", "")
		etag := req.GetString("etag", "")
		err := client.SetEnhancementImplementation(ctx, name, xmlBody, lh, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{"status": "ok"})
		return mcp.NewToolResultText(string(out)), nil
	})
}
