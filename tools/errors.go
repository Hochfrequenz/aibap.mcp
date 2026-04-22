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
// the SAP status code and message directly instead of parsing the
// `"Error: "` prefix out of the text fallback.
//
// Status code is populated when the underlying error is (or wraps) an
// adt.ADTError; otherwise it is zero and omitted from the JSON.
type ToolError struct {
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message"`
}

// errorResult converts an error to an MCP error result.
//
// The text content keeps the legacy `"Error: <full error string>"` form
// for clients that only consume text. StructuredContent carries a typed
// ToolError — for adt.ADTError, the SAP status code is split out from
// the message so clients don't have to string-parse it.
func errorResult(err error) *mcp.CallToolResult {
	toolErr := ToolError{Message: err.Error()}
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) {
		toolErr.StatusCode = adtErr.StatusCode
		toolErr.Message = adtErr.Message
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(fmt.Sprintf("Error: %s", err.Error())),
		},
		StructuredContent: toolErr,
	}
}
