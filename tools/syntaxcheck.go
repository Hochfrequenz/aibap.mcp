package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSyntaxCheckTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("syntax_check",
		mcp.WithTitleAnnotation("Syntax Check"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Run ABAP syntax check on one or more saved objects. Checks the inactive (saved but not yet activated) version. "+
				"Pass a single URI string for one object: returns messages with type (E/W/I), line, column. "+
				"Pass an array of URIs to use SAP's native batch endpoint (chunks of 10): returns {total, clean, total_errors, total_warnings, results:[...]}. "+
				"To check code without saving to an object, use verify_source instead.",
		),
		withStringOrArray(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		single, multi := getStringOrSlice(req.GetArguments(), paramObjectURI)
		if multi == nil {
			msgs, err := client.SyntaxCheck(ctx, single)
			if err != nil {
				return errorResult(err), nil
			}
			out, _ := json.Marshal(msgs)
			return mcp.NewToolResultText(string(out)), nil
		}

		results := client.BatchSyntaxCheck(ctx, multi)

		// Compute summary counts.
		totalErrors, totalWarnings, clean := 0, 0, 0
		for _, r := range results {
			hasError := false
			for _, m := range r.Messages {
				switch m.Type {
				case "E":
					totalErrors++
					hasError = true
				case "W":
					totalWarnings++
				}
			}
			if !hasError && r.Error == "" {
				clean++
			}
		}

		out, _ := json.Marshal(map[string]any{
			"total":          len(multi),
			"clean":          clean,
			"total_errors":   totalErrors,
			"total_warnings": totalWarnings,
			"results":        results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
