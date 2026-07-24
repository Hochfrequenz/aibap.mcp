# Design: `run_class` — execute an ABAP class via ADT classrun

- **Date:** 2026-07-22
- **Repo:** aibap.mcp
- **Status:** Implemented (this PR; revised: decoupled from lock use-case, explicit confirmation)
- **Companion spec:** adtler `RunClass` endpoint client — `<adtler>/docs/superpowers/specs/2026-07-22-classrun-endpoint-design.md`. This tool **consumes** `adt.Client.RunClass`, shipped in **adtler v0.3.12** (already on `main`); the tool is **no longer `blocked-by-adtler`**.

## Motivation

There is no way to execute arbitrary ABAP headlessly through this MCP today.
ADT's classrun capability ("Run as ABAP Application") runs any global class that
implements `IF_OO_ADT_CLASSRUN` and returns its console output. Exposing it as a
generic `run_class` tool makes "run this ABAP now" a first-class MCP operation
and closes the object lifecycle loop: `create_object` → `set_source_from_file`
→ `activate_object` → **`run_class`**. Today anything that needs SE24 F9 / SE38
forces a switch to a GUI-driven MCP; `run_class` keeps it in ADT.

The tool stands on this generic capability alone — no single downstream use is
the reason it exists. Concrete uses it unlocks:

- **Diagnostics / introspection** — a helper class prints system state or
  configuration to the console.
- **Data fixes / setup helpers** — one-off maintenance logic in a dev system,
  no GUI.
- **Reproducers** — make a bug's minimal repro tool-callable instead of manual
  SE24 F9 / SE38.
- **Orphaned enqueue-lock cleanup** — *one* example, explicitly **not** the
  driver. A deployed helper (e.g. `cl_enq_admin~remove_locks`) could clear a
  stale enqueue that `force_unlock` cannot (see #383). This is **unproven
  end-to-end**: it may 403 cross-session (needs `S_ENQUE` authorization for
  another session's lock), and it touches only the runtime **ENQUEUE** domain —
  it does **not** clear a #442 "locked in request `<TR>`" CTS-registration
  conflict (different lock domain, cleared by retargeting the transport / SE09).
  Any lock-cleanup tool is a separate, later effort built on this primitive;
  `run_class` does not depend on it succeeding.

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
  (COMMIT WORK, deletes). This is the machine-readable hint; the user-facing
  guard is the explicit confirmation below (see Confirmation). Categorically
  riskier than `activate_objects` (which legitimately sets `false`).
- **Output:** reuse **`adt.ClassRunResult`** directly as the wire type
  (`{class_name, console_output}`) via `mcp.NewToolResultJSON` +
  `mcp.WithOutputSchema[adt.ClassRunResult]()`. No parallel DTO — the success
  return is pure pass-through of the adtler struct, and its JSON tags already
  match, so the CLAUDE.md "prefer the adtler struct as the wire type" rule
  applies. Do **not** define a separate `RunClassResult`.

The client dependency is `interface { adt.SearchClient; adt.ClassRunClient }`
(the existence pre-check calls `GetObjectInfo`, which lives on `adt.SearchClient`),
filled from the `adt.Client` passed to `RegisterAllWithLockMap` — which requires
adtler to **embed `ClassRunClient` in the aggregate `Client` interface**
(companion-spec prerequisite; without it the wiring and the `mockClient` break).
`registerClassRunTools(s, client, elicitor)` is wired into the **`system`**
group in `RegisterAllWithLockMap`, which already forwards `elicitor` to
`registerQueryTools` — no new plumbing. Follows the `registerObjectTools` /
`registerQueryTools` pattern (client + `Elicitor`).

## Confirmation (elicitation)

Because `run_class` can trigger arbitrary side effects, the handler asks for
explicit confirmation **before** the classrun POST, reusing the existing
`ConfirmDestructive(ctx, elicitor, message)` helper — the same path as
`delete_object`, `rollback_transport`, and `run_query`. A
`buildRunClassMessage(className)` helper produces a class-specific risk message:

> *"Class `<NAME>` is about to be executed via ADT classrun. It runs arbitrary
> ABAP under the configured user and may cause side effects: COMMIT WORK, data
> changes, or deletions. Approve execution?"*

Only the **helper→`ConfirmDestructive` shape** mirrors `delete_object`, not the
signature: `buildRunClassMessage` takes just `className` and returns a static
string — unlike `buildDeleteMessage(ctx, uri, sc, qc)`, which enriches the
prompt with TADIR metadata. No ctx/client params are needed here.

Decline/cancel → `errorResult(fmt.Errorf("run_class aborted: %s", reason))`,
**no POST** (same wrapping as `run_query`, query.go:74 — a plain error, not a
fabricated `adt.ADTError{StatusCode: 400}` which would render a misleading
"SAP ADT error 400" + bad-request/CSRF hint even though no HTTP call was made;
`errorResult` takes an `error`, not a string).
When the client wires no `Elicitor` (nil), `ConfirmDestructive` returns
`(true, "")` and the class runs unconditionally — matching the stock-binary
behaviour of every other destructive tool (see `elicitation.go`). Confirmation
runs **after** the class-exists check, so a missing class fails cheaply without
prompting.

## Pre-validation

One cheap, safe check before calling `RunClass`:

| Check | How | Error on failure |
|---|---|---|
| Class exists | `GetObjectInfo` on the CLAS URI; a non-nil error is the "missing" signal (same convention as the `object_exists` tool, repository.go:106) | `class ZCL_X does not exist` |

There is no `ObjectExists` client method — `object_exists` is itself built on
`GetObjectInfo`, treating a non-nil error as absent. This spec uses the same
signal.

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
  → GetObjectInfo?           ── err/missing → errorResult, no prompt, no POST
  → ConfirmDestructive(...)  ── declined → errorResult "run_class aborted", no POST
  → client.RunClass(ctx, class_name)                      [adtler]
  → adt.ClassRunResult{class_name, console_output}        → NewToolResultJSON
```

## Error handling

- **Class missing:** early `errorResult`, no confirmation prompt, no classrun POST.
- **Confirmation declined/cancelled:** `errorResult(fmt.Errorf("run_class aborted: %s", reason))`, no POST.
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
opt-out exists because `adt.NewDebugSession` panics on the mock).

The blind reflective call `{"class_name":"x"}` exercises the **happy path**, not
an early error — and that is fine. The default `mockClient.GetObjectInfo`
returns `(&adt.ObjectInfo{}, nil)` (no error), so the class-exists check treats
the class as present. The shape test registers tools with a **nil elicitor**
(`RegisterAllWithLockMap(..., nil, nil)`), so `ConfirmDestructive` returns
`(true, "")` and the call proceeds to `client.RunClass`. The `mockClient`
`RunClass` stub must therefore return `(&adt.ClassRunResult{}, nil)` — a valid,
object-shaped success payload — which is what keeps the guardrail green. **The
stub is load-bearing for the guardrail, not just for interface satisfaction:**
if it returned an error or `nil`, the reflective call would fail the shape
assertion. (Both real-world paths — success and the various `errorResult`
branches above — are object-shaped or leave `StructuredContent` unset, so both
are spec-legal regardless.)

## Testing

**Unit (mock client + stub `Elicitor`):**
- Class missing → early error, no prompt, `RunClass` **not** called.
- Confirmation declined → `errorResult("run_class aborted: …")`, `RunClass`
  **not** called (stub returns decline; mirrors `object_test.go` /
  `rollback_test.go`).
- Nil elicitor → proceeds without prompting (`ConfirmDestructive` returns
  `(true, "")`).
- Happy path (stub accepts) → `RunClass` called once, result returned with
  expected `console_output`.
- classrun error from client → `errorResult`.

**Integration (`//go:build integration`, live, `MCP_INTEGRATION_SYSTEMS` = hfq,s4u):**
- `run_class` against the real fixture class `ZCL_ADT_MCP_CLASSRUN_TST` in
  `Z_ADT_MCP_TEST` → asserts the known console string.

Coverage: `tools` has no enforced minimum (thin wrapper), but the handler gets
unit tests per the TDD convention.

## Rollout / linkage (resolved — no longer blocked)

> **Status update:** all three steps below are done. adtler v0.3.12 shipped
> `RunClass` and is on `main` (dependabot #458); this branch is rebased onto it
> and carries the implementation. The historical rollout plan is kept below for
> context.

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
