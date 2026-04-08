package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerPrettyPrinterTools(s toolAdder, client adt.SourceClient) {
	s.AddTool(mcp.NewTool("pretty_print",
		mcp.WithTitleAnnotation("Pretty Print Source"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Format ABAP source code using the SAP Pretty Printer."),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("ABAP source code to format"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source := req.GetString("source", "")
		formatted, err := client.PrettyPrint(ctx, source)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(formatted), nil
	})
}
