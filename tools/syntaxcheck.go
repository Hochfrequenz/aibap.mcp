package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSyntaxCheckTools(s toolAdder, client adt.QualityClient) {
	s.AddTool(mcp.NewTool("syntax_check",
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

	s.AddTool(mcp.NewTool("batch_syntax_check",
		mcp.WithDescription(
			"Run syntax checks on multiple ABAP objects in a single tool call. "+
				"Uses the native batch capability of SAP's checkruns endpoint. "+
				"Objects are checked in chunks of 10 per request. "+
				"Returns per-object results with messages and errors. "+
				"Use this instead of calling syntax_check in a loop to reduce round-trips.",
		),
		mcp.WithArray(paramObjectURI+"s", mcp.Required(), mcp.Description("List of ADT object URIs to check")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := req.GetStringSlice(paramObjectURI+"s", nil)
		if len(uris) == 0 {
			return errorResult(&adt.ADTError{StatusCode: 400, Message: "object_uris must be a non-empty array of strings"}), nil
		}

		results := client.BatchSyntaxCheck(ctx, uris)

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
			"total":          len(uris),
			"clean":          clean,
			"total_errors":   totalErrors,
			"total_warnings": totalWarnings,
			"results":        results,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
