package tools

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// withLogging wraps an MCP tool handler to log a canonical event per call.
func withLogging(toolName string, selector SystemSelector, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		result, err := handler(ctx, req)
		duration := time.Since(start).Milliseconds()

		attrs := []slog.Attr{
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
