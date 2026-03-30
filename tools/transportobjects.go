package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerTransportObjectTools(s toolAdder, client adt.TransportClient) {
	s.AddTool(mcp.NewTool("get_transport_objects",
		mcp.WithDescription("List all objects recorded in a transport request. Returns the PGMID, object type, and name of each entry. Use this to see what a transport contains before releasing or rolling back."),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number, e.g. HFQK900178")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		objects, err := client.GetTransportObjects(ctx, transport)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(objects)
		return mcp.NewToolResultText(string(out)), nil
	})
}
