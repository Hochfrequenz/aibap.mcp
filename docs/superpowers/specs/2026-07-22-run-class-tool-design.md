# Design: `run_class` — execute an ABAP class via ADT classrun

- **Date:** 2026-07-22
- **Repo:** aibap.mcp
- **Status:** Proposed
- **Companion spec:** adtler `RunClass` endpoint client — `<adtler>/docs/superpowers/specs/2026-07-22-classrun-endpoint-design.md`. This tool is a **consumer** of `adt.ClassRunClient.RunClass`; this PR is **`blocked-by-adtler`** until that endpoint ships in a tagged adtler release and is bumped here.

## Motivation

There is no way to execute arbitrary ABAP headlessly through this MCP today.
ADT's classrun capability ("Run as ABAP Application") runs any global class that
implements `IF_OO_ADT_CLASSRUN` and returns its console output. Exposing this as
a generic `run_class` tool unlocks diagnostic and helper flows that ADT
otherwise cannot reach.

The concrete driver is issue #383: cleaning up stale/orphaned enqueue locks
headlessly requires deploying a helper class (calling `cl_enq_admin~remove_locks`
or `ENQUE_DELETE`) and executing it — which is exactly what classrun does. That
lock-cleanup tool is a **separate, later** effort; `run_class` is the generic
primitive it will build on. This spec covers only the generic tool.

## Scope

**In scope:** a new `run_class` MCP tool that takes a class name, performs
client-side pre-validation, invokes `adt.ClassRunClient.RunClass`, and returns a
structured `RunClassResult`; unit + integration tests; the structured-content
guardrail opt-out.

**Out of scope:** the classrun HTTP endpoint itself (adtler — companion spec);
creating/activating classes (caller does that via `create_object` /
`set_source_from_file` / `activate_object`); any lock-cleanup or namespace
restriction. Per the brainstorming decision, `run_class` places **no guard** on
which class may run — it mirrors ADT's standard behaviour; authorization is
governed by `S_DEVELOP` on the configured user.

## Tool design

- **File:** new `tools/classrun.go`, `registerClassRunTools(...)`, wired into
  `RegisterAllWithLockMap()` in `tools/register.go`.
- **Name:** `run_class`.
- **Input:** `class_name` (string, required). The tool builds the CLAS URI
  (`/sap/bc/adt/oo/classes/<name>`) itself.
- **Annotations:** `readOnly=false` (executes code — side effects possible),
  `destructive` left unset/false (the class decides), `idempotent=false`,
  `openWorld=true`.
- **Output:** `RunClassResult` (named type in `tools/results.go`):
  ```go
  type RunClassResult struct {
      ClassName     string `json:"class_name"`
      ConsoleOutput string `json:"console_output"`
  }
  ```
  Returned via `mcp.NewToolResultJSON`, declared with `mcp.WithOutputSchema[RunClassResult]()`.
  (Prefer reusing `adt.ClassRunResult` directly as the wire type if its JSON
  tags match — no parallel DTO — per the CLAUDE.md structured-results rule.)

The client dependency is the interface set `interface { adt.ObjectClient;
adt.ClassRunClient }` (ObjectClient for the pre-checks, ClassRunClient for the
run), following the `registerActivateTools` pattern.

## Pre-validation (Approach ②)

Before calling `RunClass`, the handler fails early with a clear message instead
of forwarding a raw classrun HTTP code. Uses **existing** adtler reads only:

| Check | How | Error on failure |
|---|---|---|
| Class exists | `ObjectExists` / `GetObjectInfo` on the CLAS URI | `class ZCL_X does not exist` |
| Implements `IF_OO_ADT_CLASSRUN` | `GetObjectDependencies(CLAS)` (SEOMETAREL interfaces) — look for `IF_OO_ADT_CLASSRUN` | `class ZCL_X is not runnable (does not implement IF_OO_ADT_CLASSRUN)` |
| Active (optional) | inactive check via object info | `class ZCL_X is inactive — activate before running` |

All pre-validation failures → `errorResult(err)`; `StructuredContent` stays
unset (spec-legal, CLAUDE.md #354). No typed DTO on the error path.

## Data flow

```
run_class(class_name)
  → build CLAS URI
  → pre-validate (ObjectExists, interface check, [active])   ── fail → errorResult, no POST
  → client.RunClass(ctx, class_name)                          [adtler]
  → RunClassResult{class_name, console_output}                → NewToolResultJSON
```

## Error handling

- **Pre-validation failures:** early `errorResult` with the messages above; no
  classrun POST is issued.
- **classrun POST failures:** `adt.ADTError` from adtler is forwarded via
  `errorResult`; the SAP message text (e.g. the `"SAP ADT error N: "` prefix)
  flows into the text content.
- **Runtime exception in the class:** depends on the adtler-side verification
  point (HTTP error vs. 200-with-text). If HTTP error → `errorResult`. If
  200-with-text → success with the error text in `console_output`; the caller
  interprets it. This tool needs no exception-specific branch either way.

## Structured-content guardrail

`tools/structured_content_shape_test.go` auto-covers every registered tool by
invoking it with synthesised args. `run_class` needs a real `*httpClient` and a
blind reflective call would attempt an actual classrun execution, so it likely
needs a **`knownOptOut`** entry (as the `debug_*` tools do), with the reason
noted inline. Confirm during implementation whether the reflective call is
harmless (it would hit the pre-validation "class does not exist" path and return
an object-shaped `errorResult`, which is spec-legal) — if so, no opt-out is
needed. Default assumption: add the opt-out with reason.

## Testing

**Unit (mock client):**
- Class missing → early error, `RunClass` **not** called.
- Interface not implemented → early error, `RunClass` not called.
- Happy path → `RunClass` called once, `RunClassResult` returned with expected
  `console_output`.
- classrun error from client → `errorResult`.

**Integration (`//go:build integration`, live, S4U + HFQ):**
- `run_class` against the real fixture class `ZCL_ADT_MCP_CLASSRUN_TST` in
  `Z_ADT_MCP_TEST` → asserts the known console string.
- Serves as the reproducer snippet for the `blocked-by-adtler` tracking issue.

Coverage: `tools` has no enforced minimum (thin wrapper), but the handler gets
unit tests per the TDD convention.

## Linkage / rollout

1. adtler ships `RunClass` (companion spec) in a tagged release.
2. This repo bumps adtler (`go get …@vX.Y.Z`), then adds `run_class`.
3. Until then this PR is `Draft` + `blocked-by-adtler`, listed on the
   `Next adtler release` tracking issue with a reproducer snippet.
