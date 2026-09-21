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

func registerObjectTools(s toolAdder, client adt.ObjectClient, fallback BlackMagicClient) {
	s.AddTool(mcp.NewTool("create_object",
		mcp.WithTitleAnnotation("Create ABAP Object"),
		mcp.WithReadOnlyHintAnnotation(false),
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

	s.AddTool(mcp.NewTool("create_package",
		mcp.WithTitleAnnotation("Create ABAP Package"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Create an ABAP package (DEVC). Use this instead of create_object, which does not support packages. "+
			"A name starting with '$' creates a local package: SAP files it under software component LOCAL and it needs no transport. "+
			"A transportable package needs software_component (e.g. HOME), transport_layer and a transport request. "+
			"Packages are always created top-level; creating one inside another (superPackage) is not supported. "+
			"Requires S/4HANA or a recent ABAP Platform version — older ECC systems have no ADT package endpoint, so create the package in SE80 or SE21 there."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Package name, e.g. $ZLOCAL_TEST for a local package or ZPACKAGE for a transportable one. Lower case is accepted; SAP stores the name upper-cased"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Short description of the package"),
		),
		mcp.WithString("responsible",
			mcp.Required(),
			mcp.Description("SAP user the package is filed under, e.g. the logged-on user. SAP rejects the creation with 400 ExceptionInvalidData when this is empty"),
		),
		mcp.WithString("software_component",
			mcp.Description("Software component, e.g. HOME for a transportable package. Leave empty for a local one: SAP files a '$' package under LOCAL whatever is sent"),
		),
		mcp.WithString("transport_layer",
			mcp.Description("Transport layer for a transportable package; leave empty for a local one"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number; required for a transportable package"),
		),
		mcp.WithOutputSchema[ObjectCreateResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, errRes := requireString(req, "name")
		if errRes != nil {
			return errRes, nil
		}
		desc, errRes := requireString(req, "description")
		if errRes != nil {
			return errRes, nil
		}
		responsible, errRes := requireString(req, "responsible")
		if errRes != nil {
			return errRes, nil
		}
		softwareComponent := req.GetString("software_component", "")
		transportLayer := req.GetString("transport_layer", "")
		transport := req.GetString("transport", "")

		if err := client.CreatePackage(ctx, name, desc, responsible, softwareComponent, transportLayer, transport); err != nil {
			return errorResult(err), nil
		}
		// adtler upper-cases the name before sending it, so report what SAP holds.
		return mcp.NewToolResultJSON(ObjectCreateResult{Name: strings.ToUpper(name), Created: true})
	})

	s.AddTool(mcp.NewTool("delete_object",
		mcp.WithTitleAnnotation("Delete ABAP Object"),
		mcp.WithReadOnlyHintAnnotation(false),
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
		if err := client.DeleteObject(ctx, uri, "", transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ObjectDeleteResult{URI: uri, Deleted: true})
	})
}
