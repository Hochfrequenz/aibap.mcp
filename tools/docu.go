package tools

import (
	"context"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerDocuTools(s toolAdder, client adt.DocuClient) {
	s.AddTool(mcp.NewTool("get_abap_doc",
		mcp.WithTitleAnnotation("Get ABAP Documentation"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Look up ABAP keyword documentation from SAP's built-in help. Returns plain text documentation for the given keyword (e.g. SELECT, LOOP, DATA, CLASS)."),
		mcp.WithString("keyword", mcp.Required(), mcp.Description("ABAP keyword to look up (e.g. SELECT, LOOP, DATA)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keyword := req.GetString("keyword", "")
		if keyword == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "keyword must not be empty"}), nil
		}
		doc, err := client.GetABAPDoc(ctx, keyword)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(doc), nil
	})
}
