package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validQueryPurposeList is the single source of truth for accepted "purpose"
// values for the run_query tool. Both the runtime validation map and the JSON
// Schema enum are derived from this slice so they can never silently diverge.
var validQueryPurposeList = []string{
	"ddic_inspection",
	"customizing_review",
	"transport_tracking",
	"development_metadata",
}

// validQueryPurposes is derived from validQueryPurposeList for O(1) lookup.
var validQueryPurposes = func() map[string]bool {
	m := make(map[string]bool, len(validQueryPurposeList))
	for _, p := range validQueryPurposeList {
		m[p] = true
	}
	return m
}()

func registerQueryTools(s toolAdder, client adt.QueryClient, elicitor Elicitor) {
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
				"Valid values: ddic_inspection, customizing_review, transport_tracking, development_metadata. "+
				"Queries outside these categories may violate the SAP API Policy "+
				"(https://help.sap.com/doc/sap-api-policy/latest/en-US/API_Policy_latest.pdf).",
		),
		withQueryPurposeParam(),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL SELECT statement, e.g. 'SELECT BUKRS, BUTXT FROM T001'")),
		mcp.WithNumber("max_rows", mcp.Description("Maximum number of rows to return (default: 100)")),
		mcp.WithOutputSchema[adt.QueryResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		purpose := req.GetString("purpose", "")
		if !validQueryPurposes[purpose] {
			if elicitor == nil {
				return errorResult(fmt.Errorf(
					"run_query blocked: 'purpose' is missing or not a recognised development-tooling value. " +
						"Valid values: ddic_inspection, customizing_review, transport_tracking, development_metadata. " +
						"Querying tables outside this scope may violate the SAP API Policy",
				)), nil
			}
			proceed, reason := ConfirmDestructive(ctx, elicitor,
				"run_query requires a valid purpose. Declare why this query is needed for development tooling "+
					"(ddic_inspection / customizing_review / transport_tracking / development_metadata). "+
					"If none applies, this query may violate the SAP API Policy.")
			if !proceed {
				return errorResult(fmt.Errorf("run_query aborted: %s", reason)), nil
			}
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

// withQueryPurposeParam adds the "purpose" parameter with a JSON Schema enum
// to the run_query tool definition so MCP clients (and Claude) see the valid
// values at tool-listing time.
func withQueryPurposeParam() mcp.ToolOption {
	return func(t *mcp.Tool) {
		t.InputSchema.Properties["purpose"] = map[string]any{
			"type": "string",
			"description": "Declared reason for this query — must be one of the approved development-tooling categories. " +
				"ddic_inspection: reading DDIC metadata tables (DD01L, DD02L, …). " +
				"customizing_review: reading Customizing tables (T001, TVARVC, …). " +
				"transport_tracking: reading transport catalog tables (E070, E071, …). " +
				"development_metadata: reading development object catalog tables (TRDIR, TADIR, PROGDIR, …).",
			"enum": func() []any {
				out := make([]any, len(validQueryPurposeList))
				for i, p := range validQueryPurposeList {
					out[i] = p
				}
				return out
			}(),
		}
		t.InputSchema.Required = append(t.InputSchema.Required, "purpose")
	}
}
