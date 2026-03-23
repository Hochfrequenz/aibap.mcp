package tools

import (
	"github.com/dachner/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/server"
)

// Common parameter names and descriptions used across tool registrations.
const (
	paramObjectURI    = "object_uri"
	descADTObjectURI  = "ADT object URI"
)
// SystemSelector can switch the active SAP system.
type SystemSelector interface {
	Select(name string) (string, error)
	ActiveName() string
}

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	registerSourceTools(s, client)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
	registerSystemTools(s, selector)
}
