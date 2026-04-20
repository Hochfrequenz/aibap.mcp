package logging

import (
	"log/slog"
	"os"
	"runtime/debug"
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

// defaultCommit is populated via `-ldflags -X` by GoReleaser with the short
// commit SHA of the build, so release binaries carry a deterministic commit
// even when runtime/debug.ReadBuildInfo() VCS settings are absent or stripped
// (which can happen in CI builds without a `.git` directory). Plain `go build`
// leaves it empty and BuildInfo() falls back to the runtime/debug path.
//
// Same link-time-constant rule as the Papertrail vars: do NOT mutate at
// runtime outside of tests.
var defaultCommit = ""

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
//
// The version argument and the commit short-SHA from BuildInfo are attached
// as default attributes on the root logger so every log line carries both
// fields without per-call wiring. This lets us identify which build a remote
// user is running from a single log entry in their bug report.
func Setup(version string) {
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

	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = newFanoutHandler(handlers...)
	}

	logger := slog.New(handler).With(
		"version", version,
		"commit", BuildInfo(),
		"remote_logging", remoteLoggingAttr(),
	)
	slog.SetDefault(logger)
}

// RemoteLoggingBakedIn reports whether this binary was built with a
// Papertrail destination compiled in via -ldflags -X. True only for the
// `mcp-server-abap-with-remote-logging-*` release archives built by
// GoReleaser; false for source builds, the Docker image, and the default
// `mcp-server-abap-*` release archives. Callers should display this in
// `--version` output and log lines so bug reports unambiguously identify
// which variant a user is running.
//
// This does not reflect the runtime Papertrail state — a user of the
// silent binary can still enable Papertrail by setting PAPERTRAIL_HOST
// and PAPERTRAIL_PORT, and a user of the telemetry binary can disable
// it by setting PAPERTRAIL_HOST= (explicit empty). Those cases are
// captured by resolvePapertrail, not here.
func RemoteLoggingBakedIn() bool {
	return defaultPapertrailHost != "" && defaultPapertrailPort != ""
}

func remoteLoggingAttr() string {
	if RemoteLoggingBakedIn() {
		return "on"
	}
	return "off"
}

// CommitUnknown is returned by BuildInfo when no VCS metadata is embedded —
// e.g. plain `go build` outside a git checkout, or a build whose toolchain
// stripped the build settings. Exported so callers and tests can compare
// against the same sentinel.
const CommitUnknown = "unknown"

// BuildInfo returns the binary's commit identifier. Resolution order:
//
//  1. The link-time defaultCommit var, if set by GoReleaser via -ldflags -X.
//     This is the only source that is guaranteed to be present in release
//     binaries — runtime/debug VCS settings can be silently absent in CI
//     builds without a `.git` checkout.
//  2. The vcs.revision build setting from runtime/debug.ReadBuildInfo,
//     truncated to a 7-char short SHA, optionally suffixed with "+dirty"
//     when vcs.modified=true. This covers `go build` from a working tree.
//  3. CommitUnknown when neither source has a value.
func BuildInfo() string {
	if defaultCommit != "" {
		return defaultCommit
	}
	return commitFromBuildSettings(debug.ReadBuildInfo)
}

// commitFromBuildSettings extracts the short SHA from runtime/debug build
// settings. Split out from BuildInfo so unit tests can inject a fake reader
// and exercise both the VCS-present and VCS-absent code paths deterministically
// without depending on how the test binary was linked.
func commitFromBuildSettings(read func() (*debug.BuildInfo, bool)) string {
	info, ok := read()
	if !ok {
		return CommitUnknown
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return CommitUnknown
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified == "true" {
		rev += "+dirty"
	}
	return rev
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
