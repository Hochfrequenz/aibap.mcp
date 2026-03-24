package tools

import (
	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/server"
)

// Common parameter names and descriptions used across tool registrations.
const (
	paramObjectURI   = "object_uri"
	descADTObjectURI = "ADT object URI"
)

// SystemSelector can switch the active SAP system.
type SystemSelector interface {
	Select(name string) (string, error)
	ActiveName() string
}

// lockKey returns the key used to store lock state in the lock map.
func lockKey(selector SystemSelector, objectURI string) string {
	return selector.ActiveName() + ":" + objectURI
}

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	RegisterAllWithLockMap(s, client, selector, adt.NewLockMap())
}

// RegisterAllWithLockMap registers all SAP ADT MCP tools using a provided lock map.
// Use this when you need to pre-populate or inspect the lock map (e.g. in tests).
func RegisterAllWithLockMap(s *server.MCPServer, client adt.Client, selector SystemSelector, lockMap *adt.LockMap) {
	registerSourceTools(s, client, lockMap, selector)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
	registerLockTools(s, client, lockMap, selector)
	registerPatchTools(s, client, lockMap, selector)
	registerPrettyPrinterTools(s, client)
	registerObjectTools(s, client)
	registerCompletionTools(s, client)
	registerSystemTools(s, selector)
	registerFileSourceTools(s, client, lockMap, selector)
}
