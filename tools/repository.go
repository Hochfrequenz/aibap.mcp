package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerRepositoryTools(s toolAdder, client adt.SearchClient) {
	s.AddTool(mcp.NewTool("browse_package",
		mcp.WithTitleAnnotation("Browse Package Contents"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("List all ABAP objects in a package."),
		mcp.WithString("package_name", mcp.Required(), mcp.Description("Package name, e.g. ZPACKAGE")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg := req.GetString("package_name", "")
		results, err := client.BrowsePackage(ctx, pkg)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_object_info",
		mcp.WithTitleAnnotation("Get Object Info"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Get metadata for an ABAP repository object."),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		info, err := client.GetObjectInfo(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(info)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("batch_get_object_info",
		mcp.WithTitleAnnotation("Batch Get Object Info"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Get metadata for multiple ABAP repository objects concurrently. "+
				"Wraps get_object_info with parallel execution (up to 10 concurrent requests). "+
				"Returns per-object results with metadata and errors. "+
				"Use this instead of calling get_object_info in a loop to reduce round-trips.",
		),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to retrieve metadata for")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice(paramObjectURI+"s", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}

		type objectInfoResult struct {
			ObjectURI string          `json:"object_uri"`
			Info      *adt.ObjectInfo `json:"info,omitempty"`
			Error     string          `json:"error,omitempty"`
		}

		results := make([]objectInfoResult, len(uris))
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup
		wg.Add(len(uris))
		for i, uri := range uris {
			go func(idx int, u string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				info, err := client.GetObjectInfo(ctx, u)
				if err != nil {
					results[idx] = objectInfoResult{ObjectURI: u, Error: fmt.Sprintf("%v", err)}
				} else {
					results[idx] = objectInfoResult{ObjectURI: u, Info: info}
				}
			}(i, uri)
		}
		wg.Wait()

		succeeded, failed := 0, 0
		for _, r := range results {
			if r.Error != "" {
				failed++
			} else {
				succeeded++
			}
		}

		out, _ := json.Marshal(map[string]any{
			"total":     len(uris),
			"succeeded": succeeded,
			"failed":    failed,
			"results":   results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
