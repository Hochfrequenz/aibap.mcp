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

// buildDeleteMessage forms the elicitation prompt for delete_object, enriched
// with TADIR metadata (type, package, author, creation date) when available.
// Falls back to the bare URI when GetObjectInfo returns an error or empty name.
func buildDeleteMessage(ctx context.Context, uri string, sc adt.SearchClient, qc adt.QueryClient) string {
	const irreversible = "\nThis is irreversible."
	if sc == nil || qc == nil {
		return "About to delete " + uri + "." + irreversible
	}

	info, err := sc.GetObjectInfo(ctx, uri)
	if err != nil || info == nil || info.Name == "" {
		return "About to delete " + uri + "." + irreversible
	}

	// ADT type includes suffix (e.g. "PROG/P") — strip it for display and TADIR lookup.
	objType := strings.SplitN(info.Type, "/", 2)[0]
	line := fmt.Sprintf("About to delete %s %s", objType, info.Name)
	if info.PackageName != "" {
		line += " (Package: " + info.PackageName + ")"
	}

	sql := fmt.Sprintf(
		"SELECT AUTHOR, CREATED_ON FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
		adt.EscapeValue(objType), adt.EscapeValue(info.Name),
	)
	if qr, err := qc.RunQuery(ctx, sql, 1); err == nil && qr != nil && len(qr.Rows) == 1 {
		row := qr.Rows[0]
		authorIdx, createdIdx := -1, -1
		for i, col := range qr.Columns {
			switch col.Name {
			case "AUTHOR":
				authorIdx = i
			case "CREATED_ON":
				createdIdx = i
			}
		}
		var meta []string
		if authorIdx >= 0 && authorIdx < len(row) && row[authorIdx] != "" {
			meta = append(meta, "Author: "+row[authorIdx])
		}
		if createdIdx >= 0 && createdIdx < len(row) && len(row[createdIdx]) == 8 {
			d := row[createdIdx]
			meta = append(meta, "Created: "+d[:4]+"-"+d[4:6]+"-"+d[6:])
		}
		if len(meta) > 0 {
			line += " | " + strings.Join(meta, " | ")
		}
	}

	return line + "." + irreversible
}

func registerObjectTools(s toolAdder, client adt.ObjectClient, sc adt.SearchClient, qc adt.QueryClient, fallback BlackMagicClient, elicitor Elicitor) {
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
		proceed, reason := ConfirmDestructive(ctx, elicitor, buildDeleteMessage(ctx, uri, sc, qc))
		if !proceed {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "delete_object aborted: " + reason}), nil
		}
		if err := client.DeleteObject(ctx, uri, "", transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(ObjectDeleteResult{URI: uri, Deleted: true})
	})
}
