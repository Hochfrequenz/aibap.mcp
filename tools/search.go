package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// useType constants for ObjectDependency.UseType.
// Defined here because goconst requires repeated string literals to be extracted;
// they are also exported-style names to serve as documentation for callers.
const (
	useTypeTable       = "TABLE"
	useTypeStructure   = "STRUCTURE"
	useTypeDataElement = "DATA_ELEMENT"
	useTypeDomain      = "DOMAIN"
	useTypeView        = "VIEW"
	useTypeTableType   = "TABLE_TYPE"
	useTypeInterface   = "INTERFACE"
	useTypeSuperclass  = "SUPERCLASS"
	useTypeUnknown     = "UNKNOWN"
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

		var names []string
		deps := make([]ObjectDependency, 0, len(queryResult.Rows))
		for _, row := range queryResult.Rows {
			if len(row) < 1 || row[0] == "" {
				continue
			}
			names = append(names, row[0])
			deps = append(deps, ObjectDependency{Name: row[0]})
		}

		if len(names) > 0 {
			classification := classifyDDICObjects(ctx, client, names)
			for i := range deps {
				deps[i].UseType = classification[deps[i].Name]
			}
		}

		return mcp.NewToolResultJSON(ObjectDependenciesResult{
			ObjectType:   objType,
			ObjectName:   objName,
			Count:        len(deps),
			Dependencies: deps,
		})
	})
}

// classifyDDICObjects resolves the actual DDIC kind for each name using two
// sequential queries, chosen to handle the full range of objects that D010TAB
// can reference.
//
// Step 1 — DD02L: the DDIC table/structure catalog. This covers transparent
// tables, structures, cluster tables, pool tables, and views. Crucially it also
// covers SAP system objects like SYST and SCREEN that do not appear in TADIR
// under PGMID='R3TR', so DD02L must be the first source, not a fallback.
//
// Step 2 — TADIR: the global ABAP repository directory. This covers data
// elements (DTEL), domains (DOMA), and table types (TTYP) that are not stored
// in DD02L at all. Only names that are still UNKNOWN after step 1 are sent to
// TADIR, keeping the query count to one for programs whose dependencies are
// exclusively tables/structures. PGMID='R3TR' limits the result to repository
// objects (excludes transport request entries under PGMID='CORR').
//
// Errors from either query are silently swallowed so that a transient network
// hiccup degrades gracefully; affected names stay UNKNOWN rather than failing
// the whole tool call.
func classifyDDICObjects(ctx context.Context, client adt.QueryClient, names []string) map[string]string {
	result := make(map[string]string, len(names))
	for _, n := range names {
		result[n] = useTypeUnknown
	}

	// Step 1 — DD02L: classify all names that are tables or structures.
	if qr, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT TABNAME, TABCLASS FROM DD02L WHERE TABNAME IN (%s)", buildSQLInList(names)),
		len(names)); err == nil && qr != nil {
		for _, row := range qr.Rows {
			if len(row) >= 2 {
				result[row[0]] = tabclassToUseType(row[1])
			}
		}
	}

	// Step 2 — TADIR: classify remaining names that DD02L did not cover.
	// These are typically data elements, domains, and table types.
	var unknownNames []string
	for _, n := range names {
		if result[n] == useTypeUnknown {
			unknownNames = append(unknownNames, n)
		}
	}
	if len(unknownNames) > 0 {
		if qr, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT OBJECT, OBJ_NAME FROM TADIR WHERE PGMID = 'R3TR' AND OBJ_NAME IN (%s)", buildSQLInList(unknownNames)),
			len(unknownNames)); err == nil && qr != nil {
			for _, row := range qr.Rows {
				if len(row) < 2 {
					continue
				}
				objType, objName := row[0], row[1]
				switch objType {
				case "DTEL":
					result[objName] = useTypeDataElement
				case "DOMA":
					result[objName] = useTypeDomain
				case "TTYP":
					result[objName] = useTypeTableType
				case "VIEW":
					result[objName] = useTypeView
					// Other TADIR types (PROG, FUGR, CLAS, …) are not expected in
					// D010TAB (which tracks only DDIC dependencies). Leave them UNKNOWN
					// so callers see an honest signal rather than a wrong classification.
				}
			}
		}
	}

	return result
}

func buildSQLInList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + adt.EscapeValue(n) + "'"
	}
	return strings.Join(quoted, ",")
}

// fugrPoolProgramName constructs the D010TAB MASTER key for a function group.
// SAP generates a function pool program: SAPL<name> for non-namespaced groups,
// <namespace>SAPL<local> for namespaced groups (e.g. /NS/FUGR -> /NS/SAPLFUGR).
func fugrPoolProgramName(fugrName string) string {
	if len(fugrName) > 0 && fugrName[0] == '/' {
		if idx := strings.Index(fugrName[1:], "/"); idx >= 0 {
			ns := fugrName[:idx+2]    // "/NS/"
			local := fugrName[idx+2:] // "LOCALNAME"
			return ns + "SAPL" + local
		}
	}
	return "SAPL" + fugrName
}

// classPoolProgramName constructs the D010TAB MASTER key for a class.
// SAP generates a class pool program: <CLASSNAME> padded with '=' to 30 chars + "CP".
// Verified on S/4 live system (see issue #343).
func classPoolProgramName(className string) string {
	const padLen = 30
	if len(className) >= padLen {
		return className + "CP"
	}
	return className + strings.Repeat("=", padLen-len(className)) + "CP"
}

// intfPoolProgramName constructs the D010TAB MASTER key for an interface.
// SAP generates an interface pool program: <INTFNAME> padded with '=' to 30 chars + "IP".
// Verified on S/4 live system (see issue #343).
func intfPoolProgramName(intfName string) string {
	const padLen = 30
	if len(intfName) >= padLen {
		return intfName + "IP"
	}
	return intfName + strings.Repeat("=", padLen-len(intfName)) + "IP"
}

// tabclassToUseType maps DD02L.TABCLASS to a use_type string.
// TRANSP = regular transparent table (1:1 DB mapping, the common case).
// INTTAB = structure/internal table type (no own DB table).
// CLUSTER / POOL = physical storage optimisations; logically still tables.
// VIEW = database or maintenance view stored in DD02L (rare in D010TAB).
// Unknown TABCLASS values get UNKNOWN rather than TABLE so future SAP object
// kinds don't silently masquerade as tables.
func tabclassToUseType(tabclass string) string {
	switch tabclass {
	case "TRANSP":
		return useTypeTable
	case "INTTAB":
		return useTypeStructure
	case "CLUSTER", "POOL":
		return useTypeTable
	case "VIEW":
		return useTypeView
	default:
		return useTypeUnknown
	}
}
