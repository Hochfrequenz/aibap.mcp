package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Elicitor is the narrow interface ConfirmDestructive needs.
// *server.MCPServer satisfies it via its RequestElicitation method; tests
// can pass a stub.
type Elicitor interface {
	RequestElicitation(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
}

// ConfirmDestructive asks the client to confirm a destructive operation via
// MCP elicitation. Returns (true, "") when the operation should proceed, or
// (false, reason) when the user declined/cancelled.
//
// When the client does not support elicitation (ErrElicitationNotSupported),
// no session is active (ErrNoActiveSession), or the elicitor is nil, the
// helper returns (true, "") so behaviour matches today's stock binary
// (destructive tools proceed unconditionally).
func ConfirmDestructive(ctx context.Context, el Elicitor, message string) (bool, string) {
	if el == nil {
		return true, ""
	}
	req := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: message,
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Set true to proceed, false to abort.",
					},
				},
				"required": []string{"confirm"},
			},
		},
	}
	result, err := el.RequestElicitation(ctx, req)
	if err != nil {
		if errors.Is(err, server.ErrElicitationNotSupported) || errors.Is(err, server.ErrNoActiveSession) {
			return true, ""
		}
		return false, fmt.Sprintf("elicitation failed: %v", err)
	}
	switch result.Action {
	case mcp.ElicitationResponseActionDecline:
		return false, "user declined the confirmation"
	case mcp.ElicitationResponseActionCancel:
		return false, "user cancelled the confirmation"
	case mcp.ElicitationResponseActionAccept:
		content, ok := result.Content.(map[string]any)
		if !ok {
			return false, "unexpected elicitation response shape"
		}
		confirm, _ := content["confirm"].(bool)
		if !confirm {
			return false, "user set confirm=false"
		}
		return true, ""
	default:
		return false, fmt.Sprintf("unknown elicitation action: %s", result.Action)
	}
}
