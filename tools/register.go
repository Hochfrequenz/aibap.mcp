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

// AllGroups lists every tool group name.
var AllGroups = []string{
	"source", "code-intelligence", "objects", "version", "locking",
	"testing", "messages", "shortdumps", "transport", "enhancements",
	"debug", "export", "system",
}

var defaultOffGroups = map[string]bool{
	"debug":  true,
	"export": true,
}

// DefaultGroups returns the default enabled/disabled state for each group.
func DefaultGroups() map[string]bool {
	groups := make(map[string]bool, len(AllGroups))
	for _, g := range AllGroups {
		groups[g] = !defaultOffGroups[g]
	}
	return groups
}

// ParseToolGroups converts a list of group names to an enabled map.
// If names is empty, DefaultGroups is returned.
// The special name "all" enables every group.
func ParseToolGroups(names []string) map[string]bool {
	if len(names) == 0 {
		return DefaultGroups()
	}
	groups := make(map[string]bool, len(AllGroups))
	for _, name := range names {
		if name == "all" {
			for _, g := range AllGroups {
				groups[g] = true
			}
			return groups
		}
		groups[name] = true
	}
	return groups
}

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	RegisterAllWithLockMap(s, client, selector, adt.NewLockMap(), DefaultGroups())
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

// RegisterAllWithLockMap registers SAP ADT MCP tools using a provided lock map
// and an enabledGroups map controlling which tool groups are active.
// Use this when you need to pre-populate or inspect the lock map (e.g. in tests).
func RegisterAllWithLockMap(s *server.MCPServer, client adt.Client, selector SystemSelector, lockMap *adt.LockMap, enabledGroups map[string]bool) {
	ls := &loggingServer{inner: s, selector: selector}

	type group struct {
		name     string
		register func()
	}
	groups := []group{
		{"source", func() {
			registerSourceTools(ls, client, lockMap, selector)
			registerPatchTools(ls, client, lockMap, selector)
			registerFileSourceTools(ls, client, lockMap, selector)
			registerPrettyPrinterTools(ls, client)
		}},
		{"code-intelligence", func() {
			registerCompletionTools(ls, client)
			registerNavigationTools(ls, client)
			registerDocuTools(ls, client)
			registerVerifyTools(ls, client)
		}},
		{"objects", func() {
			registerSearchTools(ls, client)
			registerRepositoryTools(ls, client)
			registerObjectTools(ls, client)
			registerRefactoringTools(ls, client)
			registerDDICTools(ls, client)
		}},
		{"version", func() { registerVersionTools(ls, client) }},
		{"locking", func() {
			registerLockTools(ls, client, lockMap, selector)
			registerActivateTools(ls, client)
		}},
		{"testing", func() {
			registerSyntaxCheckTools(ls, client)
			registerUnitTestTools(ls, client)
			registerATCTools(ls, client)
		}},
		{"messages", func() {
			registerMessageClassTools(ls, client)
			registerTextElementTools(ls, client)
		}},
		{"shortdumps", func() { registerShortDumpTools(ls, client) }},
		{"transport", func() {
			registerTransportTools(ls, client)
			registerRollbackTools(ls, client)
		}},
		{"enhancements", func() { registerEnhancementTools(ls, client) }},
		{"debug", func() { registerDebuggerTools(ls, client, selector) }},
		{"export", func() {
			registerExportTools(ls, client)
			registerCustomizingTools(ls, client)
		}},
		{"system", func() {
			registerSystemTools(ls, selector)
			registerQueryTools(ls, client)
		}},
	}

	for _, g := range groups {
		if enabledGroups[g.name] {
			g.register()
		}
	}
}
