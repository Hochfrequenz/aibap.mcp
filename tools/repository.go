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
		mcp.WithDescription(
			"Get metadata for one or more ABAP repository objects: name, type, package, and description. "+
				"Pass a single URI string for one object, or an array of URIs for batch lookup (up to 10 concurrent requests). "+
				"Batch mode returns {total, succeeded, failed, results:[{object_uri, info, error}]}.",
		),
		withStringOrArray(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			info, err := client.GetObjectInfo(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			out, _ := json.Marshal(info)
			return mcp.NewToolResultText(string(out)), nil
		}

		type objectInfoResult struct {
			ObjectURI string          `json:"object_uri"`
			Info      *adt.ObjectInfo `json:"info,omitempty"`
			Error     string          `json:"error,omitempty"`
		}

		results := make([]objectInfoResult, len(multi))
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup
		wg.Add(len(multi))
		for i, uri := range multi {
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
			"total":     len(multi),
			"succeeded": succeeded,
			"failed":    failed,
			"results":   results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("object_exists",
		mcp.WithTitleAnnotation("Check Object Exists"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Check whether one or more ABAP objects exist. "+
				"Pass a single URI string to get {exists, object_uri, name, type, description}. "+
				"Pass an array of URIs for batch mode (up to 10 concurrent): returns {total, found, missing, results:[{object_uri, exists, name, type}]}. "+
				"Use this to verify object names before reading source or navigating, avoiding hallucinated references.",
		),
		withStringOrArray(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			info, err := client.GetObjectInfo(ctx, single)
			if err != nil {
				out, _ := json.Marshal(map[string]any{"exists": false, "object_uri": single})
				return mcp.NewToolResultText(string(out)), nil
			}
			out, _ := json.Marshal(map[string]any{
				"exists":      true,
				"object_uri":  single,
				"name":        info.Name,
				"type":        info.Type,
				"description": info.Description,
			})
			return mcp.NewToolResultText(string(out)), nil
		}

		type existsResult struct {
			ObjectURI string `json:"object_uri"`
			Exists    bool   `json:"exists"`
			Name      string `json:"name,omitempty"`
			Type      string `json:"type,omitempty"`
		}

		results := make([]existsResult, len(multi))
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup
		wg.Add(len(multi))
		for i, uri := range multi {
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
			"total":   len(multi),
			"found":   found,
			"missing": len(multi) - found,
			"results": results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
