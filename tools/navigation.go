package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

type NavigationResult struct {
	DefinitionURI string `json:"definition_uri"`
}

func registerNavigationTools(s toolAdder, client adt.NavigationClient) {
	s.AddTool(mcp.NewTool("navigate_to_definition",
		mcp.WithTitleAnnotation("Navigate to Definition"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Navigate to the definition of an ABAP object referenced at a source position. "+
				"Pass the source URI with a line/column fragment (e.g. /sap/bc/adt/programs/programs/z_report/source/main#start=15,4) "+
				"and the current source code that the cursor refers to. "+
				"Returns the ADT URI of the definition.",
		),
		mcp.WithString("source_uri", mcp.Required(), mcp.Description("Source URI with position fragment (e.g. .../source/main#start=15,4)")),
		mcp.WithString("source", mcp.Required(), mcp.Description("Current ABAP source code at source_uri")),
		mcp.WithOutputSchema[NavigationResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, errRes := requireString(req, "source_uri")
		if errRes != nil {
			return errRes, nil
		}
		// "source" carries ABAP source code, where leading/trailing whitespace
		// can be significant (and source_uri's #start=line,col fragment refers
		// to positions within it) — requireString's trimming would corrupt
		// that, so this only checks presence via mcp-go directly.
		source, err := req.RequireString("source")
		if err != nil {
			return errorResult(fmt.Errorf("invalid required parameter %q: %w", "source", err)), nil
		}
		targetURI, err := client.NavigateToDefinition(ctx, uri, source)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(NavigationResult{DefinitionURI: targetURI})
	})
}
