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

- `adt/` handles HTTP: building requests, parsing responses (XML/JSON)
- `tools/` translates MCP parameters → `adt/` calls → MCP responses
- `config/` is loaded once at startup and passed as `*Config`

## Project Structure

```
sapadt.mcp/
├── main.go                    # Entry point, MCP server setup
├── config/
│   └── config.go              # Config loading (file + env override)
├── adt/
│   ├── client.go              # HTTP client with Basic Auth, session handling
│   ├── source.go              # Read/write source code
│   ├── activate.go            # Activate objects
│   ├── search.go              # Search objects (QuickSearch, Where-used)
│   ├── repository.go          # Browse repository (packages, objects)
│   ├── syntaxcheck.go         # Syntax check
│   ├── unittest.go            # Run ABAP Unit Tests
│   └── transport.go           # Transport/Workbench requests
├── tools/
│   ├── register.go            # Register all MCP tools
│   ├── source.go              # MCP tool handlers for source
│   ├── activate.go
│   ├── search.go
│   ├── repository.go
│   ├── syntaxcheck.go
│   ├── unittest.go
│   └── transport.go
├── testdata/                  # SAP ADT XML response fixtures
├── config.yaml                # Example configuration file
├── Makefile                   # Build targets
├── .goreleaser.yaml           # GoReleaser config for releases
├── .github/
│   └── workflows/
│       └── release.yml        # GitHub Actions release workflow
└── go.mod
```

## Configuration

**`config.yaml`:**
```yaml
sap:
  host: "https://your-sap-system:8000"
  client: "100"
  user: "DEVELOPER"
  password: "secret"
```

**Environment variables** (override config file):
```
SAP_HOST          # SAP system URL
SAP_CLIENT        # SAP client/mandant
SAP_USER          # SAP username
SAP_PASSWORD      # SAP password
SAP_CONFIG_FILE   # Path to config.yaml (default: ./config.yaml)
```

**Priority:** env vars > config file > defaults

## MCP Tools

| Tool | Parameters | Description |
|------|------------|-------------|
| `get_source` | `object_uri` | Read ABAP source code |
| `set_source` | `object_uri`, `source`, `etag` | Write ABAP source code |
| `activate_object` | `object_uri` | Activate an ABAP object |
| `search_objects` | `query`, `object_type?`, `max_results?` | Quick search for objects |
| `browse_package` | `package_name` | List package contents |
| `get_object_info` | `object_uri` | Get object metadata |
| `syntax_check` | `object_uri` | Run syntax check |
| `run_unit_tests` | `object_uri` | Run ABAP Unit Tests |
| `get_transport_requests` | `user?`, `status?` | List transport requests |
| `add_to_transport` | `object_uri`, `transport` | Assign object to transport |

**Transport:** stdio (standard for local Claude Code / Desktop integration)
**Authentication:** HTTP Basic Auth with SAP user credentials

## Error Handling

SAP ADT returns XML error responses. These are parsed and returned as structured MCP error messages — no raw HTTP stack traces exposed to the AI client.

## Testing Strategy

- **`adt/` packages:** Unit tests using `httptest.Server` — no real SAP system required
- **`tools/` packages:** Integration tests via mocked `adt/` calls
- **`testdata/`:** SAP ADT XML response fixtures for realistic test scenarios

## Build & Distribution

- **`Makefile`** with targets: `build`, `build-all` (cross-compile Windows/Linux/macOS), `test`, `release`
- **GoReleaser** (`.goreleaser.yaml`) for automated multi-platform releases
- **GitHub Actions** (`.github/workflows/release.yml`) triggers release on git tag push
- Binary name: `sapadt-mcp` (/ `sapadt-mcp.exe` on Windows)
- Version embedded at build time via `ldflags`: `-X main.version=...`

## Dependencies

- [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) — MCP protocol implementation
- Standard library only for everything else (no HTTP framework needed)
