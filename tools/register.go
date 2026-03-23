package tools

import (
	"github.com/dachner/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client) {
	registerSourceTools(s, client)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
}
