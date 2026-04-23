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
- **Integration tests** (this repo): `go test -tags integration -v -count=1 ./tools/...` — local-only, behind `//go:build integration`. Real SAP via `~/.config/sap-mcp/systems.json`, requires VPN + `Z_ADT_MCP_TEST` package installed on target system(s). Covers the MCP wrapper layer only (adtler owns the ADT HTTP/XML/auth layer). Target systems via `MCP_INTEGRATION_SYSTEMS` env var (default `hfq,s4u`).
- **Integration tests** (adtler): live in [adtler](https://github.com/Hochfrequenz/adtler) and cover the ADT HTTP client, XML marshalling, customizing export, and OAuth2. Clone adtler to run them.
- **Reproducer verification** (bump-time): adtler bump PRs run the reproducer snippet from each linked `blocked-by-adtler` issue against the live target system(s). This is narrower than adtler's integration suite — one MCP tool call per fix claim — and is the only live test we run at the mcp-server-abap boundary. Format and flow: see "Cross-Repo Issue Tracking (adtler)".
- **Fix before creating:** When a SAP object (transport, program, etc.) has a problem, fix the existing one first. Don't keep creating new objects to work around issues.
- **Coverage thresholds** (enforced in CI per package): `config` 75%. `tools`/`logging`/`cmd` are covered by unit tests but no minimum is enforced — these packages are thin wrappers around adtler. The adt/auth packages have their own thresholds in adtler's CI.
- **Test package dependency** (for adtler integration tests): SAP package `Z_ADT_MCP_TEST` on the target system. Install from [Hochfrequenz/Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST).

## Workflow

- One PR per issue. Don't bundle unrelated changes.
- Always use feature branches (`feat/`, `fix/`, `test/`, `refactor/`), never commit directly to `main`.
- Only pick up **unassigned** issues. Assign yourself before starting work.
- Run `gofmt`, `go vet ./...`, and `go test ./...` before committing.

## Cross-Repo Issue Tracking (adtler)

Since most fixes now live in [adtler](https://github.com/Hochfrequenz/adtler), issues here often can't be closed until the next adtler release is consumed via `go get`. To keep this visible:

1. **Label proactively**: Whenever you (or an agent) conclude that an mcp-server-abap issue can't be resolved without an adtler change, immediately add the `blocked-by-adtler` label and append it to the tracking issue — don't wait to be asked. Same rule when you spot a new adtler commit/release that resolves an existing open issue here: label it, add a checklist bullet, link the adtler commit or PR. Query open blockers with `gh issue list --label blocked-by-adtler`.
2. **Tracking issue**: A single open issue titled `Next adtler release: bump to vX.Y.Z` collects all blocked issues as a checklist, each bullet `- [ ] #<n> — short description (adtler: <commit-or-PR>)`. There should only ever be one such tracking issue open at a time.
3. **Reproducer snippet on every `blocked-by-adtler` issue**: each such issue must include a copy-pastable MCP tool call (tool name + arguments JSON) or equivalent Go snippet, the target system (`hfq` R/3 / `s4u` S/4), any session preconditions (e.g. "fresh MCP session, no preceding `get_atc_customizing` — see adtler#44"), the "fixed" expected output, and the "broken" current output. This snippet is what the bump PR's reproducer-verify step runs. Without it, false negatives like mcp-server-abap#306 are easy to ship. Example: mcp-server-abap#288.
4. **When bumping adtler**:
   - **Automated path (preferred)**: dependabot opens the bump PR automatically on a new adtler release. `.github/workflows/adtler-bump-template.yml` rewrites the PR body with the tracking issue's checklist + pre-populated `Closes #` lines, and adds the `needs-reproducer-verify` label. Walk each linked issue, run its reproducer against the relevant system(s), prune `Closes` lines for anything still failing, and remove the label before merging.
   - **Manual path**: open a branch `chore/bump-adtler-vX.Y.Z`, run `go get github.com/Hochfrequenz/adtler@vX.Y.Z && go mod tidy`, verify `go test ./...` passes, and replicate the same PR-body checklist and reproducer verification by hand.
   - Either path: every checked item on the tracking issue's checklist needs its own `Closes #` line in the merged PR body.
5. **After merge**: open a fresh `Next adtler release: bump to vX.Y.Z` tracking issue, and move any blockers whose reproducer still failed onto its checklist.

## Adding a New Tool

1. Create `tools/myfeature.go`.
2. Implement `registerMyFeatureTools(s toolAdder, client adt.SomeClient)`.
3. Inside, call `s.AddTool(mcp.NewTool(...), handlerFunc)`.
4. Call `registerMyFeatureTools()` from `RegisterAllWithLockMap()` in `tools/register.go`.
5. Errors: return `errorResult(err), nil` (MCP-level), not `nil, err` (reserved for critical failures).
6. **Structured results (mandatory).** Return values are typed, not stringly. See the rules below — any new handler that serialises through text-only `NewToolResultText` will be rejected in review.

### Structured tool results (rules)

The 2025-06-18 MCP spec has first-class support for typed output via `structuredContent` + `outputSchema`. `mcp-go` exposes this; every handler in this repo uses it. Don't re-invent stringly-typed returns.

**Do:**

- Return successes via `return mcp.NewToolResultJSON(result)` — populates both text fallback (for older clients) and `StructuredContent` (for 2025-06-18 clients) from a single Go value. The helper's `(*CallToolResult, error)` signature matches the handler signature; on marshal failure the error propagates.
- Declare the output schema on every tool that returns an object shape: `mcp.WithOutputSchema[T]()` as a `ToolOption` alongside `mcp.WithDescription(...)` etc. `T` is usually an adtler struct (e.g. `adt.ATCCustomizingResult`) or a local result type in `tools/results.go`.
- Prefer the adtler struct as the wire type when the handler already just marshals one. No parallel DTO layer.
- If there is no adtler type, define a named struct in `tools/results.go` with explicit `json:"..."` tags. One type per tool is fine. Don't inline `struct{...}` literals or `map[string]any{...}`.
- Single-handler types (tightly coupled to one registration) can live next to the registration instead of `tools/results.go` — see `RollbackResult`, `NavigationResult`, `VerifyResult`, `BAdIImplementationWithXML`.

**Do not:**

- `out, _ := json.Marshal(x); return mcp.NewToolResultText(string(out)), nil` — stringly-typed, drops marshal errors. Dead pattern.
- Return bare strings for "success" (`"Transport X released"`, `"Object unlocked"`). Return a typed struct with a boolean flag and relevant IDs.
- Inline anonymous struct types inside handlers just to give `json.Marshal` something shaped. Pull them up to a named type.
- Return `map[string]any{...}` as the success payload. Define a struct.
- **Pass a slice (or any non-object value) to `NewToolResultJSON`**. MCP 2025-06-18 requires `CallToolResult.structuredContent` to be a JSON object (`{ [key: string]: unknown }`); Claude's client enforces this with a Zod `record(...)` check and rejects arrays/nulls/scalars. `mcp-go` does not validate, so the leak is on you. Wrap the slice in a named struct (convention: `{count, <domain-plural>}` — see `SearchObjectsResult`, `BrowsePackageResult`, `VersionHistoryResult`). See issue #351. `tools/structured_content_shape_test.go` is the guardrail: it walks every tool the server exposes via `tools/list`, invokes each with synthesised minimum-required args, and (a) asserts the wire-level `structuredContent` is a JSON object, and (b) validates it against the tool's declared `outputSchema` when one is present. Both checks run on success and error paths. Per MCP 2025-06-18 there is no exemption for `isError: true` — the error path is also covered. New tools are covered automatically — no table entry to remember. If a handler genuinely can't be exercised by a blind reflective call (panics, e.g. the debug_* tools need a real `*httpClient`), add it to `knownOptOuts` in that file with a reason.
- **Attach typed DTOs to `StructuredContent` on the error path**. MCP 2025-06-18 /server/tools requires `structuredContent` to conform to the declared `outputSchema` with no exemption for `isError: true`, so a typed error DTO would contradict every tool's declared success shape. `errorResult` deliberately leaves `StructuredContent` unset — absence is spec-legal. Wrapped SAP status codes already surface via `adt.ADTError.Error()`'s `"SAP ADT error N: "` prefix, which flows into the text content. See issue #354.

**When `WithOutputSchema` does NOT apply (and you leave it off):**

- The tool's return shape is polymorphic across success branches of the same handler (e.g. single-URI path returns one object, array-URI path returns a batch-result object). A schema that describes only one branch is actively misleading. Leave a comment noting why. Both success branches must still return an object — the spec rule above is absolute and independent of whether a schema is advertised. This is independent of the error path, which intentionally omits `structuredContent` entirely (see `errorResult`, #354).
- The tool forwards pre-marshaled JSON bytes from adtler (see `tools/debugger.go` — wrap in `json.RawMessage` so it round-trips through `NewToolResultJSON` without base64-encoding). No typed Go struct exists to generate a schema from.

If you're tempted to reach for any of those escape hatches, first check whether the upstream adtler call can return a typed struct instead — and if it already does, use it.

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

## ADT Discovery & Content-Type Negotiation

SAP ADT endpoints advertise their supported content types and API versions via the **discovery document** (`/sap/bc/adt/discovery`). S/4 and ECC systems often support different versions of the same endpoint. adtler caches this discovery data and provides:

- `NegotiateContentType(endpoint, preferred, default)` — picks the best version the server actually supports
- `acceptHeaderForURI(objectURI)` — resolves the correct Accept header via longest-prefix match + discovery fallback
- `objectTypeAcceptHeaders` map — hardcoded fallback when discovery is empty

**When adding or modifying ADT operations in adtler:**
- **Always use `NegotiateContentType` or `acceptHeaderForURI`** instead of hardcoding content types. The hardcoded map is a fallback, not the primary source of truth.
- **Source path varies by object type**: Programs use `{uri}/source/main`, class includes use `{uri}/includes/{type}` (no `/source/main`), DDIC objects (DTEL, DOMA, TABL) may not have a `/source/main` endpoint at all.
- **ETag fetching uses a two-step fallback**: `ResolveETag` first calls `GetSource` (fine for source-bearing objects), and if that fails (400 for CLAS on S/4, 404 for DDIC) falls back to `FetchETag` — a GET on the bare object URI with a type-appropriate `Accept` header, which works for all object types. Don't regress this fallback; see adtler#9, adtler#14.
- **Preserve full Content-Type in ETags**: SAP ETags may embed `text/plain; charset=utf-8` — never strip the charset suffix, or the `If-Match` will fail with 412. See adtler#15 and the `source_etag_charset_integration_test.go` regression guard.
- **Test against both systems** — the discovery response differs between S/4 and ECC. A content type that works on one may 406 on the other.

## Coding Pitfalls

- **Never use Go backtick (raw) string literals for ABAP source code** in test fixtures. Backtick strings preserve tab indentation from the Go source file, causing invisible syntax errors in SAP. Use double-quoted strings with `\n` concatenation instead.

## SAP ADT

- Credentials live in `~/.config/sap-mcp/systems.json` (never commit, never put in plain text in commands).
- Config format: see [sap-mcp-config](https://github.com/Hochfrequenz/sap-mcp-config).
- Override config path via `SAP_CONFIG_FILE` env var.
- S4 systems require HTTPS (secure cookie flag breaks HTTP — see #108).
- ECC systems may not have all endpoints (e.g. `/sap/bc/adt/packages` is S4-only).
- **Transport release** works via REST on S4 (`/newreleasejobs`). ADT has no release endpoint on ECC; release there depends on whether a BlackMagic fallback is available:
  - **With** a `BlackMagicClient` compiled into the binary (package-level `var blackMagic` in `main.go` set from a build-tagged `init()` in a downstream fork; interface declared in `tools/blackmagic.go`), `ReleaseTransportFallback` performs the release from this MCP transparently.
  - **Without** one, this MCP cannot release on ECC — the caller has to do it from a GUI-driven MCP (`sap-desktop` / `sap-webgui`) via SE09 instead.
- **Stateful sessions** (`X-sap-adt-sessiontype: stateful`) solve 423 lock errors when SAP checks locks in the wrong enqueue table. Proven for debugger and class includes. When hitting 423 on new endpoints, try stateful sessions first.

## Related projects

This server does not live alone. Keep these in mind when making changes that might affect users, docs, or downstream consumers:

- **[`adtler`](https://github.com/Hochfrequenz/adtler)** — the SAP ADT client library this server wraps. Most bug fixes for the SAP-touching code belong here, not in `mcp-server-abap`. Already referenced above under Testing, Project Structure, and SAP ADT.
- **[`sap-mcp-config`](https://github.com/Hochfrequenz/sap-mcp-config)** — shared config schema for `systems.json`, consumed by both `mcp-server-abap` (Go) and [`sapwebgui.mcp`](https://github.com/Hochfrequenz/sapwebgui.mcp) (Python). If you touch config loading, coordinate changes across both consumers.
- **[`sapwebgui.mcp`](https://github.com/Hochfrequenz/sapwebgui.mcp)** — complementary Python MCP server that drives SAP GUI and SAP Web GUI. Users often run it alongside `mcp-server-abap` for operations ADT cannot handle: abapGit pull (via [`Z_ABAPGIT_PULL_MCP_SHORTCUT`](https://github.com/Hochfrequenz/Z_ABAPGIT_PULL_MCP_SHORTCUT)), customizing table maintenance screens, and ECC-only workflows such as transport release via `SE09` (see the SAP ADT note above). The two MCPs share `~/.config/sap-mcp/systems.json`. When a user asks this server to do something that is fundamentally a GUI-only workflow, the honest answer is usually "use `sapwebgui.mcp` for that step" — do not try to fake it via BlackMagic unless there is already a proven fallback path.
- **[`AIBAP_TEMPLATE_REPOSITORY`](https://github.com/Hochfrequenz/AIBAP_TEMPLATE_REPOSITORY)** — the GitHub template users start from when they set up an AI-driven ABAP vibe coding project. Its README presents the ADT workflow (powered by `mcp-server-abap`) and the abapGit workflow (powered by `sapwebgui.mcp`) as two peer options with a comparison table. If you rename tools, change the `--tools` flag semantics, change supported `create_object` types, or break tool contracts, grep the template repo for stale references and fix them in the same change.
- **[`Z_ADT_MCP_TEST`](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST)** — SAP-side test package for adtler integration tests (already referenced under Testing).
