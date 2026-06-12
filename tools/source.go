package tools

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

var errMissingIncludeParam = errors.New("required parameter 'include' is missing or empty — must be one of: testclasses, definitions, implementations, macros")

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
		// No WithOutputSchema: single/array paths differ in return shape
		// (SourceResult vs SourceMultiResult).
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			result, err := client.GetSource(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), single), result.ETag)
			return mcp.NewToolResultJSON(SourceResult{Source: result.Source, ETag: result.ETag})
		}

		results := make([]SourceMultiEntry, len(multi))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(multi))
		for i, uri := range multi {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				res, err := client.GetSource(ctx, uri)
				results[i] = SourceMultiEntry{ObjectURI: uri}
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

		return mcp.NewToolResultJSON(SourceMultiResult{
			Total:      len(multi),
			Succeeded:  succeeded,
			Failed:     failed,
			TotalLines: totalLines,
			Results:    results,
		})
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
		mcp.WithOutputSchema[SourceResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.GetClassDefinition(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(SourceResult{Source: result.Source, ETag: result.ETag})
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
		mcp.WithOutputSchema[IncludeSourceResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		include := req.GetString("include", "")
		if include == "" {
			return errorResult(errMissingIncludeParam), nil
		}
		result, err := client.GetIncludeSource(ctx, uri, include)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(IncludeSourceResult{
			Source:  result.Source,
			ETag:    result.ETag,
			Include: include,
		})
	})

	s.AddTool(mcp.NewTool("set_include_source",
		mcp.WithTitleAnnotation("Set Class Include Source"),
		mcp.WithReadOnlyHintAnnotation(false),
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
		mcp.WithOutputSchema[SetIncludeSourceResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		include := req.GetString("include", "")
		if include == "" {
			return errorResult(errMissingIncludeParam), nil
		}
		source := req.GetString("source", "")
		lh := req.GetString("lock_handle", "")
		transport := req.GetString("transport", "")
		etag := req.GetString("etag", "")
		newETag, err := client.SetIncludeSource(ctx, uri, include, source, lh, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(SetIncludeSourceResult{
			ETag:    newETag,
			Include: include,
		})
	})

	s.AddTool(mcp.NewTool("create_test_include",
		mcp.WithTitleAnnotation("Create Test-Classes Include"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Bootstrap the test-classes include (CCAU) for a class that has never had one. "+
				"set_include_source fails with 500 when the include has no inactive version; "+
				"call this tool first to create it, then write source with set_include_source. "+
				"The class must be locked first (use lock_object).",
		),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description("Class URI, e.g. /sap/bc/adt/oo/classes/ZCL_MY_CLASS")),
		mcp.WithString("lock_handle", mcp.Description("Lock handle from lock_object (optional, looked up from lock map)")),
		mcp.WithString("transport", mcp.Description("Transport request number (required for non-local packages)")),
		mcp.WithOutputSchema[CreateTestIncludeResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		lh := req.GetString("lock_handle", "")
		if lh == "" {
			if state, ok := lockMap.Get(adt.LockKey(selector.ActiveName(), uri)); ok {
				lh = state.LockHandle
			}
		}
		transport := req.GetString("transport", "")
		if err := client.CreateTestInclude(ctx, uri, lh, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(CreateTestIncludeResult{ClassURI: uri, Created: true})
	})
}
