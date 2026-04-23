package tools

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// errorResult converts an error to an MCP error result.
//
// The text content carries the `"Error: <full error string>"` payload that
// every client has historically consumed. StructuredContent is deliberately
// left unset on the error path: MCP 2025-06-18 /server/tools requires
// structuredContent to conform to the declared outputSchema with no
// exemption for isError=true, so a typed error DTO (previously ToolError,
// removed #354) would contradict every tool's declared output shape and
// be rejected by strict clients. Absence is spec-legal; clients extract
// the wrapped SAP status code — if needed — from the `"SAP ADT error N:"`
// prefix produced by adt.ADTError.Error(), which flows into the text
// content untouched.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
	}
}
