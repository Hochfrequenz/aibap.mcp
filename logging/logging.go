package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Compile-time Papertrail defaults, populated via `-ldflags -X` at link time
// for the binaries published on GitHub Releases (see .goreleaser.yaml). They
// are empty in normal builds (`go build`, `make build`, `go install`,
// Dockerfile) so Papertrail stays off unless the user opts in via env vars.
//
// These vars are link-time constants in production. Do NOT mutate them at
// runtime in non-test code. Tests may swap them via the withPapertrailDefaults
// helper in logging_test.go, which restores the prior values in t.Cleanup.
var (
	defaultPapertrailHost = ""
	defaultPapertrailPort = ""
)

// Setup configures the global slog logger based on environment variables:
//
//   - LOG_FORMAT: "text" (default) or "json"
//   - LOG_LEVEL: "debug", "info" (default), "warn", "error"
//   - PAPERTRAIL_HOST + PAPERTRAIL_PORT: if either env var is set (even to
//     empty), both values come from the environment — pair-wise override.
//     If neither is set, both come from the compile-time defaults
//     (defaultPapertrailHost / defaultPapertrailPort). The TLS syslog handler
//     is added only when the resolved host AND port are both non-empty.
//     The pair-wise rule prevents accidental delivery to the wrong Papertrail
//     account if a release-binary user sets only one of the two env vars.
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

	// Papertrail handler (optional). Pair-wise override: if either env var is
	// explicitly set, both must be set; otherwise fall through to defaults.
	ptHost, ptPort := resolvePapertrail()
	if ptHost != "" && ptPort != "" {
		handlers = append(handlers, newPapertrailHandler(ptHost, ptPort, level))
	}

	if len(handlers) == 1 {
		slog.SetDefault(slog.New(handlers[0]))
	} else {
		slog.SetDefault(slog.New(newFanoutHandler(handlers...)))
	}
}

// resolvePapertrail picks the Papertrail destination using pair-wise override
// semantics. If either PAPERTRAIL_HOST or PAPERTRAIL_PORT is explicitly set
// (even to an empty string), the user is overriding the compile-time defaults
// and both values come from the environment. If neither is set, both come
// from the compile-time defaults.
func resolvePapertrail() (host, port string) {
	hostEnv, hostSet := os.LookupEnv("PAPERTRAIL_HOST")
	portEnv, portSet := os.LookupEnv("PAPERTRAIL_PORT")
	if hostSet || portSet {
		return hostEnv, portEnv
	}
	return defaultPapertrailHost, defaultPapertrailPort
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
