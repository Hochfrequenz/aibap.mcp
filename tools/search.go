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

	// get_object_dependencies is intentionally NOT recursive for PROG/FUGR/FUNC/CLAS/INTF:
	//
	// For those types, D010TAB is populated by the ABAP activator at activation time and
	// already stores the complete, flat set of DDIC objects (tables, structures, type pools)
	// used by a program — including all objects pulled in transitively via INCLUDE statements.
	// A single query with MASTER = '<prog>' therefore returns the full dependency set with no
	// need for client-side recursion.
	//
	// TABL/DTEL/DOMA/TTYP use iterative BFS (ddicChainDeps) instead of recursive function
	// calls. D010TAB does not model DDIC→DDIC chains at all — a TABL entry in D010TAB names
	// the table but not its data elements, domains, or check tables. BFS over the DDIC catalog
	// tables (DD03L, DD04L, DD01L, DD40L) is therefore required. Iterative BFS rather than
	// recursive descent avoids stack overflow on cyclic type chains (e.g. DOMA→ENTITYTAB→
	// TABL→field ROLLNAME→DTEL→DOMA is a classic cycle).
	//
	// The scenario where recursion *would* be needed for PROG-level deps is transitive
	// program-level dependencies (e.g. CALL PROGRAM / SUBMIT). D010TAB does not model those
	// relationships; that is a different, significantly more complex question and is
	// deliberately out of scope for this tool.
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
		mcp.WithOutputSchema[ObjectDependenciesResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objType := strings.ToUpper(req.GetString("object_type", ""))
		objName := req.GetString("object_name", "")
		maxResults := int(req.GetFloat("max_results", 200))

		switch objType {
		case "PROG":
			deps, err := d010tabDeps(ctx, client, objName, maxResults)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(deps),
				Dependencies: deps,
			})
		case "FUGR":
			master := fugrPoolProgramName(objName)
			deps, err := d010tabDeps(ctx, client, master, maxResults)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(deps),
				Dependencies: deps,
			})

		case "FUNC":
			master, err := funcPoolProgramName(ctx, client, objName)
			if err != nil {
				return errorResult(err), nil
			}
			deps, err := d010tabDeps(ctx, client, master, maxResults)
			if err != nil {
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(deps),
				Dependencies: deps,
			})

		case "CLAS":
			master := classPoolProgramName(objName)
			ddic, err := d010tabDeps(ctx, client, master, maxResults)
			if err != nil {
				return errorResult(err), nil
			}
			oo, err := ooDeps(ctx, client, objName, []string{"1", "2"})
			if err != nil {
				return errorResult(err), nil
			}
			all := make([]ObjectDependency, 0, len(ddic)+len(oo))
			all = append(all, ddic...)
			all = append(all, oo...)
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(all),
				Dependencies: all,
			})

		case "INTF":
			master := intfPoolProgramName(objName)
			ddic, err := d010tabDeps(ctx, client, master, maxResults)
			if err != nil {
				return errorResult(err), nil
			}
			oo, err := ooDeps(ctx, client, objName, []string{"0"})
			if err != nil {
				return errorResult(err), nil
			}
			all := make([]ObjectDependency, 0, len(ddic)+len(oo))
			all = append(all, ddic...)
			all = append(all, oo...)
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(all),
				Dependencies: all,
			})

		case "TABL", "DTEL", "DOMA", "TTYP":
			maxDepth := int(req.GetFloat("max_depth", 3))
			if maxDepth < 1 {
				maxDepth = 1
			}
			if maxDepth > 10 {
				maxDepth = 10
			}
			deps, warns := ddicChainDeps(ctx, client, objName, objType, maxDepth)
			if maxResults > 0 && len(deps) > maxResults {
				warns = append(warns, fmt.Sprintf("output truncated to %d entries (%d total)", maxResults, len(deps)))
				deps = deps[:maxResults]
			}
			return mcp.NewToolResultJSON(ObjectDependenciesResult{
				ObjectType:   objType,
				ObjectName:   objName,
				Count:        len(deps),
				Dependencies: deps,
				Warnings:     warns,
			})

		default:
			return errorResult(fmt.Errorf("unsupported object_type %q: supported are PROG, FUGR, FUNC, CLAS, INTF, TABL, DTEL, DOMA, TTYP", objType)), nil
		}
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

// d010tabDeps uses the narrow adt.QueryClient interface (not searchQueryClient) so it
// can be called directly from object-type-specific switch cases without coupling them
// to the search client. D010TAB is the right source here: it is populated flat by the
// ABAP activator at activation time, so one query returns the complete dependency set.
func d010tabDeps(ctx context.Context, client adt.QueryClient, master string, maxResults int) ([]ObjectDependency, error) {
	qr, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT TABNAME FROM D010TAB WHERE MASTER = '%s' ORDER BY TABNAME", adt.EscapeValue(master)),
		maxResults)
	if err != nil {
		return nil, err
	}
	if qr == nil {
		return nil, nil
	}
	var names []string
	deps := make([]ObjectDependency, 0, len(qr.Rows))
	for _, row := range qr.Rows {
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
	return deps, nil
}

// seometarelMaxRows caps OO relationship lookups. SAP class/interface metadata
// is populated during activation and is bounded by the number of directly inherited
// or implemented types; 100 is well above any realistic class hierarchy depth.
const seometarelMaxRows = 100

// ooDeps complements d010tabDeps for OO types: D010TAB covers DDIC references but does
// not model class hierarchy or interface implementation. SEOMETAREL is SAP's OO
// meta-relationship table; callers pass relTypes to select the relevant relationship
// kinds (CLAS: ["1","2"], INTF: ["0"]) without duplicating the query logic.
func ooDeps(ctx context.Context, client adt.QueryClient, clsName string, relTypes []string) ([]ObjectDependency, error) {
	qr, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT REFCLSNAME, RELTYPE FROM SEOMETAREL WHERE CLSNAME = '%s' AND RELTYPE IN (%s) ORDER BY RELTYPE, REFCLSNAME",
			adt.EscapeValue(clsName), buildSQLInList(relTypes)),
		seometarelMaxRows)
	if err != nil {
		return nil, err
	}
	if qr == nil {
		return nil, nil
	}
	deps := make([]ObjectDependency, 0, len(qr.Rows))
	for _, row := range qr.Rows {
		if len(row) < 2 || row[0] == "" {
			continue
		}
		deps = append(deps, ObjectDependency{Name: row[0], UseType: ooRelTypeToUseType(row[1])})
	}
	return deps, nil
}

// ooRelTypeToUseType collapses RELTYPE "0" (interface extension) and "1" (interface
// implementation) into a single INTERFACE use_type — the distinction matters for SAP
// internally but is not meaningful for transport completeness checks, which is this
// tool's primary use case. RELTYPE "2" (superclass) is kept separate because it names
// a class, not an interface, and callers may need to treat it differently.
func ooRelTypeToUseType(relType string) string {
	switch relType {
	case "0", "1":
		return useTypeInterface
	case "2":
		return useTypeSuperclass
	default:
		return useTypeUnknown
	}
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
// Verified against live S/4 system via TFDIR.PNAME lookup (e.g. /1BCDWB/SF00000001 -> /1BCDWB/SAPLSF00000001).
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

// funcPoolProgramName resolves the D010TAB MASTER key for a FUNC object by looking up
// TFDIR.PNAME — the function pool program SAP generated for the function module's group.
func funcPoolProgramName(ctx context.Context, client adt.QueryClient, funcName string) (string, error) {
	qr, err := client.RunQuery(ctx,
		fmt.Sprintf("SELECT PNAME FROM TFDIR WHERE FUNCNAME = '%s'", adt.EscapeValue(funcName)),
		1)
	if err != nil {
		return "", fmt.Errorf("looking up function module %q in TFDIR: %w", funcName, err)
	}
	if qr == nil || len(qr.Rows) == 0 || len(qr.Rows[0]) == 0 || qr.Rows[0][0] == "" {
		return "", fmt.Errorf("function module %q not found in TFDIR", funcName)
	}
	return qr.Rows[0][0], nil
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

// ddicMaxFieldRows caps DD03L batch queries. A table with 2000 fields would be
// pathological; realistic SAP tables top out well below this.
const ddicMaxFieldRows = 2000

// ddicInListMaxBytes is the maximum byte length of a buildSQLInList result per
// query batch. SAP's data-preview ABAP endpoint appends an internal INTO TABLE
// clause; if the total SQL grows large, the parser misreads long IN-lists as
// an unclosed string literal and raises a 400 "text literal longer than 255
// characters" error. 150 bytes allows ~4-5 namespaced (30-char) names per
// batch while permitting ~14 short standard names — keeping all batches well
// inside the safe zone.
const ddicInListMaxBytes = 150

// chunkNames splits names into batches where buildSQLInList output for each
// batch stays within maxBytes. This avoids SAP data-preview parser errors when
// IN-lists become very long (see ddicInListMaxBytes).
func chunkNames(names []string, maxBytes int) [][]string {
	var chunks [][]string
	var cur []string
	curLen := 0
	for _, n := range names {
		entryLen := len(n) + 3 // 'name', = name + 2 quotes + 1 comma
		if len(cur) > 0 && curLen+entryLen > maxBytes {
			chunks = append(chunks, cur)
			cur = nil
			curLen = 0
		}
		cur = append(cur, n)
		curLen += entryLen
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// ddicQueryTabl queries DD03L for TABL entries: ROLLNAME → DTEL, CHECKTABLE → TABL.
func ddicQueryTabl(ctx context.Context, client adt.QueryClient, names []string, addDep func(string, string, string), warnings *[]string) {
	for _, chunk := range chunkNames(names, ddicInListMaxBytes) {
		qr, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT ROLLNAME, CHECKTABLE FROM DD03L WHERE TABNAME IN (%s)", buildSQLInList(chunk)),
			ddicMaxFieldRows)
		if err != nil {
			*warnings = append(*warnings, "DD03L query failed: "+err.Error())
			continue
		}
		if qr == nil {
			continue
		}
		for _, row := range qr.Rows {
			if len(row) < 2 {
				continue
			}
			if row[0] != "" {
				addDep(row[0], "DTEL", useTypeDataElement)
			}
			if row[1] != "" {
				addDep(row[1], "TABL", useTypeTable)
			}
		}
	}
}

// ddicQueryDtel queries DD04L for DTEL entries: DOMNAME → DOMA.
// DD04L has ROLLNAME as PK, so the mapping is 1:1 — maxRows = chunk size is exact.
func ddicQueryDtel(ctx context.Context, client adt.QueryClient, names []string, addDep func(string, string, string), warnings *[]string) {
	for _, chunk := range chunkNames(names, ddicInListMaxBytes) {
		qr, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT DOMNAME FROM DD04L WHERE ROLLNAME IN (%s)", buildSQLInList(chunk)),
			len(chunk))
		if err != nil {
			*warnings = append(*warnings, "DD04L query failed: "+err.Error())
			continue
		}
		if qr == nil {
			continue
		}
		for _, row := range qr.Rows {
			if len(row) < 1 {
				continue
			}
			addDep(row[0], "DOMA", useTypeDomain)
		}
	}
}

// ddicQueryDoma queries DD01L for DOMA entries: ENTITYTAB → TABL.
// DD01L has DOMNAME as PK, so the mapping is 1:1 — maxRows = chunk size is exact.
func ddicQueryDoma(ctx context.Context, client adt.QueryClient, names []string, addDep func(string, string, string), warnings *[]string) {
	for _, chunk := range chunkNames(names, ddicInListMaxBytes) {
		qr, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT ENTITYTAB FROM DD01L WHERE DOMNAME IN (%s)", buildSQLInList(chunk)),
			len(chunk))
		if err != nil {
			*warnings = append(*warnings, "DD01L query failed: "+err.Error())
			continue
		}
		if qr == nil {
			continue
		}
		for _, row := range qr.Rows {
			if len(row) < 1 {
				continue
			}
			addDep(row[0], "TABL", useTypeTable)
		}
	}
}

// ddicQueryTtyp queries DD40L for TTYP entries: ROWKIND='E'→DTEL, ROWKIND='S'→TABLE/STRUCTURE.
// ROWKIND='' means built-in scalar, no further traversal needed.
// DD40L has TYPENAME as PK; ddicMaxFieldRows is used as a safety cap.
func ddicQueryTtyp(ctx context.Context, client adt.QueryClient, names []string, addDep func(string, string, string), warnings *[]string) {
	var rowKindS []string
	for _, chunk := range chunkNames(names, ddicInListMaxBytes) {
		qr, err := client.RunQuery(ctx,
			fmt.Sprintf("SELECT ROWTYPE, ROWKIND FROM DD40L WHERE TYPENAME IN (%s)", buildSQLInList(chunk)),
			ddicMaxFieldRows)
		if err != nil {
			*warnings = append(*warnings, "DD40L query failed: "+err.Error())
			continue
		}
		if qr == nil {
			continue
		}
		for _, row := range qr.Rows {
			if len(row) < 2 || row[0] == "" {
				continue
			}
			rowType, rowKind := row[0], row[1]
			switch rowKind {
			case "E":
				addDep(rowType, "DTEL", useTypeDataElement)
			case "S":
				rowKindS = append(rowKindS, rowType)
			}
		}
	}
	// Classify all ROWKIND='S' entries across chunks: TABLE vs STRUCTURE requires a DD02L lookup.
	if len(rowKindS) > 0 {
		cls := classifyDDICObjects(ctx, client, rowKindS)
		for _, n := range rowKindS {
			addDep(n, "TABL", cls[n])
		}
	}
}

// ddicChainDeps traverses the DDIC type chain starting from a single object
// (TABL, DTEL, DOMA, or TTYP) using iterative BFS. It returns a flat, deduplicated
// list of all transitive dependencies with correct use_type classification, along
// with any non-fatal query warnings.
//
// Iterative BFS rather than recursive function calls avoids stack overflow on
// cyclic type chains. DOMA→ENTITYTAB→TABL→field ROLLNAME→DTEL→DOMA is a classic cycle;
// the visited map catches it at the DOMA re-entry so BFS terminates cleanly.
func ddicChainDeps(ctx context.Context, client adt.QueryClient, name, objType string, maxDepth int) ([]ObjectDependency, []string) {
	type queueEntry struct {
		name    string
		objType string
	}

	visited := make(map[string]bool)
	visited[objType+"|"+name] = true

	var deps []ObjectDependency
	var warnings []string

	current := []queueEntry{{name: name, objType: objType}}

	for depth := 0; depth < maxDepth && len(current) > 0; depth++ {
		var next []queueEntry

		// addDep registers one discovered dependency, guarded by the visited map.
		addDep := func(depName, depType, useType string) {
			if depName == "" {
				return
			}
			k := depType + "|" + depName
			if visited[k] {
				return
			}
			visited[k] = true
			deps = append(deps, ObjectDependency{Name: depName, UseType: useType})
			next = append(next, queueEntry{name: depName, objType: depType})
		}

		// Collect by DDIC type so each catalog table is queried exactly once per level.
		typeGroups := map[string][]string{}
		for _, e := range current {
			typeGroups[e.objType] = append(typeGroups[e.objType], e.name)
		}

		if tabls := typeGroups["TABL"]; len(tabls) > 0 {
			ddicQueryTabl(ctx, client, tabls, addDep, &warnings)
		}
		if dtels := typeGroups["DTEL"]; len(dtels) > 0 {
			ddicQueryDtel(ctx, client, dtels, addDep, &warnings)
		}
		if domas := typeGroups["DOMA"]; len(domas) > 0 {
			ddicQueryDoma(ctx, client, domas, addDep, &warnings)
		}
		if ttyps := typeGroups["TTYP"]; len(ttyps) > 0 {
			ddicQueryTtyp(ctx, client, ttyps, addDep, &warnings)
		}

		current = next
	}

	return deps, warnings
}
