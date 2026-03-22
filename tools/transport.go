package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTransportTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("get_transport_requests",
		mcp.WithDescription("List CTS transport requests on the configured SAP system. Status: D=modifiable, L=released."),
		mcp.WithString("user", mcp.Description("Filter by owner username")),
		mcp.WithString("status", mcp.Description("Filter by status: D (modifiable) or L (released)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		status := req.GetString("status", "")
		transports, err := client.GetTransportRequests(ctx, user, status)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(transports)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("add_to_transport",
		mcp.WithDescription("Assign an ABAP object to a CTS transport request."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number, e.g. DEVK900123")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("object_uri", "")
		transport := req.GetString("transport", "")
		if err := client.AddToTransport(ctx, uri, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object added to transport successfully"), nil
	})
}
