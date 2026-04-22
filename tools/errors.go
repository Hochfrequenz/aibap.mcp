package tools

import (
	"errors"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolError is the structured error payload attached to the
// StructuredContent field of every error-channel CallToolResult built by
// errorResult. Clients reading the 2025-06-18 MCP wire format can access
// the SAP status code directly instead of parsing the `"Error: "` prefix
// out of the text fallback.
//
// Message always mirrors err.Error() — identical modulo the "Error: "
// prefix to the text content. This preserves any wrap context
// (e.g. "auto-lock failed: SAP ADT error 423: ...") that would otherwise
// be lost by surfacing only the inner adt.ADTError.Message.
//
// StatusCode is populated when the underlying error is (or wraps) an
// adt.ADTError; otherwise it is zero and omitted from the JSON.
type ToolError struct {
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message"`
}

// errorResult converts an error to an MCP error result.
//
// The text content keeps the legacy `"Error: <full error string>"` form
// for clients that only consume text. StructuredContent carries a typed
// ToolError — see the type doc for the semantics.
func errorResult(err error) *mcp.CallToolResult {
	toolErr := ToolError{Message: err.Error()}
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) {
		toolErr.StatusCode = adtErr.StatusCode
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
		StructuredContent: toolErr,
	}
}
