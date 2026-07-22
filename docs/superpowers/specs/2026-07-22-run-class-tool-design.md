# Design: `run_class` — execute an ABAP class via ADT classrun

- **Date:** 2026-07-22
- **Repo:** aibap.mcp
- **Status:** Proposed (revised after agent review)
- **Companion spec:** adtler `RunClass` endpoint client — `<adtler>/docs/superpowers/specs/2026-07-22-classrun-endpoint-design.md`. This tool **consumes** `adt.Client.RunClass`; this PR is **`blocked-by-adtler`** until that endpoint ships in a tagged adtler release and is bumped here.

## Motivation

There is no way to execute arbitrary ABAP headlessly through this MCP today.
ADT's classrun capability ("Run as ABAP Application") runs any global class that
implements `IF_OO_ADT_CLASSRUN` and returns its console output. Exposing this as
a generic `run_class` tool unlocks diagnostic and helper flows that ADT
otherwise cannot reach.

The concrete driver is issue #383: cleaning up stale/orphaned enqueue locks
headlessly requires deploying a helper class (calling `cl_enq_admin~remove_locks`
or `ENQUE_DELETE`) and executing it — exactly what classrun does. That
lock-cleanup tool is a **separate, later** effort; `run_class` is the generic
primitive it will build on. This spec covers only the generic tool.

## Scope

**In scope:** a new `run_class` MCP tool that takes a class name, does a cheap
class-exists pre-check, invokes `adt.Client.RunClass`, and returns the console
output; unit + integration tests.

**Out of scope:** the classrun HTTP endpoint itself (adtler — companion spec);
creating/activating classes (caller does that via `create_object` /
`set_source_from_file` / `activate_object`); any lock-cleanup logic; any
namespace/allowlist restriction on which class may run.

Per the brainstorming decision, `run_class` places **no guard** on which class
runs — it mirrors ADT's standard behaviour; authorization is governed by
`S_DEVELOP` on the configured user. Consistent with that, there is **no
interface pre-check** (see Pre-validation).

## Tool design

- **File:** new `tools/classrun.go`, `registerClassRunTools(...)`, wired into
  `RegisterAllWithLockMap()` in `tools/register.go`.
- **Group:** `system` — a **default-on** group (not in `defaultOffGroups`). Per
  the decision, `run_class` ships enabled by default.
- **Name:** `run_class`.
- **Input:** `class_name` (string, required). The tool builds the CLAS URI
  (`/sap/bc/adt/oo/classes/<name>`) itself. Note: `class_name` deviates from the
  usual `paramObjectURI` convention because the handler constructs the URI and
  the adtler call also takes a bare `className`; documented here as intentional.
- **Description (draft):** *"Execute an ABAP class that implements
  IF_OO_ADT_CLASSRUN (ADT 'Run as ABAP Application') and return its console
  output. The class must already exist and be active. Runs arbitrary ABAP —
  side effects (COMMIT WORK, data changes) are possible."*
- **Annotations:** `readOnly=false`, `idempotent=false`, `openWorld=true`, and
  **`destructive=true`** — `run_class` can trigger arbitrary side effects
  (COMMIT WORK, deletes); the #383 driver literally deletes enqueue locks. A
  client wiring the `Elicitor` (as object/transport/query tools do) will then
  prompt for confirmation before execution. This is categorically riskier than
  `activate_objects` (which legitimately sets `false`).
- **Output:** reuse **`adt.ClassRunResult`** directly as the wire type
  (`{class_name, console_output}`) via `mcp.NewToolResultJSON` +
  `mcp.WithOutputSchema[adt.ClassRunResult]()`. No parallel DTO — the success
  return is pure pass-through of the adtler struct, and its JSON tags already
  match, so the CLAUDE.md "prefer the adtler struct as the wire type" rule
  applies. Do **not** define a separate `RunClassResult`.

The client dependency is `interface { adt.ObjectClient; adt.ClassRunClient }`,
filled from the `adt.Client` passed to `RegisterAllWithLockMap` — which requires
adtler to **embed `ClassRunClient` in the aggregate `Client` interface**
(companion-spec prerequisite; without it the wiring and the `mockClient` break).
Follows the `registerActivateTools` pattern.

## Pre-validation

One cheap, safe check before calling `RunClass`:

| Check | How | Error on failure |
|---|---|---|
| Class exists | `ObjectExists` / `GetObjectInfo` on the CLAS URI | `class ZCL_X does not exist` |

**No interface pre-check.** Detecting `IF_OO_ADT_CLASSRUN` via
`GetObjectDependencies` (SEOMETAREL) only sees **directly** implemented
interfaces — a class inheriting the interface via a superclass or a composite
interface would be a false negative, blocking a run that ADT itself would
execute, contradicting the "mirror ADT" scope. `get_class_definition` is worse
(fragile text-parsing of `INTERFACES` lines). So: skip the interface check
entirely and let the classrun endpoint's own error surface if the class is not
runnable. The class-exists check stays (cheap, friendly, no false negatives).

Pre-validation failure → `errorResult(err)`; `StructuredContent` stays unset
(spec-legal, CLAUDE.md #354). No typed DTO on the error path.

## Data flow

```
run_class(class_name)
  → build CLAS URI
  → ObjectExists?            ── missing → errorResult, no POST
  → client.RunClass(ctx, class_name)                      [adtler]
  → adt.ClassRunResult{class_name, console_output}        → NewToolResultJSON
```

## Error handling

- **Class missing:** early `errorResult`, no classrun POST.
- **classrun POST failure:** `adt.ADTError` forwarded via `errorResult`; the SAP
  message text (`"SAP ADT error N: "` prefix) flows into the text content.
- **Runtime exception in the class:** per the adtler verification point — HTTP
  error → `errorResult`; 200-with-text → success with the error text in
  `console_output`, caller interprets. No exception-specific branch here.
- **Large output / long run:** bounded by adtler's HTTP client timeout (30 s);
  console output returned in full (no client-side truncation). Noted, not
  specially handled.

## Structured-content guardrail

`run_class` stays in the regular `structured_content_shape_test.go` coverage —
**no `knownOptOut`.** The guardrail runs against a `mockClient`, never a real
HTTP client, so there is no panic risk (unlike the `debug_*` tools, whose
opt-out exists because `adt.NewDebugSession` panics on the mock). A blind
reflective call with `{"class_name":"x"}` hits the class-exists check, which on
the mock returns "not found" → `errorResult` with `StructuredContent` unset →
spec-legal → test passes. The only code needed: the `mockClient` gains a
`RunClass` stub (returns `&adt.ClassRunResult{}, nil`) so it still satisfies
`adt.Client`.

## Testing

**Unit (mock client):**
- Class missing → early error, `RunClass` **not** called.
- Happy path → `RunClass` called once, result returned with expected
  `console_output`.
- classrun error from client → `errorResult`.

**Integration (`//go:build integration`, live, `MCP_INTEGRATION_SYSTEMS` = hfq,s4u):**
- `run_class` against the real fixture class `ZCL_ADT_MCP_CLASSRUN_TST` in
  `Z_ADT_MCP_TEST` → asserts the known console string.

Coverage: `tools` has no enforced minimum (thin wrapper), but the handler gets
unit tests per the TDD convention.

## Rollout / linkage (blocked-by-adtler)

1. adtler ships `RunClass` + embeds `ClassRunClient` in `Client` (companion spec), tagged release.
2. This repo bumps adtler (`go get …@vX.Y.Z`, no pseudo-version to `main`), then adds `run_class`.
3. Until then: `Draft` PR + `blocked-by-adtler` label, listed on the
   `Next adtler release: bump to vX.Y.Z` tracking issue as
   `- [ ] #<n> — run_class tool (adtler: <PR>)`.

**Reproducer snippet** (for the tracking issue, per CLAUDE.md point 3):
- Tool call: `run_class` with `{"class_name": "ZCL_ADT_MCP_CLASSRUN_TST"}`
- Target: `s4u` (S/4) and `hfq` (ECC), fresh MCP session, no preconditions.
- Fixed (expected): `{"class_name": "ZCL_ADT_MCP_CLASSRUN_TST", "console_output": "<known string the fixture writes>"}`
- Broken (current): tool `run_class` does not exist / not registered.

At bump time, this snippet goes into the throwaway
`tools/bump_<version>_verify_integration_test.go` harness
(`TestBumpVerify_RunClass`, build tag `integration`, `// Delete after the bump
PR merges.`), listed in the PR's Test Plan as a follow-up deletion item.
