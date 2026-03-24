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

func registerFileSourceTools(s toolAdder, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
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
		key := lockKey(selector, uri)

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			return errorResult(fmt.Errorf("reading file: %w", err)), nil
		}
		source := string(data)

		// Resolve lock handle
		lockHandle := explicitHandle
		if lockHandle == "" {
			if state, ok := lockMap.Get(key); ok {
				lockHandle = state.LockHandle
			}
		}

		// Auto-lock if needed
		if lockHandle == "" {
			handle, err := client.LockObject(ctx, uri)
			if err != nil {
				return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
			}
			lockHandle = handle
			lockMap.Set(key, handle, "")
		}

		// Get ETag if not in lock map
		state, _ := lockMap.Get(key)
		etag := state.ETag
		if etag == "" {
			sourceResult, err := client.GetSource(ctx, uri)
			if err != nil {
				return errorResult(err), nil
			}
			etag = sourceResult.ETag
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
