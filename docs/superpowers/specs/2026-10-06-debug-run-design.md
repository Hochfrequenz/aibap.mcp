# Design: `debug_run` — breakpoint debugging that does not depend on client concurrency

- **Date:** 2026-10-06
- **Repo:** aibap.mcp
- **Status:** Draft (design approved in conversation; spec under review)
- **Issues:** #558 (this tool), #559 / PR #561 (debugging guide), #562 / PR #563 (debug session lifecycle), #513, #435
- **adtler prerequisites:** Hochfrequenz/adtler#200 (breakpoints), Hochfrequenz/adtler#201 (variables), Hochfrequenz/adtler#196 (attach without retry); already on adtler `main`: #189, #190, #192

## Motivation

The debug tools work over plain ADT HTTP. Setting a breakpoint, catching the run, attaching, reading the stack and variables, and stepping were all verified live on both systems:
- SAP S/4HANA 2025 on-premise (SAP_BASIS 816);
- SAP ERP 6.0 EHP8 (SAP_BASIS 750).

What fails in practice is the choreography:

- **`debug_start` blocks** until a run hits the breakpoint or the timeout expires. A trigger has to run *during* that call.
- **Some MCP clients run tool calls one after another**, even when they are issued together. In those clients, a `run_unit_tests` issued next to `debug_start` starts only after the listener has timed out, and nothing is caught. This client behaviour, not SAP, explains the earlier misses with unit tests (#435).
- **The caller has to know the procedure:** order, timing, which triggers work, and how to end a session. Without it, the call fails silently: the listener times out while the program runs to completion.

The server should therefore own the concurrency (listener in the background), trigger the run itself where it can, and otherwise hand back exact instructions.

## Verified facts this design relies on

All live on both releases unless marked otherwise, 2026-10-05/06.

**Triggers that are caught (external breakpoint, user mode, listener waiting):**
- **ABAP Unit run over ADT:** 4 of 4 per system.
- **A plain HTTP request from a separate client**, authenticated as the same user, with no ADT session:
  - breakpoint in an ICF handler class method;
  - breakpoint in a function module called through the SOAP-RFC service.

  Both caught on both systems within 0.1–0.5 s.
- **A SAP GUI session** enabled for ADT external debugging, with the program run in that same session. Enabling uses the OK code `/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>` on 816 and transaction `SADT_START_TCODE` on 750 (#559).
- **Global class breakpoints** work with the plain source URI form.

**Not caught:**
- breakpoints in system programs (TRDIR status `S`);
- a GUI session that was not enabled;
- a different GUI session of the same user;
- a run that starts after the listener has returned.

**Breakpoint mechanics** (Hochfrequenz/adtler#200):
- Sending the complete list in **one** request is the only form that behaves the same on both releases.
- Repeated requests replace (816) or add (750).
- An empty `syncMode="full"` request clears all breakpoints on 816 and nothing on 750.
- `DELETE …/debugger/breakpoints/<id>` removes one breakpoint on both releases.
- **While attached**, a new breakpoint must be sent in the stateful debug session with `scope="debugger"`. Sent the external way, it detaches the debugger (`CX_TPDAPI_NOT_ATTACHED`).
- External breakpoints persist across runs. After a hit and `detachDebugger`, a fresh listener catches the next run.

**Ending a session:**
- `detachDebugger` ends cleanly on both releases.
- `stepContinue` to a *next* breakpoint works on both.
- `stepContinue` past the end of a run behaves differently by release (#513):
  - 750 answers with `AdiFailed` / debuggee ended;
  - 816 answers with `400 ExceptionInvalidData` since 2026-10-06.
- After a SAP GUI trigger, `stepContinue` hung and wedged the session.

**Variables** (Hochfrequenz/adtler#201): structures, internal tables (paged), object and data references are readable through `getChildVariables`, `getVariables` and `getVariableData` in the stateful session.

## Scope

**In scope**
1. A new tool **`debug_run`** that replaces `debug_start`.
2. A new tool **`debug_wait`**.
3. A run state machine in the server, with a background listener and an optional background trigger.
4. An optional extension interface **`DebugTriggerer`** that a build may provide (see "Trigger extension").
5. Changes to the existing tools:
   - `debug_step` returns the position;
   - `debug_get_variable` can expand complex variables;
   - `debug_set_breakpoint` picks the scope automatically;
   - `debug_remove_breakpoint` is implemented for real;
   - `debug_stop` cleans up on the SAP side.
6. Instructions for the triggers that need a person or another agent.
7. The debugging guide (#559) updated to the new tools.

**Out of scope** (separate issues): setting variable values; conditional, statement or exception breakpoints; the server-instance header for multi-server systems (#513); the server itself calling HTTP services or RFC function modules as a trigger. The last one would cross the scope guardrail in `README.md`; a build may provide it as a `DebugTriggerer`.

## Tool surface

### `debug_run` (new; `debug_start` is removed)

Input:

```
breakpoints:      [{object_uri, line}]   // ≥ 1, source URIs (…/source/main)
user?:            SAP user whose run is caught (default: logon user of the active system)
trigger: {
  kind:   "unit_tests" | "external" | "gui"
  target? {                                  // gui only
    type:   "report" | "transaction" | "function_module" | "class_method"
    name:   string                           // program, tcode, function module, class=>method
    inputs? {string: string}                 // selection screen / parameters, free-form
  }
  object_uri?: string                        // unit_tests: object whose tests run (default: first breakpoint's object)
}
timeout_seconds?: listener budget for this run (default 300)
```

Behaviour:
1. If a run is active, stop it first (see "Cleanup").
2. Set **all** breakpoints in one request with `scope="external"`. If any breakpoint is rejected (e.g. "Cannot create a breakpoint at this position"), remove the ones that were set and return an error that names the rejected breakpoint.
3. Start the listener goroutine. Wait about 4 s, because the activation reaches the server asynchronously.
4. Trigger:
   - `unit_tests`: start `RunUnitTests` in a goroutine on the main client.
   - `gui` with a `DebugTriggerer`: call it.
   - `external`, or `gui` without a triggerer: do not trigger.
5. If the server triggered, wait up to 15 s for the first event. On a hit, attach and return `attached`. Otherwise return `waiting`.

Output (one object; fields depend on `status`):

```
status:          "attached" | "waiting" | "trigger_failed" | "error"
run_id:          string
breakpoints:     [{object_uri, line, id}]
debuggee_id?:    string
position?:       {program, include, line, source_excerpt}   // attached
instructions?:   {steps: [string], notes: [string], user, listening_until}  // waiting / trigger_failed
trigger_error?:  string
```

### `debug_wait` (new)

Input: `timeout_seconds?` (default 60; kept short because MCP clients often have their own per-call limit).

The tool blocks on the run's event channel. One of these comes back:
- **`attached`**, with position, after an automatic attach;
- **`trigger_done`**, with the trigger result (e.g. the unit-test result) once the debuggee has finished;
- **`timeout`**, when the listener budget is used up;
- **`still_waiting`**, when only this call's own timeout expired. The caller simply calls again.

Re-arming: if the listener has ended, no debuggee is attached, and the run is still active, `debug_wait` restarts the listener for the next run. A fresh listener catches the next run with the same breakpoints.

### Changes to existing tools

| Tool | Change |
|---|---|
| `debug_step` | Returns `position` (program, include, line, source excerpt) instead of raw XML. The description recommends `detachDebugger` to end, and explains both "debuggee ended" signals. |
| `debug_get_variable` | Gains an optional `expand` mode: children of structures, object attributes, `REF->*`; table rows by `offset`/`limit`. Scalars are read as today. |
| `debug_set_breakpoint` | While a debuggee is attached, sends `scope="debugger"` in the stateful session; otherwise adds an external breakpoint by re-sending the run's full list. |
| `debug_remove_breakpoint` | Implemented with `DELETE …/breakpoints/<id>`; external or debugger scope, chosen as in `debug_set_breakpoint`. |
| `debug_stop` | Runs the full cleanup below and reports what it removed. |
| `debug_attach`, `debug_get_stack`, `debug_set_watchpoint`, `debug_get_sessions` | Unchanged. `debug_attach` is rarely needed, because `debug_run` and `debug_wait` attach automatically. |

### Trigger extension

```go
// DebugTriggerer starts a run so that the external breakpoints of user are hit.
// Optional: checked by type assertion on the BlackMagicClient, so existing
// implementations keep compiling. Called only for trigger.kind "gui", only after
// the listener is running.
type DebugTriggerer interface {
    TriggerDebugRun(ctx context.Context, system, user string, t DebugTarget) error
}
```

How a triggerer starts the run is its own business. It must start the run in a way that is eligible for the user's external debugging. If it returns an error, `debug_run` answers `trigger_failed` and includes the instructions as the fallback.

## Run state and concurrency

- **Ownership:** one active run per debug session (the session from #563, bound to user and system).
- **The run holds:** breakpoint IDs, trigger kind, a buffered event channel, a cancel function, state (`listening` → `attached` → `ended` | `stopped`) and the trigger result. All of it sits behind its own mutex. No HTTP call is made while a lock is held.
- **Listener goroutine:** calls `StartListener` in 60 s slices until a hit, the run budget, or cancellation. A hit becomes `hit{debuggee_id}` on the channel, and the goroutine ends.
- **Trigger goroutine** (for `unit_tests`, and for `gui` with a triggerer):
  - `RunUnitTests` runs on the main client, which is a separate cookie jar from the debug session.
  - It blocks while the debuggee is halted. When it returns, it emits `trigger_done{result | error}`.
- **Attaching:** happens in the handler of `debug_run` or `debug_wait`, never in a goroutine, so errors reach the caller.

### Cleanup

The same cleanup runs on `debug_stop`, on a new `debug_run`, and when #563 replaces the session for another user or system. Every step is best effort and bounded to 10 s:

1. If a debuggee is attached, send `detachDebugger` first. Otherwise the program stays suspended in SAP: the work process stays occupied and the trigger request stays open.
2. Cancel the run context. That ends the pending long poll, and `StopListener` sends the `DELETE`.
3. Remove the run's breakpoints with one `DELETE` per ID, in both scopes. An empty `full` request is not used, because it removes nothing on 750.
4. Let a trigger goroutine finish on its own after the detach. Its result is attached to the discarded run and dropped.

When the server process exits, the listeners expire on the SAP side. External breakpoints that cleanup could not remove stay until they are deleted or the user's external-debugging activation expires; `debug_stop` reports any it could not remove.

### Errors

| Case | Result |
|---|---|
| a breakpoint is rejected | `debug_run` errors before listening; the others are removed |
| listener `409` (another debugger listens for the user) | `error`: "another debugger, e.g. Eclipse ADT, is listening for this user" |
| listener budget used up | `timeout`, with the usual causes: different user, system program, run started before or after listening, GUI session not enabled |
| attach `AdiFailed` | one attempt only (Hochfrequenz/adtler#196); the message says to start a new `debug_run` |
| triggerer error | `trigger_failed` with instructions |
| unit test finished without a hit | `trigger_done` without a prior hit: "the breakpoint line was not executed" |

## Instructions

`instructions` is structured (`steps`, `notes`, `user`, `listening_until`) so that a person, an agent or a script can follow it.

**`external`**
- Step: "Call your HTTP service or RFC function module now, as user `<user>`, before `<listening_until>`."
- Notes:
  - Use a new connection, or the first request of a stateful session.
  - The call must run as the same user.
  - Breakpoints in system programs are never hit.

**`gui`**
- **First step:** enable the GUI session. To pick the method, the server checks once per system whether program `RS_ADTDBG_ACTIVATE_BY_OKCODE` exists (a TRDIR read, repository metadata only):
  - if it exists, the step is the OK code `/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>`;
  - otherwise it is `SADT_START_TCODE` with `D_AIE_TCODE`, `D_IDE_USER` and `D_REQUEST_USER`, Eclipse navigation off;
  - if the check fails, both are given, OK code first.
- **Then, by target:**
  - **report:** SE38, name, F8, selection screen from `inputs`;
  - **transaction:** `/n<tcode>`, or directly as `D_AIE_TCODE` of `SADT_START_TCODE`;
  - **function module:** SE37, name, F8 (test environment), `inputs`, F8;
  - **class method:** SE24, name, F8 (test environment), method, `inputs`, execute.
- **Always:** "in the same window".
- The transaction, SE37 and SE24 paths are marked untested until the live acceptance confirms them.

## Debugging guide

The guide (#559, PR #561) is rewritten around `debug_run` and `debug_wait`:
- triggers and instructions;
- ending with `detachDebugger`;
- breakpoints while halted;
- variables;
- system programs;
- the per-release notes above.

PR #561 stays open until the live acceptance below has passed.

## Testing

**adtler** (in Hochfrequenz/adtler#200 and #201):
- unit tests with httptest;
- integration tests on both systems:
  - two breakpoints in one request: stop at A, then at B;
  - a debugger-scope breakpoint while halted;
  - `RemoveBreakpoint` stops catching;
  - variables of each kind.

**aibap.mcp unit tests**, with the transport-level fake from #563 extended (listener hit after a delay, `RunUnitTests` endpoint):
- `unit_tests` returns `attached`, then `trigger_done`;
- `external` returns `waiting`; `debug_wait` then returns `attached`;
- `gui` without a triggerer returns instructions, chosen per OK-code availability;
- `gui` with a fake triggerer: the triggerer is called only after the listener has started; its error gives `trigger_failed`;
- a rejected breakpoint: the others are removed;
- cleanup order: detach, then listener stop, then breakpoint deletes;
- a session replaced during an active run;
- `debug_wait` re-arms for the next run;
- concurrency tests under `-race` (in WSL; the Windows host has no cgo toolchain).

**Live acceptance:** the real binary, a fresh MCP client session without prior context, only the guide and tool descriptions, both systems:
1. `unit_tests`: hit, variables (structure, table page), step, `detachDebugger`, test result.
2. `external`: a SOAP-RFC call to a test function module from a separate HTTP client.
3. `gui`: a person follows the instructions.
4. `gui`: an agent drives a GUI-automation MCP server in the same client while `debug_wait` waits. This end-to-end case has never been run so far.
5. A breakpoint added while halted; `debug_stop`; a control run is no longer caught.

## Delivery

| # | Repo | Content | Depends on |
|---|---|---|---|
| A1 | adtler | #200 breakpoints (`SetBreakpoints`, scope, `RemoveBreakpoint`) | — |
| A2 | adtler | #201 variables | — |
| A3 | adtler | #196 attach without retry (open PR) | — |
| A4 | adtler | **release** | A1–A3 |
| B1 | aibap.mcp | #563 session lifecycle (open PR) | — |
| B2 | aibap.mcp | #558 `debug_run`, `debug_wait`, run state, `DebugTriggerer`, instructions; remove `debug_start` | B1, A1 |
| B3 | aibap.mcp | position in `debug_step`, variable expansion, breakpoint scope, `debug_remove_breakpoint`, cleanup in `debug_stop` | B2, A1, A2 |
| B4 | aibap.mcp | #561 guide, after the live acceptance | B2, B3 |

B2 and B3 are built as drafts against a pinned adtler `main` commit, which CLAUDE.md allows as the "pre-release path". They are re-pinned to the release tag before merge.

**Versioning:** removing `debug_start` and adding `debug_run`/`debug_wait` changes the tool surface, so this is a **minor** bump.
