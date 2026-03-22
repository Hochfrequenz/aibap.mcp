# SAP ADT MCP Server — Design Spec

**Date:** 2026-03-22
**Status:** Approved

## Overview

A Go-based MCP (Model Context Protocol) server that exposes SAP ABAP Development Tools (ADT) REST API functionality as MCP tools. Enables AI assistants like Claude to read, write, and manage ABAP source code and development objects on SAP systems.

## Architecture

**Approach:** Thin Proxy — the MCP server is a lightweight layer over the SAP ADT REST API. ADT concepts are mapped directly to MCP tools without an intermediate domain abstraction layer.

**Data Flow:**
```
Claude → MCP (stdio) → tools/*.go → adt/*.go → HTTP → SAP ADT REST API
```

- `adt/` handles HTTP: building requests, parsing responses (XML). All files in `adt/` belong to package `adt`.
- `tools/` translates MCP parameters → `adt.Client` calls → MCP responses
- `config/` is loaded once at startup and passed as `*Config`

## Project Structure

```
sapadt.mcp/
├── main.go                    # Entry point, MCP server setup
├── config/
│   └── config.go              # Config loading (file + env override)
├── adt/
│   ├── client.go              # HTTP client: Basic Auth, CSRF token, session re-auth; adt.Client interface
│   ├── types.go               # Shared XML struct types and result types for ADT responses
│   ├── source.go              # Read/write source code
│   ├── activate.go            # Activate objects
│   ├── search.go              # Search objects (QuickSearch, Where-used)
│   ├── repository.go          # Browse repository (packages, objects)
│   ├── syntaxcheck.go         # Syntax check
│   ├── unittest.go            # Run ABAP Unit Tests
│   └── transport.go           # Transport/Workbench requests
├── tools/
│   ├── register.go            # Register all MCP tools: func RegisterAll(s *mcp.Server, client adt.Client)
│   ├── source.go              # MCP tool handlers for source
│   ├── activate.go
│   ├── search.go              # includes where_used tool handler
│   ├── repository.go
│   ├── syntaxcheck.go
│   ├── unittest.go
│   └── transport.go
├── testdata/                  # SAP ADT XML response fixtures
├── config.yaml.example        # Example config (no credentials, committed to VCS)
├── Makefile                   # Build targets
├── .goreleaser.yaml           # GoReleaser config for releases
├── .gitignore                 # Includes config.yaml
├── .github/
│   └── workflows/
│       └── release.yml        # GitHub Actions release workflow
└── go.mod                     # module github.com/dachner/sapadt-mcp
```

## Configuration

**`config.yaml.example`** (committed to VCS — no real credentials):
```yaml
sap:
  host: "https://your-sap-system:8000"
  client: "100"
  user: "DEVELOPER"
  password: ""
  tls_skip_verify: false       # set true for self-signed SAP certificates (dev only)
```

**`config.yaml`** (local only, listed in `.gitignore`) holds actual credentials.

**Environment variables** (override config file):
```
SAP_HOST              # SAP system URL
SAP_CLIENT            # SAP client/mandant
SAP_USER              # SAP username
SAP_PASSWORD          # SAP password
SAP_TLS_SKIP_VERIFY   # "true" to skip TLS verification (dev/internal CAs)
SAP_CONFIG_FILE       # Path to config.yaml (default: ./config.yaml, resolved from process working directory)
```

**Priority:** env vars > config file > defaults

**TLS:** TLS verification is enforced by default. Set `tls_skip_verify: true` or `SAP_TLS_SKIP_VERIFY=true` for internal SAP systems with self-signed certificates.

## ADT Client Design

### CSRF Token Handling

SAP ADT requires a CSRF token for all mutating operations (POST, PUT, DELETE). The `client.go` is responsible for fetching and caching this token:

1. On first mutating request, send a preflight GET to `/sap/bc/adt/compatibility/product` with `X-CSRF-Token: Fetch`
2. Cache the returned token and session cookie
3. Include `X-CSRF-Token: <token>` in all subsequent mutating requests
4. On 403 response (token expired), invalidate token and retry once (re-fetch cycle)

### Session Handling

Long-running processes may encounter expired SAP sessions. `client.go` handles this by:
- Detecting 401 responses and re-authenticating with Basic Auth
- Re-fetching the CSRF token after re-authentication
- Retrying the original request once

### Timeouts and Cancellation

- `adt/client.go` creates the underlying `http.Client` with a configurable `Timeout` (default: 30 seconds)
- `run_unit_tests` uses a per-request context derived from the tool's `timeout_seconds` parameter: `ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)`. The HTTP client timeout for unit test calls is set to `timeout_seconds + 5` seconds to allow for SAP processing overhead.
- All other tool handlers pass the MCP-provided context through to the HTTP client without adding a deadline (the global `http.Client.Timeout` applies).

### `adt.Client` Interface

Defined in `adt/client.go`. `tools/` depends only on this interface, enabling mock-based testing:

```go
type Client interface {
    GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
    SetSource(ctx context.Context, objectURI, source, etag string) error
    ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error)
    SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error)
    WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error)
    BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error)
    GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error)
    SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error)
    RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error)
    GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error)
    AddToTransport(ctx context.Context, objectURI, transport string) error
}
```

### XML Parsing

SAP ADT responses are almost entirely XML. All shared result types (`SourceResult`, `ObjectInfo`, `ActivationResult`, `SyntaxMessage`, `TestResult`, `TransportRequest`) are defined in `adt/types.go`. Each `adt/*.go` file may define its own private XML structs for endpoint-specific shapes.

### Version Variable

Declared in `main.go` as: `var version = "dev"` — embedded at build time via `-X main.version=<tag>`.

## MCP Tools

### `object_uri` Format

ADT object URIs are absolute paths on the SAP server, e.g.:
- `/sap/bc/adt/programs/programs/ZREPORT` (ABAP program)
- `/sap/bc/adt/classes/classes/ZCL_EXAMPLE` (ABAP class)
- `/sap/bc/adt/functions/groups/ZFUGR/fmodules/ZFUNC` (function module)

`tools/` passes `object_uri` as-is to `adt/`; the client prepends the configured host.

### Tool Table

| Tool | Parameters | Description | Returns |
|------|------------|-------------|---------|
| `get_source` | `object_uri` | Read ABAP source code | `{ source: string, etag: string }` |
| `set_source` | `object_uri`, `source`, `etag` | Write ABAP source code | success/error |
| `activate_object` | `object_uri` | Activate an ABAP object | `ActivationResult` |
| `search_objects` | `query`, `object_type?`, `max_results?` | Quick search for objects | `[]ObjectInfo` |
| `where_used` | `object_uri` | Find all usages of an object | `[]ObjectInfo` |
| `browse_package` | `package_name` | List package contents | `[]ObjectInfo` |
| `get_object_info` | `object_uri` | Get object metadata | `ObjectInfo` |
| `syntax_check` | `object_uri` | Run syntax check | `[]SyntaxMessage` |
| `run_unit_tests` | `object_uri`, `timeout_seconds?` (default: 30) | Run ABAP Unit Tests | `TestResult` |
| `get_transport_requests` | `user?`, `status?` | List transport requests on the configured SAP system | `[]TransportRequest` |
| `add_to_transport` | `object_uri`, `transport` | Assign object to transport | success/error |

**Notes:**
- `get_transport_requests` returns transports for the configured SAP system only (DEV/QA/PRD scope is implicit).
- `status` values for `get_transport_requests`: `D` = modifiable, `L` = released.

### ETag Workflow for Source Editing

1. Call `get_source` → returns `{ source, etag }`
2. Modify source text
3. Call `set_source` with the `etag` value **verbatim** (including surrounding quotes if present, e.g. `"abc123"`) — passed as the `If-Match` HTTP header to SAP

### ActivationResult

SAP activation can partially succeed (some objects in a dependency chain may fail). `ActivationResult` contains:
```go
type ActivationResult struct {
    Success  bool
    Messages []ActivationMessage  // per-object status from ADT activation response XML
}
```

### MCP Error Format

`tools/` returns errors using `mcp.CallToolResult` with `IsError: true`. The error content includes the ADT HTTP status code and the parsed SAP error message text — no raw Go stack traces.

**Transport:** stdio (standard for local Claude Code / Desktop integration)
**Authentication:** HTTP Basic Auth with SAP user credentials

## Error Handling

SAP ADT returns XML error responses. `client.go` parses these and returns typed Go errors. `tools/` converts them to `mcp.CallToolResult{IsError: true}` with the SAP error message text and HTTP status code surfaced to the AI client.

## Testing Strategy

- **`adt/` packages:** Unit tests using `httptest.Server` — no real SAP system required
- **`tools/` packages:** Unit tests using a mock `adt.Client` interface
- **`testdata/`:** SAP ADT XML response fixtures for realistic test scenarios

## Build & Distribution

- **Go module:** `github.com/dachner/sapadt-mcp`
- **`Makefile`** targets: `build`, `build-all` (cross-compile Windows/Linux/macOS), `test`, `lint`, `release`
- **GoReleaser** (`.goreleaser.yaml`) for automated multi-platform releases
- **GitHub Actions** (`.github/workflows/release.yml`) triggers release on git tag push
- Binary name: `sapadt-mcp` (/ `sapadt-mcp.exe` on Windows)
- Version embedded at build time: `-X main.version=$(git describe --tags --always)`
- `golangci-lint` integrated in `lint` Makefile target and CI

## Dependencies

- [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) — MCP protocol implementation
- Standard library only for everything else (no HTTP framework needed)
