package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// customizingEntryItemsSchema describes one entry in the update_customizing
// `entries` array. It mirrors the runtime CustomizingEntry struct in
// blackmagic.go: a `keys` map identifying the row and a `values` map of
// fields to set, both string-to-string. additionalProperties is closed so
// clients cannot smuggle unrecognised top-level fields onto an entry.
var customizingEntryItemsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"keys": map[string]any{
			"type":                 "object",
			"description":          "Field-to-value map identifying the row to update (primary-key columns).",
			"additionalProperties": map[string]any{"type": "string"},
		},
		"values": map[string]any{
			"type":                 "object",
			"description":          "Field-to-value map of columns to set on the identified row.",
			"additionalProperties": map[string]any{"type": "string"},
		},
	},
	"required":             []any{"keys", "values"},
	"additionalProperties": false,
}

func registerCustomizingWriteTools(s toolAdder, fallback BlackMagicClient) {
	s.AddTool(mcp.NewTool("update_customizing",
		mcp.WithTitleAnnotation("Update Customizing"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write entries to a customizing table (SM30/SM34). "+
				"This operation is NOT available via the ADT REST API — SAP does not expose "+
				"customizing writes through ADT. Requires a BlackMagic fallback (SAP GUI automation) "+
				"to be configured. Without it, this tool will return an error with guidance "+
				"to use SAP GUI (SM30) directly.",
		),
		mcp.WithString("table", mcp.Required(), mcp.Description("Customizing table or view name (e.g. V_T077D, T001W)")),
		mcp.WithArray("entries", mcp.Required(),
			mcp.Description("Entries to write. Each entry: {\"keys\": {\"FIELD1\": \"VAL1\"}, \"values\": {\"FIELD2\": \"VAL2\"}}. Keys identify the row, values are the fields to set."),
			mcp.Items(customizingEntryItemsSchema),
		),
		mcp.WithString("transport", mcp.Description("Transport request number for recording the change")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := req.GetString("table", "")
		transport := req.GetString("transport", "")

		// Parse entries
		var entries []CustomizingEntry
		if rawEntries, ok := req.GetArguments()["entries"]; ok && rawEntries != nil {
			data, _ := json.Marshal(rawEntries)
			if err := json.Unmarshal(data, &entries); err != nil {
				return errorResult(fmt.Errorf("invalid entries format: %w", err)), nil
			}
		}
		if len(entries) == 0 {
			return errorResult(fmt.Errorf("at least one entry must be provided")), nil
		}

		if fallback == nil {
			return errorResult(fmt.Errorf(
				"customizing writes are not available via the ADT REST API — "+
					"SAP does not expose SM30/SM34 functionality through REST endpoints. "+
					"Configure a BlackMagic fallback (SAP GUI automation) or make the change "+
					"manually in SAP GUI (SM30 → table %s)", table)), nil
		}

		// The transport parameter is not part of the BlackMagic interface yet;
		// the implementation is responsible for handling transport prompts.
		_ = transport

		if err := fallback.UpdateCustomizing(ctx, table, entries); err != nil {
			return errorResult(err), nil
		}

		out, _ := json.Marshal(map[string]string{
			"status": "updated",
			"table":  table,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
