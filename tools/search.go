package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// searchQueryClient is the combined interface required by registerSearchTools.
// It extends adt.SearchClient with adt.QueryClient so get_object_dependencies
// can call RunQuery without changing the register.go call site.
type searchQueryClient interface {
	adt.SearchClient
	adt.QueryClient
}

func registerSearchTools(s toolAdder, client searchQueryClient) {
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

	// get_object_dependencies is intentionally NOT recursive:
	//
	// For DDIC references, recursion is unnecessary. D010TAB is populated by the ABAP
	// activator at activation time and already stores the complete, flat set of DDIC
	// objects (tables, structures, type pools) used by a program — including all objects
	// pulled in transitively via INCLUDE statements. A single query with MASTER = '<prog>'
	// therefore returns the full dependency set with no need for client-side recursion.
	//
	// The scenario where recursion *would* be needed is transitive program-level dependencies
	// (e.g. CALL PROGRAM / SUBMIT). D010TAB does not model those relationships at all; that
	// is a different, significantly more complex question and is deliberately out of scope for
	// this tool. If that information is ever needed, a separate tool should be implemented.
	s.AddTool(mcp.NewTool("get_object_dependencies",
		mcp.WithTitleAnnotation("Get Object Dependencies"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Find all DDIC objects (tables, structures, types) that a given ABAP program references at runtime. "+
				"Counterpart to where_used, which answers the reverse question. "+
				"Queries D010TAB, the ABAP program-to-DDIC dependency table. "+
				"Useful for transport completeness checks: given a PROG in a transport, "+
				"find which DDIC objects it depends on. "+
				"The result is already complete and flat: D010TAB is populated by the ABAP activator and "+
				"includes all objects pulled in via INCLUDE statements, so no further recursion is needed "+
				"for DDIC references. "+
				"Note: transitive program-level dependencies (CALL PROGRAM, SUBMIT) are NOT covered "+
				"by this tool — D010TAB does not model those relationships.",
		),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("ABAP object type — currently only PROG is supported (D010TAB)")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("Program name, e.g. Z_MY_REPORT or SAPL_MY_FUGR")),
		mcp.WithNumber("max_results", mcp.Description("Maximum number of results to return (default: 200)")),
		mcp.WithOutputSchema[ObjectDependenciesResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objType := req.GetString("object_type", "")
		objName := req.GetString("object_name", "")
		maxResults := int(req.GetFloat("max_results", 200))

		sql := fmt.Sprintf(
			"SELECT TABNAME FROM D010TAB WHERE MASTER = '%s' ORDER BY TABNAME",
			adt.EscapeValue(objName),
		)

		queryResult, err := client.RunQuery(ctx, sql, maxResults)
		if err != nil {
			return errorResult(err), nil
		}
		if queryResult == nil {
			queryResult = &adt.QueryResult{}
		}

		deps := make([]ObjectDependency, 0, len(queryResult.Rows))
		for _, row := range queryResult.Rows {
			if len(row) < 1 || row[0] == "" {
				continue
			}
			deps = append(deps, ObjectDependency{Name: row[0], UseType: "TABLE"})
		}

		return mcp.NewToolResultJSON(ObjectDependenciesResult{
			ObjectType:   objType,
			ObjectName:   objName,
			Count:        len(deps),
			Dependencies: deps,
		})
	})
}
