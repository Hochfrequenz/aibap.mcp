package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerRefactoringTools(s toolAdder, client adt.RefactoringClient) {
	s.AddTool(mcp.NewTool("rename",
		mcp.WithTitleAnnotation("Rename Symbol"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Rename an ABAP variable, method, or other symbol. Automatically finds and updates all references. "+
				"Pass the source URI with position of the symbol to rename "+
				"(e.g. /sap/bc/adt/programs/programs/z_report/source/main#start=5,7).",
		),
		mcp.WithString("source_uri", mcp.Required(), mcp.Description("Source URI with position of the symbol (#start=line,col)")),
		mcp.WithString("new_name", mcp.Required(), mcp.Description("New name for the symbol")),
		mcp.WithString("transport", mcp.Description("Transport request number (required for non-local objects)")),
		mcp.WithOutputSchema[adt.RenameResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, errRes := requireString(req, "source_uri")
		if errRes != nil {
			return errRes, nil
		}
		newName, errRes := requireString(req, "new_name")
		if errRes != nil {
			return errRes, nil
		}
		transport := req.GetString("transport", "")
		result, err := client.Rename(ctx, uri, newName, transport)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}
