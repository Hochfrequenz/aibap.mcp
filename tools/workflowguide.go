package tools

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
)

// WorkflowGuideURI is the resource that tells a client how to organise a
// development project with this server: where the source of truth lives, how
// code gets into SAP and back out, and how it is delivered. The server
// instructions point every session at it (#604).
const WorkflowGuideURI = "sap-adt://guides/workflow"

// defaultWorkflowGuide is the public, neutral guide. A downstream build can
// replace it under the same URI with WithWorkflowGuide, so the pointer in the
// server instructions is valid in every build.
//
//go:embed guides/workflow.md
var defaultWorkflowGuide string

// WithWorkflowGuide replaces the text of the workflow guide resource. An empty
// text keeps the public default: it is the zero value of the package-level
// variable a downstream build sets, and serving an empty guide would be worse
// than serving the default.
func WithWorkflowGuide(text string) RegisterOption {
	return func(rs *registerSettings) {
		if text != "" {
			rs.workflowGuide = text
		}
	}
}

// registerWorkflowGuide registers the guide as a resource. Unlike the
// debugging guide it belongs to no tool group: it describes the workflow
// across all of them, and the instructions refer to it unconditionally.
func registerWorkflowGuide(s resourceAdder, text string) {
	s.AddResource(
		mcp.NewResource(WorkflowGuideURI, "Development workflow guide",
			mcp.WithResourceDescription("How to organise an ABAP development project with this server: a decision tree (self-contained or embedded change; ADT, SAP GUI or no access), the write–check–export–review loop, delivery routes, and the tool limits that matter. Read before starting a project."),
			mcp.WithMIMEType("text/markdown"),
		),
		func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{mcp.TextResourceContents{
				URI:      WorkflowGuideURI,
				MIMEType: "text/markdown",
				Text:     text,
			}}, nil
		},
	)
}
