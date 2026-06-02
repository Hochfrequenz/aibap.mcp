package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// customizingEntryItemsSchema describes one entry in the update_customizing
// `entries` array. It mirrors the runtime CustomizingEntry struct in
// blackmagic.go: a `keys` map identifying the row, an optional `values`
// map of fields to set, and an optional `op` selecting the mutation kind.
// additionalProperties is closed so clients cannot smuggle unrecognised
// top-level fields onto an entry.
//
// `values` is required at runtime when `op` is "upsert" (or omitted) and
// forbidden when `op` is "delete" — that pairing is enforced in Go code
// at request time rather than via JSON Schema, so the validation error
// can name the offending entry index.
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
			"description":          "Field-to-value map of columns to set on the row. Required when op is \"upsert\" (or omitted). Forbidden when op is \"delete\".",
			"additionalProperties": map[string]any{"type": "string"},
		},
		"op": map[string]any{
			"type":        "string",
			"enum":        []any{"upsert", "delete"},
			"default":     "upsert",
			"description": "Mutation kind. \"upsert\" (default) inserts the row if missing or updates it if a row matches Keys. \"delete\" removes the row matching Keys.",
		},
	},
	"required":             []any{"keys"},
	"additionalProperties": false,
}

func registerCustomizingWriteTools(s toolAdder, fallback BlackMagicClient, elicitor Elicitor) {
	s.AddTool(mcp.NewTool("update_customizing",
		mcp.WithTitleAnnotation("Update Customizing"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write entries to a customizing table (SM30/SM34). "+
				"This operation is NOT available via the ADT REST API — SAP does not expose "+
				"customizing writes through ADT. Requires a BlackMagic fallback (SAP GUI automation) "+
				"to be configured. Without it, this tool will return an error with guidance "+
				"to use SAP GUI (SM30) directly. "+
				"Each entry has an optional \"op\" field: \"upsert\" (default) inserts or updates; \"delete\" removes the row matching keys.",
		),
		mcp.WithString("table", mcp.Required(), mcp.Description("Customizing table or view name (e.g. V_T077D, T001W)")),
		mcp.WithArray("entries", mcp.Required(),
			mcp.Description("Entries to write. Each entry: {\"keys\": {\"FIELD1\": \"VAL1\"}, \"values\": {\"FIELD2\": \"VAL2\"}, \"op\": \"upsert\"|\"delete\"}. Keys identify the row, values are the fields to set, op selects the mutation kind."),
			mcp.Items(customizingEntryItemsSchema),
		),
		mcp.WithString("transport", mcp.Description("Transport request number for recording the change")),
		mcp.WithOutputSchema[UpdateCustomizingResult](),
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

		proceed, reason := ConfirmDestructive(ctx, elicitor,
			fmt.Sprintf("Confirm update to customizing table %s (%d entries). Customizing changes are difficult to reverse without an explicit before-image.", table, len(entries)))
		if !proceed {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "update_customizing aborted: " + reason}), nil
		}

		// Validate per-entry op semantics: upsert requires values; delete forbids them.
		for i, e := range entries {
			if len(e.Keys) == 0 {
				return errorResult(fmt.Errorf("entry %d: keys must be non-empty", i)), nil
			}
			op := e.Op
			if op == "" {
				op = "upsert"
			}
			switch op {
			case "upsert":
				if len(e.Values) == 0 {
					return errorResult(fmt.Errorf("entry %d: op \"upsert\" requires non-empty values", i)), nil
				}
			case "delete":
				if len(e.Values) > 0 {
					return errorResult(fmt.Errorf("entry %d: op \"delete\" must have empty values", i)), nil
				}
			default:
				return errorResult(fmt.Errorf("entry %d: invalid op %q (must be \"upsert\" or \"delete\")", i, e.Op)), nil
			}
		}

		if err := fallback.UpdateCustomizing(ctx, table, entries, transport); err != nil {
			return errorResult(err), nil
		}

		return mcp.NewToolResultJSON(UpdateCustomizingResult{Status: "updated", Table: table})
	})
}
