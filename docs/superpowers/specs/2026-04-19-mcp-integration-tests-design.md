# MCP-layer integration tests against live SAP

**Issue:** #277
**Date:** 2026-04-19

## Problem

After the adtler extraction, mcp-server-abap has **zero integration tests** — only unit tests with mocks in `tools/`. There is no automated way to verify that the MCP tool wrappers work end-to-end against a real SAP system.

adtler already has integration tests covering the ADT HTTP/XML/auth/discovery layer. This repo needs tests that specifically cover the MCP **wrapper** layer: MCP arg JSON parsing, adtler-error → `CallToolResult` formatting, tool registration, system selector switching, and lock-map integration.

## Scope

**In scope (first PR):**

Five read-only tools exercised end-to-end against two systems (`hfq` = ECC, `s4u` = S/4):

- `select_system` — verify system switching
- `search_objects` — read-only query
- `get_source` — read-only, exercises lock-map integration
- `get_text_elements` — read-only
- `syntax_check` — read-only, uses a warning fixture

Plus one **negative test** (`get_source` on a non-existent object) to verify SAP errors round-trip as MCP `IsError=true` results rather than Go `error` returns.

**Out of scope:**

- Write operations (`create_transport`, `patch_source`, etc.) — need cleanup strategy, deferred
- CI execution — integration tests remain local-only per issue #277; future work may add a manual-dispatch workflow
- Duplicating adtler's HTTP/XML/auth assertions

## Design

### Build-tag gating

All new test code lives behind `//go:build integration`. The default `go test ./...` invocation is unchanged.

Run with:

```bash
go test -tags integration -v -count=1 ./tools/...
```

### Config source

Tests load the shared `sap-mcp-config` systems.json via the existing `config.Load()` entry point, honoring `SAP_CONFIG_FILE` exactly like `main.go`. No per-field env overrides; no new credential env vars. Matches the shared convention with `sapwebgui.mcp`.

The stale `.env` file at repo root (containing unused `SAP_INTEGRATION_*` values) is deleted in the same PR.

### Target systems (env-driven)

A new env var selects which systems to run against:

```
MCP_INTEGRATION_SYSTEMS=hfq,s4u   # default if unset
```

Parsed once in `TestMain` into a package-level slice. Each test iterates the slice via `t.Run(sys, ...)`. If a listed key is not present in the loaded config, the corresponding subtest calls `t.Skipf` with a message pointing at the env var — the skip is loud, not silent.

### Reachability + fixture skip strategy

Two layers of environment-driven skip, both loud:

1. **System reachability** — `TestMain` tries to construct an adtler client and perform a cheap ping (e.g., `search_objects` on a narrow pattern) for each target system. Unreachable systems are marked unavailable; subtests for them call `t.Skipf("system %q unreachable — see TestMain log", sys)`.
2. **Fixture presence** — each subtest that depends on a `Z_ADT_MCP_TEST` object first calls `object_exists`. If the fixture is missing on that system, `t.Skipf("fixture %q missing on %s — install Hochfrequenz/Z_ADT_MCP_TEST", name, sys)`.

Skips log via `t.Logf` **and** `t.Skipf` so the reason appears twice in output. At the end of `TestMain`, a single-line summary prints:

```
integration targets: hfq=OK s4u=UNREACHABLE
```

This makes it impossible to miss when a system wasn't actually covered — supporting the rule that untested code must not ship.

### Test harness structure

**Helper extraction** (mechanical, no behavior change):

Today `callTool()` lives in `tools/source_test.go`. To reuse it from an integration-tagged file without build-tag leakage, extract it plus `newTestServer`-style construction into `tools/testhelper_test.go` (no build tag — visible to both unit and integration tests). Existing unit tests keep working unchanged.

**Shared server, not per-subtest:** we construct **one** MCP server in `TestMain` with all reachable target systems registered in the `ClientRegistry`. Tests switch between them via `select_system` inside each `t.Run(sys)` subtest. This:

- mirrors real MCP client behavior (one server, many systems, `select_system` between calls)
- actually exercises `select_system` in every test, not just in its own dedicated test
- avoids redundant server construction

**New helper** (integration-tagged):

```go
// in tools/integration_test.go
func newIntegrationServer(t *testing.T, cfg *config.Config, reachable []string) *server.MCPServer
```

Builds a real adtler `ClientRegistry` from `cfg` restricted to the `reachable` systems, picks the first one as default, constructs `adt.NewLockMap()`, and calls `RegisterAllWithLockMap()` once. No mocks at any layer.

**Per-test shape:**

```go
var (
    sharedServer        *server.MCPServer   // initialized in TestMain
    reachableSystems    []string
)

func TestIntegration_GetSource(t *testing.T) {
    for _, sys := range integrationSystems {
        t.Run(sys, func(t *testing.T) {
            requireReachable(t, sys)
            requireFixture(t, sys, "Z_ADT_MCP_TEST_REPORT")

            mustSelectSystem(t, sharedServer, sys)
            res := callTool(t, sharedServer, "get_source", map[string]any{
                "object_uri": "/sap/bc/adt/programs/programs/z_adt_mcp_test_report",
            })

            requireNotError(t, res)
            var payload struct{ Source string }
            mustUnmarshalTextContent(t, res, &payload)
            require.NotEmpty(t, payload.Source)
            require.Contains(t, strings.ToUpper(payload.Source), "REPORT")
        })
    }
}
```

`mustSelectSystem` is a thin helper that calls `select_system` and asserts `IsError=false`. `TestIntegration_SelectSystem` additionally asserts on the response text and verifies a follow-up `search_objects` actually hits the newly-selected system (by querying a system-specific pattern if feasible, or just by checking no error).

### Assertion matrix

All assertions target **wrapper shape**, not SAP content (adtler already pins content).

Return shapes were verified against the current tool source. Assertions target those exact shapes — they will catch a wrapper-layer refactor that breaks the shape contract.

| Tool | Fixture | Return shape (verified) | Shape assertion |
|---|---|---|---|
| `select_system` | — | human-readable text (not JSON) | `IsError=false`; result text mentions the selected system key. Follow-up `search_objects` call verifies subsequent tools hit the new client. |
| `search_objects` | query `Z_ADT_MCP_TEST*` | top-level JSON array of `ObjectInfo` (fields `URI`, `Type`, `Name`, `Description`, `PackageName` — no JSON tags, so capitalized) | `IsError=false`; JSON unmarshals into `[]struct{Name,Type,URI string}`; slice non-empty; at least one element's `Name` matches the `Z_ADT_MCP_TEST*` prefix. |
| `get_source` | `Z_ADT_MCP_TEST_REPORT` | `{"source":"…","etag":"…"}` | `IsError=false`; JSON unmarshals with both keys; `source` non-empty and contains the keyword `REPORT` (case-insensitive, weak marker proving real ABAP passed through). |
| `get_text_elements` | `Z_ADT_MCP_TEST_REPORT` | `TextElements{Symbols,Selections omitempty}` — fixture has neither, so result is `{}` | `IsError=false`; JSON unmarshals as an object (even if empty). Weaker than other assertions because the fixture has no text symbols. See "Fixture gap" below. |
| `syntax_check` | `Z_ADT_MCP_TEST_SYNWARN` | top-level JSON array of `SyntaxMessage` (fields `Type`, `Text`, `Line`, `Column` — no JSON tags) | `IsError=false`; JSON unmarshals into `[]struct{Type,Text string; Line,Column int}`; slice has ≥1 entry with `Type=="W"` (fixture declares an unused variable). |
| **negative** `get_source` | `ZZZ_DOES_NOT_EXIST_<rand>` | error result | `IsError=true`; error text non-empty. Proves SAP errors round-trip as MCP errors, not Go `error` returns from the tool handler. |

**Fixture gap:** `get_text_elements` has no fixture with actual text symbols. The current assertion only proves the tool doesn't panic or return invalid JSON — it cannot prove symbols/selections marshalling. Follow-up: extend `Z_ADT_MCP_TEST_REPORT` with `SELECTION-SCREEN` + `TEXT-001` so we can strengthen this assertion. Tracked as a note in the plan, not blocking this PR.

### File layout

- `tools/testhelper_test.go` (new, no build tag) — extracted `callTool`, shared test server construction
- `tools/integration_test.go` (new, `//go:build integration`) — `TestMain`, `newIntegrationServer`, all six integration tests, skip helpers
- `tools/source_test.go` (modified) — remove the helpers that moved to `testhelper_test.go`
- `.env` (deleted) — stale, unused

### Cleanup

All five tools are read-only. No transports created, no lock handles held, no SAP-side state to unwind. The negative test targets a deliberately-nonexistent object, so no cleanup there either.

### Docs

**README** gets a new subsection under "Testing":

> **Running integration tests locally**
> Requires: VPN to hfq/s4u, `~/.config/sap-mcp/systems.json`, `Z_ADT_MCP_TEST` package installed on each target system (see [Hochfrequenz/Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST)).
> ```
> go test -tags integration -v -count=1 ./tools/...
> ```
> Override target systems: `MCP_INTEGRATION_SYSTEMS=hfq go test ...`
> Skipped systems are logged as `SKIP system=<key>` — always check the output to confirm coverage.

**CLAUDE.md** Testing section gets one paragraph distinguishing:

- **Unit tests** (no tag, always run, mock adtler clients)
- **Integration tests** (`-tags integration`, local-only, real SAP)

### Observability for release gating

"Untested software shall not ship." The design makes skips loud but does not yet enforce full coverage at release time. Follow-up issue (not this PR): add a release-gate script that runs `-tags integration` and fails if any target shows `UNREACHABLE` in the `TestMain` summary.

## Out of scope (follow-up issues)

- Write-operation integration tests (`patch_source`, `create_transport`, etc.) — need SAP-side cleanup strategy
- CI wiring with SAP secrets — needs runner with VPN access
- Release-gating script enforcing full system coverage
- Per-test timeout / flake-retry policies — add once we see real flake patterns
