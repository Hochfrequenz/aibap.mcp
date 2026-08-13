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

// DestructiveConfirmationNote is appended to the description of every tool
// that routes through ConfirmDestructive, so a caller reading tools/list can
// see that a confirmation is requested and that its outcome is the client's.
// Guarded by TestGuardedToolDescriptionsCarryTheConfirmationNote, which keeps
// the set of tools carrying it identical to the set that actually confirms.
//
// The second sentence is not padding: on this server's stdio transport the
// request goes out regardless of what the client declared during initialize
// (see ConfirmDestructive below), and a client with no elicitation support
// commonly answers with a decline no human ever saw. Promising a prompt
// unconditionally would be wrong in exactly that case — see issue #475.
const DestructiveConfirmationNote = "\n\nConfirmation: the MCP client is asked to confirm before this runs. " +
	"Clients that support elicitation prompt the user; clients that do not answer on their " +
	"own, usually refusing — so an abort here does not necessarily mean a person declined."

// ConfirmDestructive asks the client to confirm a destructive operation via
// MCP elicitation. Returns (true, "") when the operation should proceed, or
// (false, reason) when the confirmation was declined/cancelled.
//
// The elicitation request is sent to any client with an active session,
// whether or not that client declared the elicitation capability during
// initialize: (*server.MCPServer).RequestElicitation only type-asserts the
// session to server.SessionWithElicitation, and mcp-go's stdioSession
// satisfies that unconditionally. A client without elicitation support
// therefore does not land in the ErrElicitationNotSupported branch below —
// whatever it answers decides the outcome, and an observed answer is a
// well-formed decline that blocks the operation with no prompt shown
// (issue #475).
//
// The helper returns (true, "") — proceed, no confirmation — only when the
// elicitor is nil (RegisterAll; the shipped main.go wires the server itself),
// no session is active (ErrNoActiveSession), or the elicitation-unsupported
// error does arrive on some future transport that checks the capability.
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
	// Guard against upstream returning (nil, nil) — observed occasionally with
	// some transports. Treat as "do not proceed" rather than panicking on
	// result.Action below.
	if result == nil {
		return false, "elicitation returned nil result"
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
