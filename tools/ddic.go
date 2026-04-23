package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerDDICTools(s toolAdder, client adt.DDICClient) {
	s.AddTool(mcp.NewTool("get_table_fields",
		mcp.WithTitleAnnotation("Get Table Fields"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Get the field definitions of a DDIC table or structure. "+
				"Returns field name, position, key flag, data type, length, decimals, domain, and data element. "+
				"Queries DD03L on the SAP system.",
		),
		mcp.WithString("table_name", mcp.Required(), mcp.Description("DDIC table or structure name, e.g. T001, MARA, BKPF")),
		mcp.WithOutputSchema[TableFieldsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("table_name", "")
		if name == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "table_name must not be empty"}), nil
		}
		fields, err := client.GetTableFields(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(TableFieldsResult{TableName: name, Count: len(fields), Fields: fields})
	})
}
