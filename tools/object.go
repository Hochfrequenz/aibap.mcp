package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// ddicTypes are object types that require S4 — on ECC, ADT returns 404 and
// we fall back to BlackMagic (SAP GUI automation via SE11).
var ddicTypes = map[string]bool{
	"TABL": true,
	"DTEL": true,
	"DOMA": true,
}

func registerObjectTools(s toolAdder, client adt.ObjectClient, fallback BlackMagicClient, elicitor Elicitor) {
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
		mcp.WithOutputSchema[ObjectCreateResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objectType := strings.ToUpper(req.GetString("object_type", ""))
		name := req.GetString("name", "")
		pkg := req.GetString("package", "")
		desc := req.GetString("description", "")
		transport := req.GetString("transport", "")

		err := client.CreateObject(ctx, objectType, name, pkg, desc, transport)
		if err != nil {
			// DDIC types (TABL, DTEL, DOMA) may not be available via ADT on ECC.
			// Fall back to BlackMagic (SAP GUI automation via SE11) if configured.
			if ddicTypes[objectType] && strings.Contains(err.Error(), "404") {
				if fallback != nil {
					if fbErr := fallback.CreateObjectFallback(ctx, objectType, name, pkg, desc, transport); fbErr != nil {
						return errorResult(fbErr), nil
					}
					return mcp.NewToolResultJSON(ObjectCreateResult{Name: name, Created: true})
				}
				return errorResult(fmt.Errorf(
					"DDIC object creation (%s) is not available via ADT on this system — "+
						"this endpoint is S4-only. Configure a BlackMagic fallback (SAP GUI automation) "+
						"or create the object manually in SAP GUI (SE11)", objectType)), nil
			}
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ObjectCreateResult{Name: name, Created: true})
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
		mcp.WithOutputSchema[ObjectDeleteResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		proceed, reason := ConfirmDestructive(ctx, elicitor,
			fmt.Sprintf("Confirm deletion of %s. This is irreversible.", uri))
		if !proceed {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "delete_object aborted: " + reason}), nil
		}
		if err := client.DeleteObject(ctx, uri, "", transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ObjectDeleteResult{URI: uri, Deleted: true})
	})
}
