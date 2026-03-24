package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSourceTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("get_source",
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
		out, _ := json.Marshal(map[string]string{
			"source": result.Source,
			"etag":   result.ETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("set_source",
		mcp.WithDescription("Write ABAP source code to SAP. Requires the ETag returned by get_source and the lock handle from lock_object."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("New ABAP source code"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle from lock_object (required on most systems)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("etag",
			mcp.Required(),
			mcp.Description("ETag value from get_source, passed verbatim including quotes"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		source := req.GetString("source", "")
		lockHandle := req.GetString("lock_handle", "")
		transport := req.GetString("transport", "")
		etag := req.GetString("etag", "")
		if _, err := client.SetSource(ctx, uri, source, lockHandle, transport, etag); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Source updated successfully"), nil
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
