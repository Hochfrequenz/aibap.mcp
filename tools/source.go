package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerSourceTools(s toolAdder, client adt.SourceClient, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("get_source",
		mcp.WithTitleAnnotation("Get ABAP Source Code"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read ABAP source code from SAP. Returns source text and ETag for optimistic locking."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(adt.LockKey(selector.ActiveName(), uri), result.ETag)
		out, _ := json.Marshal(map[string]string{
			"source": result.Source,
			"etag":   result.ETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}

// errorResult converts an error to an MCP error result with the SAP error message.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
	}
}
