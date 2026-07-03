package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
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

// defaultOffGroups lists tool groups that are hidden from MCP clients by
// default. Hidden tools don't appear in the tool list and can't be called.
//
// Background (#230): the MCP server exposes 60+ tools. Each tool definition
// is sent to the client on connect and consumes tokens in the LLM context
// window. Tool groups were introduced to let users control which tools are
// loaded. Groups in this map are off unless explicitly enabled via --tools
// or the config's "tools" array.
//
// The problem is that MCP has no "show me what's hidden" mechanism — from
// the client's perspective, hidden tools simply don't exist. There's no
// discovery, no hint, nothing. So hiding tools here means users can't know
// they're available unless they read this source code or the docs.
//
// "export" was originally in this map alongside "debug" (#230). Removed in
// #303 because the export tools (export_package, export_packages,
// export_customizing) are read-only and production-ready — hiding them
// just caused confusion when users tried to export packages and couldn't
// find the tools.
//
// "debug" stays off because debugger tools (breakpoints, stepping, variable
// inspection) can interfere with other active debugger sessions on the same
// SAP system. They're opt-in by design.
//
// TODO: this whole approach is a stopgap. The proper fix is either MCP-level
// tool categories / lazy loading, or shorter tool descriptions that reduce
// the token footprint without hiding functionality. We don't have a good
// solution for this yet.
var defaultOffGroups = map[string]bool{
	"debug": true,
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
	RegisterAllWithLockMap(s, client, selector, adt.NewLockMap(), DefaultGroups(), nil, nil)
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
// The fallback parameter is optional (nil = no fallback for unsupported operations).
// The elicitor parameter is optional (nil = destructive tools proceed without
// confirmation, matching pre-elicitation behaviour).
func RegisterAllWithLockMap(s *server.MCPServer, client adt.Client, selector SystemSelector, lockMap *adt.LockMap, enabledGroups map[string]bool, fallback BlackMagicClient, elicitor Elicitor) {
	ls := &loggingServer{inner: s, selector: selector}

	// tracker records lock-map keys so reset_session can clear the active
	// system's cached handles after a session drop (adt.LockMap is not
	// enumerable). See #383.
	tracker := newSessionLockTracker()

	type group struct {
		name     string
		register func()
	}
	groups := []group{
		{"source", func() {
			registerSourceTools(ls, client, lockMap, selector)
			registerPatchTools(ls, client, lockMap, tracker, selector)
			registerFileSourceTools(ls, client, lockMap, tracker, selector)
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
			registerObjectTools(ls, client, client, client, fallback, elicitor)
			registerRefactoringTools(ls, client, elicitor)
			registerDDICTools(ls, client)
		}},
		{"version", func() { registerVersionTools(ls, client) }},
		{"locking", func() {
			registerLockTools(ls, client, lockMap, tracker, selector)
			registerResetSessionTool(ls, client, lockMap, tracker, selector)
			registerActivateTools(ls, client, lockMap, selector)
		}},
		{"testing", func() {
			registerSyntaxCheckTools(ls, client)
			registerUnitTestTools(ls, client)
			registerATCTools(ls, client)
		}},
		{"messages", func() {
			registerMessageClassTools(ls, client)
			registerTextElementTools(ls, client, lockMap, tracker, selector)
		}},
		{"shortdumps", func() { registerShortDumpTools(ls, client) }},
		{"transport", func() {
			registerTransportTools(ls, client, fallback, elicitor)
			registerRollbackTools(ls, client, elicitor)
		}},
		{"enhancements", func() { registerEnhancementTools(ls, client) }},
		{"debug", func() { registerDebuggerTools(ls, client, selector) }},
		{"export", func() {
			registerExportTools(ls, client)
			registerCustomizingTools(ls, client)
			registerCustomizingWriteTools(ls, fallback, elicitor)
		}},
		{"system", func() {
			registerSystemTools(ls, selector)
			registerQueryTools(ls, client, elicitor)
		}},
	}

	for _, g := range groups {
		if enabledGroups[g.name] {
			g.register()
		}
	}
}
