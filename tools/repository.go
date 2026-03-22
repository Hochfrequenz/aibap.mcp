package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRepositoryTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("browse_package",
		mcp.WithDescription("List all ABAP objects in a package."),
		mcp.WithString("package_name", mcp.Required(), mcp.Description("Package name, e.g. ZPACKAGE")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg := req.GetString("package_name", "")
		results, err := client.BrowsePackage(ctx, pkg)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_object_info",
		mcp.WithDescription("Get metadata for an ABAP repository object."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString("object_uri", "")
		info, err := client.GetObjectInfo(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(info)
		return mcp.NewToolResultText(string(out)), nil
	})
}
