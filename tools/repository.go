package tools

import (
	"context"
	"encoding/json"
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
		mcp.WithDescription("List all ABAP objects in a package (flat, one level deep)."),
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
		mcp.WithDescription("Get metadata for an ABAP repository object: name, type, package, and description."),
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

	s.AddTool(mcp.NewTool("object_exists",
		mcp.WithTitleAnnotation("Check Object Exists"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Check whether an ABAP object exists. Returns true/false with basic metadata if found. "+
				"Use this to verify object names before reading source or navigating, avoiding hallucinated references.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		info, err := client.GetObjectInfo(ctx, uri)
		if err != nil {
			out, _ := json.Marshal(map[string]any{"exists": false, "object_uri": uri})
			return mcp.NewToolResultText(string(out)), nil
		}
		out, _ := json.Marshal(map[string]any{
			"exists":      true,
			"object_uri":  uri,
			"name":        info.Name,
			"type":        info.Type,
			"description": info.Description,
		})
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
					results[idx] = objectInfoResult{ObjectURI: u, Error: err.Error()}
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

	s.AddTool(mcp.NewTool("batch_object_exists",
		mcp.WithTitleAnnotation("Batch Check Objects Exist"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Check existence of multiple ABAP objects concurrently. "+
				"Returns true/false per object. Use this to validate a list of object references in bulk.",
		),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to check")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice(paramObjectURI+"s", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}

		type existsResult struct {
			ObjectURI string `json:"object_uri"`
			Exists    bool   `json:"exists"`
			Name      string `json:"name,omitempty"`
			Type      string `json:"type,omitempty"`
		}

		results := make([]existsResult, len(uris))
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
					results[idx] = existsResult{ObjectURI: u, Exists: false}
				} else {
					results[idx] = existsResult{ObjectURI: u, Exists: true, Name: info.Name, Type: info.Type}
				}
			}(i, uri)
		}
		wg.Wait()

		found := 0
		for _, r := range results {
			if r.Exists {
				found++
			}
		}

		out, _ := json.Marshal(map[string]any{
			"total":   len(uris),
			"found":   found,
			"missing": len(uris) - found,
			"results": results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
