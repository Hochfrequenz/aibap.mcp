package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

type hintRule struct {
	statusCode  int    // 0 = match any status code
	textPattern string // "" = match any text; checked case-insensitive
	hint        string
}

var hintRules = []hintRule{
	{statusCode: 423, hint: "Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport."},
	{statusCode: 404, hint: "Object not found. Check the URI spelling or use `search_objects` to find it."},
	{statusCode: 403, hint: "Authorization error. Check that the ADT user has the required S_DEVELOP authorizations."},
	{statusCode: 400, textPattern: "transport", hint: "A transport request may be required. Use `create_transport` or `get_transport_requests` to find one."},
	{statusCode: 409, hint: "Object already exists. Use `search_objects` to find it, or choose a different name."},
	{textPattern: "already exists", hint: "Object already exists. Use `search_objects` to find it, or choose a different name."},
	{statusCode: 500, hint: "SAP server error. Retry once — if it persists, check SM21 (system log) or ST22 (short dumps)."},
	{textPattern: "inactive", hint: "Activation failed — dependent objects may be inactive. Use `activate_objects` with all dependencies."},
}

// errorResult converts an error to an MCP error result with the SAP error
// message. If the error matches a known pattern, an actionable hint is
// appended to help the LLM recover.
func errorResult(err error) *mcp.CallToolResult {
	msg := fmt.Sprintf("Error: %s", err.Error())
	if hint := matchHint(err); hint != "" {
		msg += "\n\nHint: " + hint
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(msg),
		},
	}
}

func matchHint(err error) string {
	var adtErr *adt.ADTError
	statusCode := 0
	if errors.As(err, &adtErr) {
		statusCode = adtErr.StatusCode
	}
	errText := strings.ToLower(err.Error())

	for _, rule := range hintRules {
		if rule.statusCode != 0 && rule.statusCode != statusCode {
			continue
		}
		if rule.textPattern != "" && !strings.Contains(errText, strings.ToLower(rule.textPattern)) {
			continue
		}
		return rule.hint
	}
	return ""
}
