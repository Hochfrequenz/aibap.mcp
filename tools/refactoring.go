package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerRefactoringTools(s toolAdder, client adt.RefactoringClient, elicitor Elicitor) {
	_ = elicitor // wired by Task 6 to confirm destructive operations
	s.AddTool(mcp.NewTool("rename",
		mcp.WithTitleAnnotation("Rename Symbol"),
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
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("source_uri", "")
		newName := req.GetString("new_name", "")
		transport := req.GetString("transport", "")
		if uri == "" || newName == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "source_uri and new_name are required"}), nil
		}
		result, err := client.Rename(ctx, uri, newName, transport)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
