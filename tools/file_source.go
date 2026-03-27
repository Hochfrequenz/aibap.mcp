package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerFileSourceTools(s toolAdder, client interface {
	adt.SourceClient
	adt.LockClient
}, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("set_source_from_file",
		mcp.WithDescription("Upload ABAP source code from a local file to SAP. Auto-locks if needed."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to source file (absolute or relative to working directory)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Explicit lock handle (optional, looked up from lock map)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		filePath := req.GetString("file_path", "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")
		key := adt.LockKey(selector.ActiveName(), uri)

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			return errorResult(fmt.Errorf("reading file: %w", err)), nil
		}
		source := string(data)

		// Resolve lock handle: explicit param > lock map > auto-lock.
		lockHandle, err := lockMap.ResolveLock(ctx, client, key, uri, explicitHandle)
		if err != nil {
			return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
		}

		// Get ETag if not in lock map.
		etag, err := lockMap.ResolveETag(ctx, client, key, uri)
		if err != nil {
			return errorResult(err), nil
		}

		// Write source
		newETag, err := client.SetSource(ctx, uri, source, lockHandle, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(key, newETag)

		lineCount := len(strings.Split(source, "\n"))
		out, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"lines":       lineCount,
			"locked":      true,
			"lock_handle": lockHandle,
			"etag":        newETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
