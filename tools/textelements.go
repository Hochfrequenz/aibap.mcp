package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// textElementsResourceURI maps a program/class/function-group object URI to
// the corresponding /sap/bc/adt/textelements/... URI. SAP binds the
// enqueue lock for text-element writes to this resource, not to the
// underlying program — the caller-visible object URI alone is not enough
// to acquire a usable lock.
func textElementsResourceURI(objectURI string) (string, error) {
	switch {
	case strings.HasPrefix(objectURI, "/sap/bc/adt/programs/programs/"):
		return strings.Replace(objectURI, "/sap/bc/adt/programs/programs/", "/sap/bc/adt/textelements/programs/", 1), nil
	case strings.HasPrefix(objectURI, "/sap/bc/adt/oo/classes/"):
		return strings.Replace(objectURI, "/sap/bc/adt/oo/classes/", "/sap/bc/adt/textelements/classes/", 1), nil
	case strings.HasPrefix(objectURI, "/sap/bc/adt/functions/groups/"):
		return strings.Replace(objectURI, "/sap/bc/adt/functions/groups/", "/sap/bc/adt/textelements/functiongroups/", 1), nil
	default:
		return "", fmt.Errorf("text elements not supported for %q (only programs, classes, function groups)", objectURI)
	}
}

func registerTextElementTools(s toolAdder, client interface {
	adt.DocuClient
	adt.LockClient
}, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("get_text_elements",
		mcp.WithTitleAnnotation("Get Text Elements"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read text symbols and selection texts of an ABAP program, class, or function group. "+
				"Text symbols are referenced as TEXT-001, TEXT-002 etc. in ABAP source. "+
				"Selection texts are the labels for PARAMETERS and SELECT-OPTIONS on the selection screen. "+
				"Not available on all systems — depends on the SAP Basis version.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithOutputSchema[adt.TextElements](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		result, err := client.GetTextElements(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})

	s.AddTool(mcp.NewTool("set_text_elements",
		mcp.WithTitleAnnotation("Set Text Elements"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write text symbols and/or selection texts for an ABAP program, class, or function group. "+
				"At least one of symbols or selections must be provided. "+
				"The text-element resource is auto-locked unless lock_handle is supplied. "+
				"S/4 only — ECC does not expose this endpoint.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithString("symbols",
			mcp.Description(`JSON array of text symbols, e.g. [{"key":"001","text":"My text","max_length":50}]. max_length is optional.`),
		),
		mcp.WithString("selections",
			mcp.Description(`JSON array of selection texts, e.g. [{"name":"P_PARAM","text":"Label text"}].`),
		),
		mcp.WithString("transport", mcp.Description("Transport request number (required for non-local packages on S/4)")),
		mcp.WithString("lock_handle", mcp.Description("Explicit lock handle on the textelements resource (optional, looked up from lock map otherwise)")),
		mcp.WithOutputSchema[SetTextElementsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")
		symbolsJSON := req.GetString("symbols", "")
		selectionsJSON := req.GetString("selections", "")

		var symbols []adt.TextSymbol
		if symbolsJSON != "" {
			if err := json.Unmarshal([]byte(symbolsJSON), &symbols); err != nil {
				return errorResult(&adt.ADTError{StatusCode: 400, Message: "invalid symbols JSON: " + err.Error()}), nil
			}
		}
		var selections []adt.SelectionText
		if selectionsJSON != "" {
			if err := json.Unmarshal([]byte(selectionsJSON), &selections); err != nil {
				return errorResult(&adt.ADTError{StatusCode: 400, Message: "invalid selections JSON: " + err.Error()}), nil
			}
		}
		if len(symbols) == 0 && len(selections) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "at least one of symbols or selections must be provided"}), nil
		}

		// Lock the textelements resource (separate enqueue from the program lock).
		lockURI, err := textElementsResourceURI(uri)
		if err != nil {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: err.Error()}), nil
		}
		key := adt.LockKey(selector.ActiveName(), lockURI)
		lockHandle, err := lockMap.ResolveLock(ctx, client, key, lockURI, explicitHandle)
		if err != nil {
			return errorResult(fmt.Errorf("lock textelements resource: %w", err)), nil
		}

		if err := client.SetTextElements(ctx, uri, symbols, selections, lockHandle, transport); err != nil {
			return errorResult(err), nil
		}

		return mcp.NewToolResultJSON(SetTextElementsResult{
			Success:         true,
			ObjectURI:       uri,
			SymbolsCount:    len(symbols),
			SelectionsCount: len(selections),
			LockHandle:      lockHandle,
		})
	})
}
