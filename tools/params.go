package tools

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// requireString reads a required string parameter from req. It returns the
// trimmed value, or ("", errorResult) when the parameter is absent, not a
// string, or empty/whitespace.
//
// mcp.Required() in a tool schema is only advisory — the MCP server does not
// enforce it — so without this a handler that does req.GetString(name, "")
// forwards "" straight to adtler, and the caller sees whatever obscure error
// the empty value triggers downstream, with no signal that the key itself was
// omitted or misspelled. requireString turns that into a clear, uniform
// message. See #386.
//
// The returned value is trimmed, so do NOT use this for parameters where
// surrounding whitespace is significant (e.g. ABAP source code) — read those
// with req.GetString and validate separately.
//
// Usage:
//
//	transport, errRes := requireString(req, "transport")
//	if errRes != nil {
//		return errRes, nil
//	}
func requireString(req mcp.CallToolRequest, name string) (string, *mcp.CallToolResult) {
	val, err := req.RequireString(name)
	if err != nil {
		// Absent or wrong type: mcp-go's message ("required argument ... not
		// found" / "... is not a string") is preserved via %w and already
		// distinguishes the two, so the prefix here stays neutral.
		return "", errorResult(fmt.Errorf("invalid required parameter %q: %w", name, err))
	}
	if val = strings.TrimSpace(val); val == "" {
		return "", errorResult(fmt.Errorf("required parameter %q must not be empty", name))
	}
	return val, nil
}
