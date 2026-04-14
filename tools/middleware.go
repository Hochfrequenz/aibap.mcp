package tools

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
)

// requestIDKey is the context key under which the per-call request ID is
// stored. Unexported to prevent collisions with other packages' ctx keys.
type requestIDKey struct{}

// withRequestID returns a derived context carrying the given request ID.
// A reader (RequestIDFromContext) will be added when the first handler
// needs to include the ID in its own structured log calls — until then the
// value is only consumed by withLogging via the captured local variable.
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// newRequestID returns a UUID v7 string for use as a per-tool-call correlation
// ID. v7 is time-ordered, so log entries sort naturally and IDs from the same
// session cluster together in storage. On the (vanishingly unlikely) failure
// of the underlying random source, we fall back to "no-request-id" rather
// than panicking — logging is best-effort and must never break a tool call.
func newRequestID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return "no-request-id"
	}
	return id.String()
}

// withLogging wraps an MCP tool handler to log a canonical event per call.
func withLogging(toolName string, selector SystemSelector, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		reqID := newRequestID()
		ctx = withRequestID(ctx, reqID)

		start := time.Now()
		result, err := handler(ctx, req)
		duration := time.Since(start).Milliseconds()

		attrs := []slog.Attr{
			slog.String("request_id", reqID),
			slog.String("tool", toolName),
			slog.String("system", selector.ActiveName()),
			slog.Int64("duration_ms", duration),
		}

		// Include object_uri if present in the request.
		if uri := req.GetString(paramObjectURI, ""); uri != "" {
			attrs = append(attrs, slog.String("object_uri", uri))
		}

		if err != nil {
			attrs = append(attrs, slog.String("status", "error"), slog.String("error", err.Error()))
			slog.LogAttrs(ctx, slog.LevelError, "tool call", attrs...)
		} else if result != nil && result.IsError {
			// MCP-level error (returned as result, not Go error).
			attrs = append(attrs, slog.String("status", "error"))
			slog.LogAttrs(ctx, slog.LevelError, "tool call", attrs...)
		} else {
			attrs = append(attrs, slog.String("status", "ok"))
			slog.LogAttrs(ctx, slog.LevelInfo, "tool call", attrs...)
		}

		return result, err
	}
}
