package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// patchOpItemsSchema describes the per-operation shape of an entry in the
// patch_source `operations` array. It is a discriminated union over `type`
// with one branch per operation kind. Each branch lists only its own fields,
// so MCP clients can validate payloads against the correct shape and the
// model can use the schema for autocomplete instead of relying on the
// description string.
//
// The branches mirror the runtime fields read by adt.ApplyPatchOps (in the
// adtler library). Keep them in sync when adding new operation kinds.
var patchOpItemsSchema = map[string]any{
	"oneOf": []any{
		// insert: insert `content` after line `after_line` (0 = before first line).
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":       map[string]any{"type": "string", "enum": []any{"insert"}},
				"after_line": map[string]any{"type": "integer", "description": "Line number after which to insert (must be >= 0). 0 inserts before the first line."},
				"content":    map[string]any{"type": "string", "description": "Source line(s) to insert."},
			},
			"required":             []any{"type", "after_line", "content"},
			"additionalProperties": false,
		},
		// replace: replace lines from_line..to_line (1-based, inclusive) with `content`.
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":      map[string]any{"type": "string", "enum": []any{"replace"}},
				"from_line": map[string]any{"type": "integer", "description": "First line to replace (1-based, inclusive; must be >= 1)."},
				"to_line":   map[string]any{"type": "integer", "description": "Last line to replace (1-based, inclusive; must be >= from_line)."},
				"content":   map[string]any{"type": "string", "description": "Replacement source."},
			},
			"required":             []any{"type", "from_line", "to_line", "content"},
			"additionalProperties": false,
		},
		// delete: delete lines from_line..to_line (1-based, inclusive).
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":      map[string]any{"type": "string", "enum": []any{"delete"}},
				"from_line": map[string]any{"type": "integer", "description": "First line to delete (1-based, inclusive; must be >= 1)."},
				"to_line":   map[string]any{"type": "integer", "description": "Last line to delete (1-based, inclusive; must be >= from_line)."},
			},
			"required":             []any{"type", "from_line", "to_line"},
			"additionalProperties": false,
		},
		// search_replace: textual substitution. `all` defaults to false (first match only).
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":    map[string]any{"type": "string", "enum": []any{"search_replace"}},
				"search":  map[string]any{"type": "string", "description": "Literal substring to find."},
				"replace": map[string]any{"type": "string", "description": "Replacement substring."},
				"all":     map[string]any{"type": "boolean", "description": "If true, replace all occurrences; otherwise only the first match."},
			},
			"required":             []any{"type", "search", "replace"},
			"additionalProperties": false,
		},
	},
}

// registerPatchTools registers the patch_source MCP tool on the server.
func registerPatchTools(s toolAdder, client interface {
	adt.SourceClient
	adt.LockClient
}, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("patch_source",
		mcp.WithTitleAnnotation("Patch Source Code"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Apply patch operations to ABAP source code. Supports line-based (insert/replace/delete) and text-based (search_replace) operations. Automatically acquires a lock if none exists."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
		mcp.WithArray("operations",
			mcp.Required(),
			mcp.Description(`Array of patch operations. Each operation is one of: insert, replace, delete, search_replace. The "type" field discriminates the variant; other fields depend on the type.`),
			mcp.Items(patchOpItemsSchema),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (optional)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle (optional; looked up from lock map, or auto-acquired)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")

		// Parse operations from JSON array.
		args := req.GetArguments()
		rawOps, ok := args["operations"]
		if !ok {
			return errorResult(fmt.Errorf("missing required parameter: operations")), nil
		}
		opsJSON, err := json.Marshal(rawOps)
		if err != nil {
			return errorResult(fmt.Errorf("marshal operations: %w", err)), nil
		}
		var ops []adt.PatchOp
		if err := json.Unmarshal(opsJSON, &ops); err != nil {
			return errorResult(fmt.Errorf("parse operations: %w", err)), nil
		}

		// Resolve lock handle: explicit param > lock map > auto-lock.
		key := adt.LockKey(selector.ActiveName(), uri)
		preExisting := explicitHandle != ""
		if !preExisting {
			if state, ok := lockMap.Get(key); ok && state.LockHandle != "" {
				preExisting = true
			}
		}
		lockHandle, err := lockMap.ResolveLock(ctx, client, key, uri, explicitHandle)
		if err != nil {
			return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
		}
		autoLocked := !preExisting

		// Get current source.
		srcResult, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		etag := srcResult.ETag

		// Apply patch operations.
		oldSource := srcResult.Source
		newSource, err := adt.ApplyPatchOps(oldSource, ops)
		if err != nil {
			return errorResult(fmt.Errorf("patch failed: %w", err)), nil
		}

		// Write patched source back.
		newETag, err := client.SetSource(ctx, uri, newSource, lockHandle, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}

		// Update lock map ETag.
		lockMap.UpdateETag(key, newETag)

		delta := adt.LineDelta(oldSource, newSource)

		out, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"line_delta":  delta,
			"locked":      autoLocked,
			"lock_handle": lockHandle,
			"etag":        newETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
