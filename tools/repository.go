package tools

import (
	"context"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
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
		mcp.WithOutputSchema[BrowsePackageResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg := req.GetString("package_name", "")
		results, err := client.BrowsePackage(ctx, pkg)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(BrowsePackageResult{Count: len(results), Objects: results})
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
		// No WithOutputSchema: single/array paths differ in return shape.
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			info, err := client.GetObjectInfo(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(info)
		}

		results := make([]ObjectInfoBatchEntry, len(multi))
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
					results[idx] = ObjectInfoBatchEntry{ObjectURI: u, Error: err.Error()}
				} else {
					results[idx] = ObjectInfoBatchEntry{ObjectURI: u, Info: info}
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

		return mcp.NewToolResultJSON(ObjectInfoBatchResult{
			Total:     len(multi),
			Succeeded: succeeded,
			Failed:    failed,
			Results:   results,
		})
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
		// No WithOutputSchema: single/array paths differ in return shape.
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			info, err := client.GetObjectInfo(ctx, single)
			if err != nil {
				return mcp.NewToolResultJSON(ObjectExistsResult{ObjectURI: single, Exists: false})
			}
			return mcp.NewToolResultJSON(ObjectExistsResult{
				Exists:      true,
				ObjectURI:   single,
				Name:        info.Name,
				Type:        info.Type,
				Description: info.Description,
			})
		}

		results := make([]ObjectExistsBatchEntry, len(multi))
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
					results[idx] = ObjectExistsBatchEntry{ObjectURI: u, Exists: false}
				} else {
					results[idx] = ObjectExistsBatchEntry{ObjectURI: u, Exists: true, Name: info.Name, Type: info.Type}
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

		return mcp.NewToolResultJSON(ObjectExistsBatchResult{
			Total:   len(multi),
			Found:   found,
			Missing: len(multi) - found,
			Results: results,
		})
	})
}
