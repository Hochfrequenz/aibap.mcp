package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSourceTools(s toolAdder, client adt.SourceClient, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("get_source",
		mcp.WithTitleAnnotation("Get ABAP Source Code"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read ABAP source code from SAP. Returns source text and ETag for optimistic locking."),
		withStringOrArray(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			result, err := client.GetSource(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), single), result.ETag)
			out, _ := json.Marshal(map[string]string{
				"source": result.Source,
				"etag":   result.ETag,
			})
			return mcp.NewToolResultText(string(out)), nil
		}

		type sourceResult struct {
			ObjectURI string `json:"object_uri"`
			Source    string `json:"source,omitempty"`
			ETag      string `json:"etag,omitempty"`
			Error     string `json:"error,omitempty"`
		}

		results := make([]sourceResult, len(multi))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(multi))
		for i, uri := range multi {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				res, err := client.GetSource(ctx, uri)
				results[i] = sourceResult{ObjectURI: uri}
				if err != nil {
					results[i].Error = err.Error()
				} else {
					results[i].Source = res.Source
					results[i].ETag = res.ETag
					lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), uri), res.ETag)
				}
			}(i, uri)
		}
		wg.Wait()

		succeeded, failed, totalLines := 0, 0, 0
		for _, r := range results {
			if r.Error != "" {
				failed++
				continue
			}
			succeeded++
			totalLines += strings.Count(r.Source, "\n")
		}

		out, _ := json.Marshal(map[string]any{
			"total":       len(multi),
			"succeeded":   succeeded,
			"failed":      failed,
			"total_lines": totalLines,
			"results":     results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_class_definition",
		mcp.WithTitleAnnotation("Get Class Definition Only"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read only the definition part of an ABAP class (everything up to the first ENDCLASS), "+
				"excluding method implementations. Use this instead of get_source when you only need "+
				"the class signature, inheritance hierarchy, interface implementations, or method signatures. "+
				"Saves ~95% tokens on large classes.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description("Class URI, e.g. /sap/bc/adt/oo/classes/ZCL_MY_CLASS")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.GetClassDefinition(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"source": result.Source,
			"etag":   result.ETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_include_source",
		mcp.WithTitleAnnotation("Get Class Include Source"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Read source code of a class include (local definitions, implementations, test classes, macros). "+
				"Use this to read or inspect local test classes, helper classes, or type definitions that live "+
				"in separate includes rather than in the main class source.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description("Class URI, e.g. /sap/bc/adt/oo/classes/ZCL_MY_CLASS")),
		mcp.WithString("include", mcp.Required(), mcp.Description("Include name: testclasses, definitions, implementations, or macros")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		include := req.GetString("include", "")
		result, err := client.GetIncludeSource(ctx, uri, include)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"source":  result.Source,
			"etag":    result.ETag,
			"include": include,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("set_include_source",
		mcp.WithTitleAnnotation("Set Class Include Source"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Write source code to a class include. Use this to set local test classes, helper classes, "+
				"type definitions, or macros. The object must be locked first (use lock_object). "+
				"Pass the ETag from get_include_source for optimistic locking.",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description("Class URI, e.g. /sap/bc/adt/oo/classes/ZCL_MY_CLASS")),
		mcp.WithString("include", mcp.Required(), mcp.Description("Include name: testclasses, definitions, implementations, or macros")),
		mcp.WithString("source", mcp.Required(), mcp.Description("ABAP source code to write")),
		mcp.WithString("lock_handle", mcp.Description("Lock handle from lock_object (optional, looked up from lock map)")),
		mcp.WithString("transport", mcp.Description("Transport request number (required for non-local packages)")),
		mcp.WithString("etag", mcp.Required(), mcp.Description("ETag from get_include_source for optimistic locking")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		include := req.GetString("include", "")
		source := req.GetString("source", "")
		lh := req.GetString("lock_handle", "")
		transport := req.GetString("transport", "")
		etag := req.GetString("etag", "")
		newETag, err := client.SetIncludeSource(ctx, uri, include, source, lh, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"etag":    newETag,
			"include": include,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}

// errorResult converts an error to an MCP error result with the SAP error message.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
	}
}
