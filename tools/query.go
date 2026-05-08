package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validQueryPurposes lists the only values accepted for the run_query
// "purpose" parameter. Callers outside this set must obtain explicit user
// confirmation via the Elicitor before the query is executed.
var validQueryPurposes = map[string]bool{
	"ddic_inspection":      true,
	"customizing_review":   true,
	"transport_tracking":   true,
	"development_metadata": true,
}

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
					"run_query blocked: 'purpose' is missing or not a recognised development-tooling value. "+
						"Valid values: ddic_inspection, customizing_review, transport_tracking, development_metadata. "+
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
			"enum": []any{
				"ddic_inspection",
				"customizing_review",
				"transport_tracking",
				"development_metadata",
			},
		}
		t.InputSchema.Required = append(t.InputSchema.Required, "purpose")
	}
}
