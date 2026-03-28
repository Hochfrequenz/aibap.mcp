package tools

import (
	"context"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerObjectTools(s toolAdder, client adt.ObjectClient) {
	s.AddTool(mcp.NewTool("create_object",
		mcp.WithDescription("Create a new ABAP object. Supported types: PROG (program), CLAS (class), INTF (interface), FUGR (function group), TABL (table, S4 only), DTEL (data element, S4 only), DOMA (domain, S4 only). For packages use create_package. Tables are created empty — use set_source_from_file with DDL syntax to add fields, then activate."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG, CLAS, INTF, FUGR, TABL (S4 only), DTEL (S4 only), DOMA (S4 only)"),
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
		mcp.WithDescription("Delete an ABAP object from the SAP system. Uses optimistic locking (ETag) internally."),
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
