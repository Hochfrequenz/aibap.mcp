package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup configures the global slog logger based on environment variables:
//
//   - LOG_FORMAT: "text" (default) or "json"
//   - LOG_LEVEL: "debug", "info" (default), "warn", "error"
//   - PAPERTRAIL_HOST + PAPERTRAIL_PORT: if both set, adds TLS syslog handler
func Setup() {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	var handlers []slog.Handler

	// Stderr handler (always present).
	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "json") {
		handlers = append(handlers, slog.NewJSONHandler(os.Stderr, opts))
	} else {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, opts))
	}

	// Papertrail handler (optional).
	ptHost := os.Getenv("PAPERTRAIL_HOST")
	ptPort := os.Getenv("PAPERTRAIL_PORT")
	if ptHost != "" && ptPort != "" {
		handlers = append(handlers, newPapertrailHandler(ptHost, ptPort, level))
	}

	if len(handlers) == 1 {
		slog.SetDefault(slog.New(handlers[0]))
	} else {
		slog.SetDefault(slog.New(newFanoutHandler(handlers...)))
	}
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
