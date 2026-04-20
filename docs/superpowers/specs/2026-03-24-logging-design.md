# Structured Logging

**Date:** 2026-03-24
**Issue:** #47

## Philosophy

Follow loggingsucks.com: fewer, richer events. One canonical log event per tool call that captures everything needed to understand what happened. No scattered debug lines.

## Events

### Startup event (once)

```json
{"level":"INFO","msg":"server started","version":"1.2.3","systems":["dev","prod"],"default_system":"dev"}
```

### Tool call event (one per MCP tool invocation)

```json
{"level":"INFO","msg":"tool call","tool":"get_source","system":"dev","object_uri":"/sap/bc/adt/programs/programs/ZREPORT","duration_ms":142,"status":"ok"}
```

On error:

```json
{"level":"ERROR","msg":"tool call","tool":"lock_object","system":"dev","object_uri":"/sap/bc/adt/programs/programs/ZREPORT","duration_ms":89,"status":"error","error":"SAP ADT error 406: Not Acceptable"}
```

That's it. Two event types.

## Handlers

Multiple handlers, same event goes to all:

| Handler | Destination | Format | When |
|---------|-------------|--------|------|
| stderr | local terminal | text (default) or JSON (`LOG_FORMAT=json`) | always |
| Papertrail | TLS syslog | JSON | when `PAPERTRAIL_HOST` + `PAPERTRAIL_PORT` resolve to non-empty values (env or compile-time defaults — see below) |

## Configuration

Environment variables only (no config.yaml changes):

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_FORMAT` | `text` | `text` or `json` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PAPERTRAIL_HOST` | (empty) <sup>1</sup> | Papertrail syslog host, e.g. `logs5.papertrailapp.com` |
| `PAPERTRAIL_PORT` | (empty) <sup>1</sup> | Papertrail syslog port, e.g. `35329` |

<sup>1</sup> Release binaries published via goreleaser bake in a default destination (see "Compile-time defaults" below).

### Compile-time defaults

`logging/logging.go` declares two package-level vars:

```go
var (
    defaultPapertrailHost = ""
    defaultPapertrailPort = ""
)
```

`goreleaser` injects values via `-ldflags -X` only for the **`-with-remote-logging`** release-binary flavour, so those pre-built downloads ship with `logs5.papertrailapp.com:35329` enabled by default. The default-flavour release binary, source builds (`go build`, `make build`, `go install`), and the Docker image all leave the vars empty, so Papertrail stays off. See #329 for the split-binary rationale.

### Pair-wise override semantics

`Setup()` resolves the destination using pair-wise override:

| `PAPERTRAIL_HOST` env | `PAPERTRAIL_PORT` env | Result                                |
|-----------------------|-----------------------|---------------------------------------|
| unset                 | unset                 | both fall back to compile-time defaults |
| set (any value)       | unset                 | both come from env (port empty → off) |
| unset                 | set (any value)       | both come from env (host empty → off) |
| set                   | set                   | both come from env                    |

**Why pair-wise?** Without it, a user setting only `PAPERTRAIL_PORT=12345` on a release binary would silently mix their port with the baked-in host and ship logs to the wrong Papertrail account — a real privacy bug. Treating either env var as an explicit override forces the user to be deliberate.

To **disable** the baked-in default in a release binary, set `PAPERTRAIL_HOST=` (explicit empty). The pair-wise rule then makes both values empty and the handler is not added.

## Implementation

### Library

`log/slog` (stdlib since Go 1.21, zero new dependencies).

### Files

| File | Purpose |
|------|---------|
| `logging/logging.go` | `Setup()` function: creates handlers, sets `slog.SetDefault()` |
| `logging/papertrail.go` | TLS syslog handler for Papertrail |
| `logging/fanout.go` | Multi-handler that writes to all handlers |
| `logging/logging_test.go` | Tests for setup, fanout, format switching |
| `logging/papertrail_test.go` | Tests for TLS syslog formatting |
| `tools/middleware.go` | Logging middleware that wraps tool handlers |
| `main.go` | Call `logging.Setup()` before server start, log startup event |

### Middleware approach

Instead of adding logging to every tool handler, wrap the MCP tool handler with a middleware that:
1. Records start time
2. Calls the real handler
3. Logs the canonical event with duration, tool name, status, error

This keeps tool handlers clean — zero logging code in them.

### Papertrail handler

TLS connection to syslog endpoint. BSD syslog format (RFC 3164) over TLS, same as sapwebgui.mcp. Persistent connection with reconnect on failure.

## Scope

- `logging/` package with setup, fanout, papertrail
- Middleware in `tools/` for tool call logging
- Startup log event in `main.go`
- Tests
- No changes to existing tool handlers
