package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSyntaxCheckTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("syntax_check",
		mcp.WithTitleAnnotation("Syntax Check"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Run ABAP syntax check on a saved object. Checks the inactive (saved but not yet activated) version. Returns messages with type (E/W/I), line, column. To check code without saving to an object, use verify_source instead."),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		msgs, err := client.SyntaxCheck(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(msgs)
		return mcp.NewToolResultText(string(out)), nil
	})
}
