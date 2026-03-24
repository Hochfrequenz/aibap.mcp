package tools

import (
	"context"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerObjectTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("create_object",
		mcp.WithDescription("Create a new ABAP object (program, class, or interface)."),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("Object type: PROG (program), CLAS (class), or INTF (interface)"),
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
		mcp.WithDescription("Delete an ABAP object from the SAP system. The object is automatically locked before deletion."),
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

		lockHandle, err := client.LockObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}

		if err := client.DeleteObject(ctx, uri, lockHandle, transport); err != nil {
			// Best-effort unlock on delete failure.
			_ = client.UnlockObject(ctx, uri, lockHandle)
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object deleted"), nil
	})
}
