package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validQueryPurposeList is the single source of truth for accepted "purpose"
// values for the run_query tool. The runtime validation map, the JSON Schema
// enum, and all human-readable strings are derived from this slice.
var validQueryPurposeList = []string{
	"ddic_inspection",
	"customizing_review",
	"transport_tracking",
	"development_metadata",
}

// validPurposesInline is a comma-separated list of valid purpose values for
// use in tool descriptions and error messages.
var validPurposesInline = strings.Join(validQueryPurposeList, ", ")

// validQueryPurposes is derived from validQueryPurposeList for O(1) lookup.
var validQueryPurposes = func() map[string]bool {
	m := make(map[string]bool, len(validQueryPurposeList))
	for _, p := range validQueryPurposeList {
		m[p] = true
	}
	return m
}()

func registerQueryTools(s toolAdder, client adt.QueryClient) {
	s.AddTool(mcp.NewTool("run_query",
		mcp.WithTitleAnnotation("Run SQL Query"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Execute a SELECT query on SAP database tables. Returns columns and rows. "+
				"Use standard ABAP SQL syntax (e.g. 'SELECT BUKRS, BUTXT FROM T001 ORDER BY BUKRS'). "+
				"Only SELECT statements are supported — no INSERT, UPDATE, or DELETE. "+
				"SAP API Policy: This tool is intended for development tooling only. "+
				"You MUST declare the purpose of the query via the 'purpose' parameter. "+
				"Valid values: "+validPurposesInline+". "+
				"Queries outside these categories may violate the SAP API Policy "+
				"(https://help.sap.com/doc/sap-api-policy/latest/en-US/API_Policy_latest.pdf). "+
				missingPurposeClause,
		),
		withQueryPurposeParam(),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL SELECT statement, e.g. 'SELECT BUKRS, BUTXT FROM T001'")),
		mcp.WithNumber("max_rows", mcp.Description("Maximum number of rows to return (default: 100)")),
		mcp.WithOutputSchema[adt.QueryResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		purpose := req.GetString("purpose", "")
		if !validQueryPurposes[purpose] {
			return errorResult(fmt.Errorf(
				"run_query blocked: 'purpose' is missing or not a recognised development-tooling value. "+
					"Valid values: %s. Querying tables outside this scope may violate the SAP API Policy",
				validPurposesInline,
			)), nil
		}

		sql := req.GetString("sql", "")
		maxRows := int(req.GetFloat("max_rows", 100))
		result, err := client.RunQuery(ctx, sql, maxRows)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}

// missingPurposeClause states what run_query does when 'purpose' is missing or
// unrecognised: it rejects locally, without reaching SAP. The purpose gate is
// a scope check under the SAP API Policy, not a consent step — it asks nothing
// of the caller's client and is unaffected by the --consent mode.
const missingPurposeClause = "A missing or unrecognised 'purpose' causes the query to be rejected without reaching SAP. "

// withQueryPurposeParam adds the optional "purpose" parameter to the run_query
// tool definition. The parameter is intentionally NOT required and carries no
// enum constraint in the JSON Schema: a schema-level required+enum would let a
// conforming MCP client reject the call generically before it reaches the
// handler, so the caller would never see which values this server accepts or
// why. Enforcement lives exclusively in the handler, which rejects with the
// list of valid purposes.
func withQueryPurposeParam() mcp.ToolOption {
	const outcome = "Omitting it or using a different value causes the query to be rejected."
	return func(t *mcp.Tool) {
		t.InputSchema.Properties["purpose"] = map[string]any{
			"type": "string",
			"description": "Declared reason for this query — should be one of the approved development-tooling categories: " +
				"ddic_inspection (DDIC metadata tables: DD01L, DD02L, …), " +
				"customizing_review (Customizing tables: T001, TVARVC, …), " +
				"transport_tracking (transport catalog tables: E070, E071, …), " +
				"development_metadata (development object catalog: TRDIR, TADIR, PROGDIR, …). " +
				outcome,
		}
	}
}
