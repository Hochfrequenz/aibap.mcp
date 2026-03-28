package tools

import (
	"context"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
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

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	RegisterAllWithLockMap(s, client, selector, adt.NewLockMap())
}

// toolAdder is the subset of server.MCPServer used by register functions.
type toolAdder interface {
	AddTool(tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error))
}

// loggingServer wraps MCPServer.AddTool to inject logging middleware.
type loggingServer struct {
	inner    *server.MCPServer
	selector SystemSelector
}

func (ls *loggingServer) AddTool(tool mcp.Tool, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	ls.inner.AddTool(tool, withLogging(tool.Name, ls.selector, handler))
}

// RegisterAllWithLockMap registers all SAP ADT MCP tools using a provided lock map.
// Use this when you need to pre-populate or inspect the lock map (e.g. in tests).
func RegisterAllWithLockMap(s *server.MCPServer, client adt.Client, selector SystemSelector, lockMap *adt.LockMap) {
	ls := &loggingServer{inner: s, selector: selector}
	registerSourceTools(ls, client, lockMap, selector)
	registerActivateTools(ls, client)
	registerSearchTools(ls, client)
	registerRepositoryTools(ls, client)
	registerSyntaxCheckTools(ls, client)
	registerUnitTestTools(ls, client)
	registerTransportTools(ls, client)
	registerLockTools(ls, client, lockMap, selector)
	registerPatchTools(ls, client, lockMap, selector)
	registerPrettyPrinterTools(ls, client)
	registerObjectTools(ls, client)
	registerCompletionTools(ls, client)
	registerSystemTools(ls, selector)
	registerATCTools(ls, client)
	registerFileSourceTools(ls, client, lockMap, selector)
	registerDebuggerTools(ls, client, selector)
	registerExportTools(ls, client)
	registerCustomizingTools(ls, client)
	registerVerifyTools(ls, client)
	registerDocuTools(ls, client)
	registerNavigationTools(ls, client)
	registerRefactoringTools(ls, client)
	registerDDICTools(ls, client)
}
