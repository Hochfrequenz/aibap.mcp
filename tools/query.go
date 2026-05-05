package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

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
				"SAP API Policy: This tool is intended for development tooling only (e.g. inspecting DDIC metadata, "+
				"querying customizing tables during development). "+
				"Do not use it for reading application or business tables, data integration, reporting, "+
				"or any purpose outside the ADT development tooling scope — "+
				"this would violate the SAP API Policy (https://help.sap.com/doc/sap-api-policy/latest/en-US/API_Policy_latest.pdf).",
		),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL SELECT statement, e.g. 'SELECT BUKRS, BUTXT FROM T001'")),
		mcp.WithNumber("max_rows", mcp.Description("Maximum number of rows to return (default: 100)")),
		mcp.WithOutputSchema[adt.QueryResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sql := req.GetString("sql", "")
		maxRows := int(req.GetFloat("max_rows", 100))
		result, err := client.RunQuery(ctx, sql, maxRows)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
