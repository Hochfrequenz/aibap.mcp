package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// registerPatchTools registers the patch_source MCP tool on the server.
func registerPatchTools(s toolAdder, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("patch_source",
		mcp.WithDescription("Apply patch operations to ABAP source code. Supports line-based (insert/replace/delete) and text-based (search_replace) operations. Automatically acquires a lock if none exists."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
		mcp.WithArray("operations",
			mcp.Required(),
			mcp.Description(`Array of patch operations. Each has "type" field (insert/replace/delete/search_replace) plus op-specific fields.`),
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
