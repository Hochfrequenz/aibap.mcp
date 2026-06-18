package tools

import (
	"context"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// searchDepsClient is the combined interface required by registerSearchTools:
// object discovery (search_objects, where_used) plus dependency resolution
// (get_object_dependencies).
type searchDepsClient interface {
	adt.SearchClient
	adt.DependencyClient
}

func registerSearchTools(s toolAdder, client searchDepsClient) {
	s.AddTool(mcp.NewTool("search_objects",
		mcp.WithTitleAnnotation("Search ABAP Objects"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Search for ABAP repository objects by name. Supports wildcards, e.g. ZREPORT*."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query, e.g. ZREPORT*")),
		mcp.WithString("object_type", mcp.Description("Filter by type, e.g. PROG/P for programs")),
		mcp.WithNumber("max_results", mcp.Description("Maximum number of results (default: 50)")),
		mcp.WithOutputSchema[SearchObjectsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		objType := req.GetString("object_type", "")
		maxResults := req.GetInt("max_results", 50)
		results, err := client.SearchObjects(ctx, query, objType, maxResults)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(SearchObjectsResult{Count: len(results), Results: results})
	})

	s.AddTool(mcp.NewTool("where_used",
		mcp.WithTitleAnnotation("Where-Used List"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Find all ABAP objects that use the given object(s). "+
				"Pass a single URI string for one lookup, or an array of URIs to run lookups concurrently (up to 10). "+
				"Batch mode returns {total, total_references, results:[{object_uri, references, error}]}.",
		),
		withStringOrArray(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		// No WithOutputSchema: single/array paths differ in return shape.
		// Both branches return an object so structuredContent stays spec-legal (#351).
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			results, err := client.WhereUsed(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(WhereUsedSingleResult{Count: len(results), References: results})
		}

		results := make([]WhereUsedBatchEntry, len(multi))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 10)
		wg.Add(len(multi))
		for i, uri := range multi {
			go func(i int, uri string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				refs, err := client.WhereUsed(ctx, uri)
				results[i] = WhereUsedBatchEntry{ObjectURI: uri, References: refs}
				if err != nil {
					results[i].Error = err.Error()
				}
			}(i, uri)
		}
		wg.Wait()

		totalRefs := 0
		for _, r := range results {
			totalRefs += len(r.References)
		}

		return mcp.NewToolResultJSON(WhereUsedBatchResult{
			Total:           len(multi),
			TotalReferences: totalRefs,
			Results:         results,
		})
	})

	// get_object_dependencies resolves the DDIC objects and OO relationships an
	// ABAP object references. The DDIC/OO engine (D010TAB, DD0xL, SEOMETAREL,
	// pool-program naming, BFS) lives in adtler's DependencyClient; this handler
	// only parses args and shapes the result.
	s.AddTool(mcp.NewTool("get_object_dependencies",
		mcp.WithTitleAnnotation("Get Object Dependencies"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Find all DDIC objects (tables, structures, types) and OO relationships "+
				"(implemented interfaces, superclass) that a given ABAP object references. "+
				"Counterpart to where_used, which answers the reverse question. "+
				"Supported object types:\n"+
				"  PROG — program: queries D010TAB (MASTER = program name)\n"+
				"  FUGR — function group: queries D010TAB (MASTER = SAPL<name>)\n"+
				"  FUNC — function module: resolves FUGR via TFDIR, then queries D010TAB\n"+
				"  CLAS — class: queries D010TAB (class pool program) + SEOMETAREL (interfaces, superclass)\n"+
				"  INTF — interface: queries D010TAB (interface pool program) + SEOMETAREL (extended interfaces)\n"+
				"  TABL — transparent table or structure: BFS over DD03L (fields→DTELs, check tables)\n"+
				"  DTEL — data element: BFS over DD04L (domain)\n"+
				"  DOMA — domain: BFS over DD01L (entity table)\n"+
				"  TTYP — table type: BFS over DD40L (row type)\n"+
				"For PROG/FUGR/FUNC/CLAS/INTF, D010TAB is populated flat by the ABAP activator — no client-side recursion needed. "+
				"For TABL/DTEL/DOMA/TTYP, the DDIC type chain is traversed iteratively up to max_depth levels. "+
				"Useful for transport completeness checks.",
		),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("ABAP object type: PROG, FUGR, FUNC, CLAS, INTF, TABL, DTEL, DOMA, TTYP")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("Object name, e.g. Z_MY_REPORT (PROG), Z_MY_FGRP (FUGR), Z_MY_FM (FUNC), ZCL_MY_CLASS (CLAS), ZIF_MY_INTF (INTF), ZORDERS (TABL), S_CARR_ID (DTEL), S_LAND1 (DOMA)")),
		mcp.WithNumber("max_results", mcp.Description("Maximum number of results to return (default: 200)")),
		mcp.WithNumber("max_depth", mcp.Description("Maximum BFS depth for TABL/DTEL/DOMA/TTYP traversal (default: 3, min: 1, max: 10; ignored for PROG/FUGR/FUNC/CLAS/INTF)")),
		mcp.WithOutputSchema[adt.DependencyResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objType := req.GetString("object_type", "")
		objName := req.GetString("object_name", "")
		maxResults := int(req.GetFloat("max_results", 200))
		maxDepth := int(req.GetFloat("max_depth", 3))

		result, err := client.GetObjectDependencies(ctx, objType, objName, maxResults, maxDepth)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
