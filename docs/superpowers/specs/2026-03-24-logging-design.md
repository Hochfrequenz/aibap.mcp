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
| Papertrail | TLS syslog | JSON | when `PAPERTRAIL_HOST` + `PAPERTRAIL_PORT` are set |

## Configuration

Environment variables only (no config.yaml changes):

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_FORMAT` | `text` | `text` or `json` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PAPERTRAIL_HOST` | (empty) | Papertrail syslog host, e.g. `logs.papertrailapp.com` |
| `PAPERTRAIL_PORT` | (empty) | Papertrail syslog port, e.g. `12345` |

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
