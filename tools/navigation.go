package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerNavigationTools(s toolAdder, client adt.NavigationClient) {
	s.AddTool(mcp.NewTool("navigate_to_definition",
		mcp.WithTitleAnnotation("Navigate to Definition"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Navigate to the definition of an ABAP object referenced at a source position. "+
				"Pass the source URI with a line/column fragment (e.g. /sap/bc/adt/programs/programs/z_report/source/main#start=15,4). "+
				"Returns the ADT URI of the definition.",
		),
		mcp.WithString("source_uri", mcp.Required(), mcp.Description("Source URI with position fragment (e.g. .../source/main#start=15,4)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("source_uri", "")
		if uri == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "source_uri must not be empty"}), nil
		}
		targetURI, err := client.NavigateToDefinition(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{"definition_uri": targetURI})
		return mcp.NewToolResultText(string(out)), nil
	})
}
