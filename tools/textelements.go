package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerTextElementTools(s toolAdder, client adt.DocuClient, lockClient adt.LockClient, lockMap *adt.LockMap, selector SystemSelector) {
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
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uri is required"}), nil
		}
		result, err := client.GetTextElements(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("set_text_elements",
		mcp.WithTitleAnnotation("Set Text Elements"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write text symbols and/or selection texts for an ABAP program, class, or function group. "+
				"Provide symbols (TEXT-001 etc.) and/or selections (parameter labels). "+
				"At least one of symbols or selections must be provided. "+
				"The object is auto-locked if no lock_handle is given. "+
				"Only works on S4 systems — ECC does not support this endpoint.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithArray("symbols",
			mcp.Description("Text symbols to write. Each entry: {\"key\": \"001\", \"text\": \"My text\", \"max_length\": 50}. max_length is optional."),
		),
		mcp.WithArray("selections",
			mcp.Description("Selection texts to write. Each entry: {\"name\": \"P_PARAM\", \"text\": \"Label text\"}."),
		),
		mcp.WithString("transport", mcp.Description("Transport request number for recording the change")),
		mcp.WithString("lock_handle", mcp.Description("Lock handle from lock_object. If omitted, the object is locked automatically.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")

		args := req.GetArguments()

		// Parse symbols
		var symbols []adt.TextSymbol
		if rawSymbols, ok := args["symbols"]; ok && rawSymbols != nil {
			data, _ := json.Marshal(rawSymbols)
			if err := json.Unmarshal(data, &symbols); err != nil {
				return errorResult(fmt.Errorf("invalid symbols format: %w", err)), nil
			}
		}

		// Parse selections
		var selections []adt.SelectionText
		if rawSelections, ok := args["selections"]; ok && rawSelections != nil {
			data, _ := json.Marshal(rawSelections)
			if err := json.Unmarshal(data, &selections); err != nil {
				return errorResult(fmt.Errorf("invalid selections format: %w", err)), nil
			}
		}

		if symbols == nil && selections == nil {
			return errorResult(fmt.Errorf("at least one of symbols or selections must be provided")), nil
		}

		// Auto-lock if no explicit handle
		key := adt.LockKey(selector.ActiveName(), uri)
		lockHandle, err := lockMap.ResolveLock(ctx, lockClient, key, uri, explicitHandle)
		if err != nil {
			return errorResult(err), nil
		}

		if err := client.SetTextElements(ctx, uri, symbols, selections, lockHandle, transport); err != nil {
			return errorResult(err), nil
		}

		return mcp.NewToolResultText("Text elements updated successfully"), nil
	})
}
