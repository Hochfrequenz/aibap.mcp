# CLAUDE.md

## Project Overview

MCP server that exposes SAP ADT (ABAP Development Tools) operations as tools for Claude.
Go project using `mcp-go` for the MCP protocol and `stdio` transport.

## Build & Run

- **Build**: `make build` (or `go build -o mcp-server-abap .`)
- **Lint**: `make lint` (runs `golangci-lint` with `dupl`, `goconst`, `gocyclo` enabled)
- **Format**: `gofmt -w .`

## Testing

- **Test-driven**: Write unit tests before or alongside implementation, not after.
- **Unit tests**: `go test ./...` — must always pass before committing.
- **Integration tests**: `go test ./adt/ -tags integration` — run against a real SAP system. Require `SAP_INTEGRATION_*` env vars from `.env`.
- **Transport tests**: `go test ./adt/ -tags 'integration transport'` — create, release, and modify transports on SAP. **Only run when explicitly requested** — these leave artifacts on the system.
- **Never run transport tests automatically** as part of a general integration test run.
- **Coverage thresholds** (enforced in CI per package): `config` 80%, `auth` 75%, `adt/custexport` 60%, `adt/adtxml` 50%, `adt` 40%. `tools`/`logging`/`cmd` are integration-tested only (0%).
- **Test package dependency**: Integration tests require SAP package `Z_ADT_MCP_TEST` on the target system. Install from [Hochfrequenz/Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST).

## Workflow

- One PR per issue. Don't bundle unrelated changes.
- Always use feature branches (`feat/`, `fix/`, `test/`, `refactor/`), never commit directly to `main`.
- Only pick up **unassigned** issues. Assign yourself before starting work.
- Run `gofmt`, `go vet ./...`, and `go test ./...` before committing.

## Adding a New Tool

1. Create `tools/myfeature.go`.
2. Implement `registerMyFeatureTools(s toolAdder, client adt.SomeClient)`.
3. Inside, call `s.AddTool(mcp.NewTool(...), handlerFunc)`.
4. Call `registerMyFeatureTools()` from `RegisterAllWithLockMap()` in `tools/register.go`.
5. Errors: return `errorResult(err), nil` (MCP-level), not `nil, err` (reserved for critical failures).

## Project Structure

- `main.go` — entry point, config loading, MCP server setup (stdio)
- `adt/` — SAP ADT HTTP client (requests, parsing, session handling)
- `adt/adtxml/` — XML serialization for ADT responses
- `adt/custexport/` — customizing table export (SQLite/JSON)
- `tools/` — MCP tool registrations and handlers
- `tools/register.go` — central tool registration, `toolAdder` interface
- `tools/middleware.go` — logging middleware (tool name, system, duration)
- `config/` — multi-system JSON config loading
- `auth/` — OAuth2 and basic auth
- `adtmodel/` — shared type definitions
- `logging/` — structured logging setup

## Investigating ADT Endpoints

When you need to understand how an ADT endpoint works or debug unexpected behavior:

1. **Use our own MCP server tools** (`mcp__sap-adt__*`) to query the live SAP system — call `select_system`, then use `get_object_info`, `search_objects`, `get_source`, etc. to inspect real responses.
2. **Write throwaway integration tests** to probe endpoint behavior (paths, headers, response formats). Delete them once the investigation is done.
3. **Debug handler code** by setting breakpoints in the `adt/` package and running the relevant unit test — see `docs/debugger-investigation.md` for the proven debug flow.
4. **Check ADT discovery** — the server caches `/sap/bc/adt/discovery` XML which lists available endpoints and their accepted content types per system.
5. **Test against both systems** (`hfq` = ECC, `s4u` = S4) — endpoint behavior often differs.

## SAP ADT

- Credentials live in `~/.config/sap-mcp/systems.json` (never commit, never put in plain text in commands).
- Config format: see [sap-mcp-config](https://github.com/Hochfrequenz/sap-mcp-config).
- Override config path via `SAP_CONFIG_FILE` env var.
- S4 systems require HTTPS (secure cookie flag breaks HTTP — see #108).
- ECC systems may not have all endpoints (e.g. `/sap/bc/adt/packages` is S4-only).
