package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerObjectTools(s toolAdder, client adt.ObjectClient) {
	s.AddTool(mcp.NewTool("create_object",
		mcp.WithTitleAnnotation("Create ABAP Object"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Create a new ABAP object. Supported types: PROG (program), CLAS (class), INTF (interface), FUGR (function group), MSAG (message class), DDLS (CDS view, S4 only), TABL (table, S4 only), DTEL (data element, S4 only), DOMA (domain, S4 only). DDLS/TABL are created empty — use set_source_from_file with CDS DDL syntax to define them, then activate."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG, CLAS, INTF, FUGR, MSAG, DDLS (S4 only), TABL (S4 only), DTEL (S4 only), DOMA (S4 only)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Object name, e.g. ZREPORT or ZCL_MY_CLASS"),
		),
		mcp.WithString("package",
			mcp.Required(),
			mcp.Description("Package to create the object in, e.g. ZPACKAGE or $TMP for local"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Short description of the object"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objectType := req.GetString("object_type", "")
		name := req.GetString("name", "")
		pkg := req.GetString("package", "")
		desc := req.GetString("description", "")
		transport := req.GetString("transport", "")
		if err := client.CreateObject(ctx, objectType, name, pkg, desc, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object created: " + name), nil
	})

	s.AddTool(mcp.NewTool("delete_object",
		mcp.WithTitleAnnotation("Delete ABAP Object"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Delete an ABAP object from the SAP system. Uses optimistic locking (ETag) internally. "+
				"For objects in non-local packages, pass the transport number that locks the object. "+
				"If the object is locked in another user's transport, use that transport's number directly — "+
				"SAP will automatically record the deletion under your user. "+
				"Use get_transport_requests to find the locking transport if needed.",
		),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		if err := client.DeleteObject(ctx, uri, "", transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object deleted"), nil
	})
}
