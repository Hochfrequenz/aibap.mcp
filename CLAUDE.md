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
- **Integration tests**: live in [adtler](https://github.com/Hochfrequenz/adtler) (the SAP ADT client library). Clone that repo to run them. mcp-server-abap itself only has unit tests covering the MCP tool layer.
- **Fix before creating:** When a SAP object (transport, program, etc.) has a problem, fix the existing one first. Don't keep creating new objects to work around issues.
- **Coverage thresholds** (enforced in CI per package): `config` 75%. `tools`/`logging`/`cmd` are covered by unit tests but no minimum is enforced — these packages are thin wrappers around adtler. The adt/auth packages have their own thresholds in adtler's CI.
- **Test package dependency** (for adtler integration tests): SAP package `Z_ADT_MCP_TEST` on the target system. Install from [Hochfrequenz/Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST).

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
- `tools/` — MCP tool registrations and handlers (thin wrappers around the adtler library)
- `tools/register.go` — central tool registration, `toolAdder` interface
- `tools/middleware.go` — logging middleware (tool name, system, duration)
- `config/` — multi-system JSON config loading
- `cmd/` — CLI subcommands (login)
- `logging/` — structured logging setup

The SAP ADT HTTP client, XML marshalling, customizing export, and OAuth2 token management all live in [adtler](https://github.com/Hochfrequenz/adtler) and are imported as `github.com/Hochfrequenz/adtler/adt`, `github.com/Hochfrequenz/adtler/adt/adtxml`, `github.com/Hochfrequenz/adtler/adt/custexport`, and `github.com/Hochfrequenz/adtler/auth`.

## Investigating ADT Endpoints

When you need to understand how an ADT endpoint works or debug unexpected behavior:

1. **Use our own MCP server tools** (`mcp__sap-adt__*`) to query the live SAP system — call `select_system`, then use `get_object_info`, `search_objects`, `get_source`, etc. to inspect real responses. Prefer these over `mcp__sap-desktop__*` (SAP GUI automation is fragile — popups are invisible, complex layouts).
2. **Query TRDIR/TADIR first** — `SELECT NAME, SUBC FROM TRDIR WHERE NAME LIKE 'ZCL_%'` reveals internal program structure. This is ground truth.
3. **Read the ABAP handler source** — use `get_source` on ADT resource classes (`CL_SEDI_ADT_RES_SOURCE`, `CL_WB_ADT_REST_RESOURCE` etc.) to understand what the server expects. Search for error message IDs to find validation code.
4. **Write throwaway integration tests** to probe endpoint behavior (paths, headers, response formats). Delete them once the investigation is done.
5. **Debug handler code** by setting breakpoints in the relevant adtler package (cloned alongside this repo) and running the relevant unit test — see `docs/debugger-investigation.md` for the proven debug flow.
6. **Check ADT discovery** — the server caches `/sap/bc/adt/discovery` XML which lists available endpoints and their accepted content types per system.
7. **Test against both systems** (`hfq` = ECC, `s4u` = S4) — endpoint behavior often differs.
8. **Other implementations are inspiration, not truth** — code targeting BTP/Steampunk may not work on S4 on-prem. Always verify against the real system.

## Coding Pitfalls

- **Never use Go backtick (raw) string literals for ABAP source code** in test fixtures. Backtick strings preserve tab indentation from the Go source file, causing invisible syntax errors in SAP. Use double-quoted strings with `\n` concatenation instead.

## SAP ADT

- Credentials live in `~/.config/sap-mcp/systems.json` (never commit, never put in plain text in commands).
- Config format: see [sap-mcp-config](https://github.com/Hochfrequenz/sap-mcp-config).
- Override config path via `SAP_CONFIG_FILE` env var.
- S4 systems require HTTPS (secure cookie flag breaks HTTP — see #108).
- ECC systems may not have all endpoints (e.g. `/sap/bc/adt/packages` is S4-only).
- **Transport release** only works via REST on S4 (`/newreleasejobs`). On ECC, release must happen via SAP GUI (SE09).
- **Stateful sessions** (`X-sap-adt-sessiontype: stateful`) solve 423 lock errors when SAP checks locks in the wrong enqueue table. Proven for debugger and class includes. When hitting 423 on new endpoints, try stateful sessions first.

## Related projects

This server does not live alone. Keep these in mind when making changes that might affect users, docs, or downstream consumers:

- **[`adtler`](https://github.com/Hochfrequenz/adtler)** — the SAP ADT client library this server wraps. Most bug fixes for the SAP-touching code belong here, not in `mcp-server-abap`. Already referenced above under Testing, Project Structure, and SAP ADT.
- **[`sap-mcp-config`](https://github.com/Hochfrequenz/sap-mcp-config)** — shared config schema for `systems.json`, consumed by both `mcp-server-abap` (Go) and [`sapwebgui.mcp`](https://github.com/Hochfrequenz/sapwebgui.mcp) (Python). If you touch config loading, coordinate changes across both consumers.
- **[`sapwebgui.mcp`](https://github.com/Hochfrequenz/sapwebgui.mcp)** — complementary Python MCP server that drives SAP GUI and SAP Web GUI. Users often run it alongside `mcp-server-abap` for operations ADT cannot handle: abapGit pull (via [`Z_ABAPGIT_PULL_MCP_SHORTCUT`](https://github.com/Hochfrequenz/Z_ABAPGIT_PULL_MCP_SHORTCUT)), customizing table maintenance screens, and ECC-only workflows such as transport release via `SE09` (see the SAP ADT note above). The two MCPs share `~/.config/sap-mcp/systems.json`. When a user asks this server to do something that is fundamentally a GUI-only workflow, the honest answer is usually "use `sapwebgui.mcp` for that step" — do not try to fake it via BlackMagic unless there is already a proven fallback path.
- **[`AIBAP_TEMPLATE_REPOSITORY`](https://github.com/Hochfrequenz/AIBAP_TEMPLATE_REPOSITORY)** — the GitHub template users start from when they set up an AI-driven ABAP vibe coding project. Its README presents the ADT workflow (powered by `mcp-server-abap`) and the abapGit workflow (powered by `sapwebgui.mcp`) as two peer options with a comparison table. If you rename tools, change the `--tools` flag semantics, change supported `create_object` types, or break tool contracts, grep the template repo for stale references and fix them in the same change.
- **[`Z_ADT_MCP_TEST`](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST)** — SAP-side test package for adtler integration tests (already referenced under Testing).
