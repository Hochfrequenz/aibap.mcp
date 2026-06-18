package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// VerifyResult is returned by verify_source.
type VerifyResult struct {
	Valid    bool                `json:"valid"`
	Messages []adt.SyntaxMessage `json:"messages"`
}

func registerVerifyTools(s toolAdder, client adt.Client) {
	s.AddTool(mcp.NewTool("verify_source",
		mcp.WithTitleAnnotation("Verify Source"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Syntax-check ABAP source code without needing an existing object. "+
				"Creates a temporary program in $TMP, checks the source, and cleans up. "+
				"Returns {valid: true/false, messages: [...]}.",
		),
		mcp.WithString("source", mcp.Required(), mcp.Description("ABAP source code to check")),
		mcp.WithOutputSchema[VerifyResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source := req.GetString("source", "")
		if source == "" {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "source must not be empty"}), nil
		}

		valid, msgs, err := client.VerifySource(ctx, source)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(VerifyResult{Valid: valid, Messages: msgs})
	})
}
