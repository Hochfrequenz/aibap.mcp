package tools

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DebuggingGuideURI is the resource that tells a client how to get a program
// to actually stop at a breakpoint: which triggers work, in which order, and
// how to end a session. The debug_* tool descriptions only cover single steps;
// combining them wrongly fails silently (the run times out while the program
// runs to completion). See #559.
const DebuggingGuideURI = "sap-adt://guides/debugging"

// debuggingGuide is embedded so the guide is versioned with the tools it
// describes.
//
//go:embed guides/debugging.md
var debuggingGuide string

// resourceAdder is the subset of server.MCPServer used to register resources.
type resourceAdder interface {
	AddResource(resource mcp.Resource, handler server.ResourceHandlerFunc)
}

// registerDebuggingGuide registers the guide as a resource. It belongs to the
// "debug" group: resources are read on demand, so the guide costs nothing in
// sessions that never debug, and a session without the debug tools does not
// list it.
func registerDebuggingGuide(s resourceAdder) {
	s.AddResource(
		mcp.NewResource(DebuggingGuideURI, "Debugging guide",
			mcp.WithResourceDescription("How a debug run with debug_run and debug_wait works: the triggers (ABAP Unit tests, manual, SAP GUI), run states, breakpoints during a run, and how to end a session. Read before the first debug_run."),
			mcp.WithMIMEType("text/markdown"),
		),
		func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{mcp.TextResourceContents{
				URI:      DebuggingGuideURI,
				MIMEType: "text/markdown",
				Text:     debuggingGuide,
			}}, nil
		},
	)
}
