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

// withStringOrArray defines a JSON schema parameter accepting either a single string
// or an array of strings via a oneOf schema.
func withStringOrArray(name string, opts ...mcp.PropertyOption) mcp.ToolOption {
	return func(t *mcp.Tool) {
		schema := map[string]any{
			"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		}
		for _, opt := range opts {
			opt(schema)
		}
		if required, ok := schema["required"].(bool); ok && required {
			delete(schema, "required")
			t.InputSchema.Required = append(t.InputSchema.Required, name)
		}
		t.InputSchema.Properties[name] = schema
	}
}

// getStringOrSlice extracts a parameter from args that can be either a string or a []any (JSON array).
// Returns (single string, nil) for string input, or ("", []string) for array input.
func getStringOrSlice(args map[string]any, key string) (string, []string) {
	val, ok := args[key]
	if !ok {
		return "", nil
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	if arr, ok := val.([]any); ok {
		strs := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				strs = append(strs, s)
			}
		}
		return "", strs
	}
	return "", nil
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
	registerVersionTools(ls, client)
	registerTextElementTools(ls, client)
	registerMessageClassTools(ls, client)
	registerQueryTools(ls, client)
	registerRollbackTools(ls, client)
	registerEnhancementTools(ls, client)
	registerShortDumpTools(ls, client)
}
