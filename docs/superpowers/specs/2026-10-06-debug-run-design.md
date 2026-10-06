# Design: `debug_run` — breakpoint debugging that does not depend on client concurrency

- **Date:** 2026-10-06
- **Repo:** aibap.mcp
- **Status:** Draft, revised after three independent reviews; awaiting the maintainer's approval
- **Issues:** #558 (this tool; live evidence in the 2026-10-06 comment), #559 / PR #561 (debugging guide), #562 / PR #563 (debug session lifecycle), #513, #435
- **adtler prerequisites:** Hochfrequenz/adtler#200 (breakpoints), Hochfrequenz/adtler#201 (variables), Hochfrequenz/adtler#196 (attach without retry), plus the two adtler additions listed under "Delivery". Already on adtler `main`: #189, #190, #192.

## Motivation

The debug tools work over plain ADT HTTP. Setting a breakpoint, catching the run, attaching, reading the stack and variables, and stepping were all verified live (#435, #513, #558) on two systems:
- SAP S/4HANA 2025 on-premise (SAP_BASIS 816);
- SAP ERP 6.0 EHP8 (SAP_BASIS 750).

What fails in practice is the choreography:

- **`debug_start` blocks** until a run hits the breakpoint or the timeout expires, so a trigger must run *during* that call.
- **Some MCP clients run tool calls one after another**, even when they are issued together. In those clients, a `run_unit_tests` issued next to `debug_start` starts only after the listener has timed out. This is the most likely explanation for the earlier misses with unit tests (#435): the same sequence, run concurrently outside MCP, was caught 4 out of 4 times on each system.
- **The caller has to know the procedure:** order, timing, which triggers work, and how to end a session. Without it, the failure is silent: the listener times out while the program runs to completion.

The server should therefore own the concurrency (listener and attach in the background), trigger the run itself where it can, and otherwise hand back exact instructions.

## Facts this design relies on

"Both" means both releases. Run counts are given where they are small. Sources: #435, #513, #559, #558 (2026-10-06 comment), Hochfrequenz/adtler#200 and #201.

**Caught** (external breakpoint, user mode, listener waiting):

| Trigger | Evidence |
|---|---|
| ABAP Unit run over ADT | both, 4 of 4 runs per system |
| plain HTTP request from a separate client, as the same user | both. ICF handler class method: 1 run per system. Function module through the SOAP-RFC service: 2 runs (816), 1 run (750) |
| SAP GUI session enabled for ADT external debugging, program run in that same session | both, 1–2 runs per system. Enabling uses the OK code `/H_REACTIVATE_EXTD_DBG` on 816 and `SADT_START_TCODE` on 750 |
| next run, after a hit and `detachDebugger`, with a new listener on the same debug session | 816, 1 run |

**Not caught:**
- a breakpoint in a system program (TRDIR status `S`): 816, 3 lines tried;
- a GUI session that was not enabled: both;
- a different GUI session of the same user: 750 only;
- a run that started after the listener returned: by construction.

**Breakpoints:**
- Global class breakpoints are accepted with the plain source URI form and mapped to the method include. This was seen on both releases; a catch was observed in one handler class per system.
- Sending the complete list in **one** request is the only form that behaves the same on both releases. Repeated requests replace the set (816) or add to it (750). An empty `syncMode="full"` request clears everything (816) or nothing (750).
- `DELETE …/debugger/breakpoints/<id>` removes one breakpoint: both releases.
- A breakpoint added **while the debugger is attached** must be sent in the stateful session with `scope="debugger"`. Sent the external way, it detaches the debugger (`CX_TPDAPI_NOT_ATTACHED`). Tested on 750, one run per variant.

**Ending a session:**
- `detachDebugger` ended cleanly after a GUI trigger (1 run per system) and after a unit-test trigger (816, 1 run).
- `stepContinue` to a *next* breakpoint works on both releases.
- `stepContinue` past the end of a run answers `AdiFailed` / debuggee ended on 750. On 816 it answers `400 ExceptionInvalidData` since 2026-10-06 (8 runs, #513).
- After a GUI trigger, `stepContinue` hung and wedged the session (816, 1 run).

**Variables** (Hochfrequenz/adtler#201): structures, internal tables (paged), object references and data references can be read through `getChildVariables`, `getVariables` and `getVariableData` in the stateful session: both releases.

**Unknown (the design must not depend on these):**
- how long SAP keeps a caught debuggee waiting for an attach;
- whether an external breakpoint fires again in the same run after `detachDebugger`;
- the GUI targets transaction, SE37 test and SE24 test;
- whether "program `RS_ADTDBG_ACTIVATE_BY_OKCODE` exists" really means "the OK code works" (inferred from two releases);
- whether breakpoints left under the IDE ID of a crashed process expire with the user's external-debugging activation, and whether they affect listeners with a different IDE ID;
- whether deleting external breakpoints while a debugger is attached detaches it, as setting one does on 750;
- whether the server's 600 s cap per listener poll (read in the ABAP source on 816) also applies on 750.

## Scope

**In scope**
1. A new tool **`debug_run`** that replaces `debug_start`; `debug_attach` is removed (attaching becomes automatic).
2. A new tool **`debug_wait`**.
3. A run state machine in the server, with the listener, the attach and an optional trigger running in the background.
4. An optional extension interface **`DebugTriggerer`**.
5. Changes to the existing debug tools (see "Changes to existing tools").
6. Instructions for triggers that need a person or another agent.
7. The debugging guide (#559) rewritten for the new tools.

**Out of scope** (separate issues):
- setting variable values;
- conditional, statement or exception breakpoints;
- the server-instance header for systems with several application servers (#513). An attach that fails with subtype `invalidServer` gets a message that points there.
- the server calling HTTP services or RFC function modules as a trigger. That would cross the scope guardrail in `README.md`. A build may provide it as a `DebugTriggerer`.

## Tool surface

All debug tools take an optional `user`, defaulting to the logon user configured for the active system. If the active system has no configured user (OAuth2), the tools reject a call without `user`. Tools return typed results with `WithOutputSchema`. Failures go through `errorResult`, never through a status value.

### `debug_run` (new; `debug_start` is removed)

Input:

```
breakpoints:      [{object_uri, line}]   // 1–30, source URIs (…/source/main or …/includes/…)
user?:            string
trigger: {
  kind:   "unit_tests" | "manual" | "gui"
  object_uri?: string                     // unit_tests: object whose tests run (default: the object of the first breakpoint)
  target? {                               // gui only
    type:   "report" | "transaction" | "function_module" | "class_method"
    name:   string                        // program, tcode, function module, CLASS=>METHOD
    inputs? {string: string}              // free-form: selection screen, parameters
  }
}
timeout_seconds?: listener budget of the run (default 300, max 600)
```

**Kinds:**
- `unit_tests`: the server runs the tests.
- `manual`: the server only listens. Someone else starts the run: a person, an agent, Postman, a frontend or another system.
- `gui`: a SAP GUI dialog run, started by a `DebugTriggerer` if the build has one, and otherwise by following the returned instructions.

For `unit_tests`, `user` must be the logon user, because the tests run as that user. Any other value is rejected; the comparison ignores case. If the logon user is not known (OAuth2), the value is accepted unchecked and the run state carries a hint.

**Behaviour:**
1. If a run is active, clean it up first (see "Cleanup").
2. Set **all** breakpoints in **one** request (`scope="external"`).
   - If any breakpoint is rejected, delete the ones that were set and fail with the rejected breakpoint named.
   - A result of `errorKind="existing"` counts as set.
   - `object_type`/`object_name` are derived from the URI path until Hochfrequenz/adtler#200 confirms they may be omitted.
   - The object URI is the part before `/source/` or `/includes/`.
3. Start the run's background work (see "Run state and concurrency").
4. If the server triggers (`unit_tests`, or `gui` with a triggerer), wait up to 15 s for the run state to change. Otherwise return at once.
5. Return the run state.

### `debug_wait` (new)

Input:
- `since_version?`: the `version` of the last state the caller saw. Without it, `debug_wait` returns the current state at once.
- `rearm?`: see below.
- `timeout_seconds?`: default 45, kept below common per-call client limits.

The tool waits until the run's `version` is greater than `since_version`, or until its own timeout expires. Then it returns the run state. Every transition increments `version`, so concurrent waiters each see every change. A cancelled `debug_wait` does not cancel the run.

**Re-arming.** `rearm: true` starts the next listening window, inside the remaining run budget, when no listener is running and no debuggee is attached:
- for `manual` and `gui`: after `timeout`, or after the debuggee has ended;
- for `unit_tests`: never. A new test run means a new `debug_run`.

### Result: `DebugRunState` (both tools)

```
version:         int       // increments on every transition
status:          "listening" | "attaching" | "attached" | "ended" | "timeout" | "stopping" | "stopped"
end_reason?:     "detached" | "completed" | "terminated" | "no_hit" | "attach_failed" | "idle_detached"   // status ended
breakpoints:     [{object_uri, line, id, scope}]
debuggee_id?:    string
position?:       {program, include, line, source_uri, source_excerpt}   // attached
trigger?: {
  kind, state: "pending" | "running" | "done" | "failed",
  error?:        string,
  unit_tests?:   <adtler unit-test result>                       // unit_tests, when done
}
instructions?:   {steps: [string], notes: [string], user}         // manual / gui without triggerer / trigger failed
listening_until?: timestamp
hint?:           string
```

The status values mean:
- **`listening`**: the listener is waiting.
- **`attaching`**: a hit was caught and the background attach is in flight.
- **`attached`**: the debuggee is attached; in-attempt tools work.
- **`ended`**: no debuggee any more, for the reason in `end_reason`:
  - `detached`: `detachDebugger` was called;
  - `completed`: the debuggee ran to completion;
  - `terminated`: `terminateDebuggee` was called;
  - `no_hit`: the trigger finished without a hit, so the breakpoint line was not executed;
  - `attach_failed`: the background attach failed (`hint` carries the error);
  - `idle_detached`: detached after the idle limit (see "Run state").
- **`timeout`**: the listener budget is used up.
- **`stopping`**, **`stopped`**: cleanup is running or done.

`position.source_excerpt` holds a few lines around the current line, read from `source_uri`. The include-to-URI mapping and the parsing of stack positions belong in adtler (see "Delivery").

### Changes to existing tools

| Tool | Change |
|---|---|
| `debug_step` | Returns `position` instead of raw XML. Its description recommends `detachDebugger` to end a session and explains the two signals for "debuggee ended" (see "Facts"). |
| `debug_get_variable` | Optional `expand: true` returns the children of structures, object attributes and `REF->*`. Optional `offset`/`limit` returns table rows, with a hard cap of 100 rows per call, and only while a debuggee is halted. The description states that this is for debugging a halted program, not for retrieving data (scope guardrail). Scalars are read as today. |
| `debug_set_breakpoint` | Needs an active run; without one it fails with "start a run with debug_run". During `attaching` it fails with "attach in progress; call debug_wait". While attached: `scope="debugger"`, sent in the stateful session. Otherwise all external breakpoints of the run are deleted and the full list, including the new one, is set again, so the stored IDs stay correct on both releases. If setting the list fails after the delete, the previous list is set again; if that fails too, the call fails and names the breakpoints that are no longer set. |
| `debug_remove_breakpoint` | `DELETE …/breakpoints/<id>`, using the scope stored with that breakpoint. |
| `debug_stop` | Runs the cleanup and reports what it removed and what it could not remove. #563's branch that stops a listener "left behind by an earlier server process" is removed: with a per-process IDE ID it can no longer match. |
| `debug_attach` | **Removed.** The background attach takes every hit; a manual attach would bypass the run state, and SAP rejects a second attach in an attached session. |
| `debug_get_sessions` | Unchanged; works in any state. |
| `debug_get_stack`, `debug_set_watchpoint` | Unchanged; in-attempt tools. |

### Trigger extension

```go
// DebugTriggerer starts a run so that the external breakpoints of user are hit.
// Optional: checked by type assertion on the BlackMagicClient, so existing
// implementations keep compiling. Called only for trigger.kind "gui", after the
// listener is running, with a context that the run's cleanup does not cancel.
type DebugTriggerer interface {
    TriggerDebugRun(ctx context.Context, system, user string, t DebugTarget) error
}
```

How a triggerer starts the run is up to the build. It must start the run in a way that the user's external debugging applies to. If it returns an error, the trigger becomes `failed`, and the run state carries the instructions as a fallback.

## Run state and concurrency

**Ownership and locks**
- A run belongs to the debug session from #563, which is bound to a user and a system. `debugSessions` holds at most one active run.
- `open` (session replaced for another user or system) and `take` (`debug_stop`) run the run's cleanup before they return.
- Three locks, always taken in this order and never the reverse:
  1. the **run-start lock**: serialises `debug_run`, `debug_stop`, session replacement, the idle detach and the shutdown hook. It is held across their HTTP calls, so cleanup and an idle detach can never detach twice;
  2. **`debugSessions.mu`**;
  3. **`run.mu`**.
- No HTTP call is made while `debugSessions.mu` or `run.mu` is held.

**IDE ID.** Each server process uses its own `ideId`, generated at startup (32 hex characters) and passed to `adt.NewDebugSession`. Breakpoints and listeners are keyed by the logged-on user, `requestUser` and `ideId`, so two aibap.mcp processes of the same user no longer overwrite (816) or mix (750) each other's breakpoints. The price is that a new process cannot delete breakpoints a crashed process left behind; what happens to those is listed under "Unknown". The guide says so.

**State.** The run state is one struct behind `run.mu`:
- status and `version` (one counter per process, increasing across runs, so a `since_version` from an earlier run never blocks);
- breakpoints with their IDs and scopes;
- debuggee ID and position;
- trigger state;
- instructions;
- a `changed` channel that is closed and replaced on every transition (broadcast).

Waiters (`debug_run` step 4, `debug_wait`) compare `version` and wait on `changed`. Nothing is consumed, so concurrent waiters cannot steal an event from each other.

**Contexts**
- **Run context:** derived from `context.Background()`, not from the tool call. The listener uses it. Cleanup cancels it.
- **Attach context:** its own 30 s deadline, *not* derived from the run context, so that cleanup cannot abort an attach that SAP may already be completing (see "Cleanup").
- **Trigger context:** `context.Background()` with the listener budget plus 30 minutes as deadline. This holds for both `RunUnitTests` and a `DebugTriggerer`. Cleanup does not cancel it. If it expires while the debuggee is halted, the trigger reports `failed` ("trigger request timed out"); the debuggee stays attached until it is detached.
- **Cleanup contexts:** fresh, sharing one total budget of 20 s.

**Listener goroutine**
- One long poll per listening window, with the window's remaining budget (at most 600 s, the per-poll cap read in the 816 source).
- On a hit, unless the run is `stopping`, it sets `attaching` and attaches with the attach context: one attempt, Hochfrequenz/adtler#196. It then reads the position and sets `attached`.
- After its attach returns, the goroutine checks for `stopping`/`stopped`. If cleanup has started meanwhile, it detaches by itself. That closes the window in which cleanup has already skipped the detach (step 3) and the attach completes later.
- An attach error sets `ended` / `attach_failed`. An `invalidServer` subtype points to #513.
- Attaching at once closes the window in which a caught debuggee waits for an attach that a serialising client might never send.

**Trigger goroutine** (`unit_tests`, and `gui` with a triggerer)
- Starts about 4 s after the listener, because the activation reaches the server asynchronously.
- `RunUnitTests` runs on a **separate isolated session, bound to the run's system**, not on the main client. The main client may hold stateful lock sessions, and `select_system` would redirect it.
- On return it sets `trigger.state` (`done` or `failed`) with the result. If there was no hit, the run becomes `ended` / `no_hit`.

**Transitions caused by the in-attempt tools.** `debug_step`, `debug_get_variable`, `debug_get_stack`, `debug_set_watchpoint` and `debug_set_breakpoint` (debugger scope) work only in `attached`. In any other status they fail with "no debuggee attached; call debug_wait". `debug_step` changes the run state:

| Step result | New state |
|---|---|
| a step that stops again (`stepInto`, `stepOver`, `stepReturn`, `stepContinue` to a next breakpoint) | stays `attached`, new `position` |
| `detachDebugger` | `ended` / `detached` |
| `terminateDebuggee` | `ended` / `terminated` |
| a step reporting that the debuggee ended (either release signal, see "Facts") | `ended` / `completed` |
| `stepContinue` returns `400 ExceptionInvalidData` (816, end of run) and the debuggee-session list is empty | `ended` / `completed`, with a hint. With a non-empty list the error is reported and the state stays `attached` (a lost attachment also answers 400, #513) |
| a step fails without ending the debuggee (e.g. the `stepContinue` timeout after a GUI trigger) | stays `attached`; the error is reported with a hint to use `detachDebugger`. The idle limit or cleanup recovers |

**Idle limit.** If a debuggee stays `attached` for 10 minutes after the last in-attempt call ended (the timer pauses while a call is in flight), the server detaches it (`ended` / `idle_detached`). A client that serialises calls, or a caller that has moved on, must not hold a SAP work process indefinitely.

### Cleanup

Cleanup runs on `debug_stop`, on a new `debug_run`, when a session is replaced (#563), and in a shutdown hook (stdin EOF, SIGINT, SIGTERM). It is best effort, inside one total budget of 20 s:

1. Set `stopping`. From now on the listener goroutine starts no new attach.
2. In parallel:
   - **a.** `StopListener` and the `DELETE`s of the external breakpoints, sent from a **fresh session** with the same user, `terminalId` and `ideId`. These endpoints are keyed by those IDs, not by cookie, so a wedged debug session cannot block them. An empty `full` request is not used, because it removes nothing on 750.
   - **b.** Cancel the run context, then wait up to 10 s for the listener goroutine to exit, which includes an attach that was already in flight.
3. If the run ended up attached, in the debug session: delete the debugger-scope breakpoints (they need the attachment), then `detachDebugger`. Otherwise the program stays suspended in SAP, holding a work process, and the trigger request stays open. A "not attached" answer here counts as success, because the external deletes in step 2a may already have ended the attachment (see "Unknown").
4. Leave the trigger goroutine running. After the detach it finishes on its own, and its result goes into the discarded run.
5. Set `stopped`. `debug_stop` reports the breakpoints it could not delete and whether a detach failed.

### Errors

| Case | Result |
|---|---|
| a breakpoint is rejected | `debug_run` fails before listening; the others are deleted |
| the listener reports a conflict (another debugger listens for this user and IDE ID) | `errorResult` naming the conflict. Not observed yet: the exact status code is confirmed during implementation |
| listener budget used up | status `timeout`, with the usual causes in `hint`: other user, system program, run started before or after listening, GUI session not enabled |
| attach fails | `ended` / `attach_failed`, the error in `hint`, which says to start a new `debug_run` |
| triggerer fails | `trigger.state = failed`, instructions as a fallback |
| unit tests finished without a hit | `ended` / `no_hit` |
| no user available (OAuth2, no `user` given) | `errorResult` |

## Instructions

`instructions` is structured (`steps`, `notes`, `user`; the deadline is the run state's `listening_until`) so that a person, an agent or a script can follow it.

**`manual`**
- **Step:** "Start the run now, as user `<user>`, before `<listening_until>`: call the HTTP service or RFC function module, or run the program."
- **Notes:**
  - use a new connection, or the first request of a stateful session;
  - the run must be made as that same user;
  - breakpoints in system programs are never hit.

**`gui`**
1. **Enable the GUI session.** To choose the method, the server checks once per system whether program `RS_ADTDBG_ACTIVATE_BY_OKCODE` exists. The check is an ADT repository lookup of the program URI, not a table read. Depending on the result:
   - it exists: the OK code `/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>`;
   - it does not exist: `SADT_START_TCODE` with `D_AIE_TCODE`, `D_IDE_USER` and `D_REQUEST_USER`, Eclipse navigation off;
   - the lookup fails: both, OK code first.
2. **Start the target:**
   - **report:** SE38, name, F8, selection screen from `inputs`;
   - **transaction:** `/n<tcode>`, or directly as `D_AIE_TCODE` of `SADT_START_TCODE`;
   - **function module:** SE37, name, F8 (test environment), `inputs`, F8;
   - **class method:** SE24, name, F8 (test environment), method, `inputs`, execute.

   Always "in the same window". The transaction, function-module and class-method paths carry an "untested" note until the live acceptance confirms them.
3. **End the session** with `debug_step detachDebugger`, never `stepContinue`.

## Debugging guide

The guide (#559, PR #561) is rewritten around `debug_run` and `debug_wait`. It covers:
- triggers and instructions;
- ending with `detachDebugger`;
- breakpoints while halted;
- variables;
- system programs;
- the per-process IDE ID;
- the release notes above.

PR #561 stays open until the live acceptance has passed.

## Testing

**adtler** (#200, #201 and the additions under "Delivery"):
- **Unit tests** with httptest.
- **Integration tests** on both systems:
  - two breakpoints in one request: the run stops at A, then at B;
  - a breakpoint in debugger scope while halted;
  - after `RemoveBreakpoint`, the next run is not caught;
  - variables of each kind;
  - the unit-test run on an isolated session.

**aibap.mcp unit tests.** The transport-level fake from #563 is extended: the listener hits after a delay, the attach and stack calls answer, and so does the `RunUnitTests` endpoint. Cases:
- `unit_tests`: `attached` within `debug_run`, then `ended` with the trigger result;
- `manual`: `listening` with instructions, then `debug_wait` returns `attached`;
- `gui` without a triggerer: instructions, chosen by OK-code availability;
- `gui` with a fake triggerer: called only after the listener has started, and a triggerer error becomes `trigger.state = failed`;
- a rejected breakpoint: the others are deleted;
- `errorKind="existing"` counts as set;
- cleanup order and the 20 s budget on a hanging fake;
- session replacement and `debug_stop` during an active run;
- a hit without any waiter is attached in the background;
- a hit that arrives during cleanup, with the attach completing after more than the 10 s listener wait: a detach is still sent;
- cleanup with a debug session that hangs: `StopListener` and the external deletes still go out from the fresh session;
- every `debug_step` transition in the table above, and the idle limit;
- `debug_wait` with `since_version`;
- `debug_set_breakpoint` restoring the previous list when the re-set fails;
- two concurrent `debug_wait` calls both see `attached`;
- two concurrent `debug_run` calls do not interleave;
- `rearm`;
- unit-test `user` validation;
- a missing default user;
- the shutdown hook;
- everything under `-race`, run in WSL because the Windows host has no cgo toolchain.

**Live acceptance:** the real binary, a fresh MCP client session without prior context, only the guide and the tool descriptions, both systems.
1. `unit_tests`: hit, variables (structure, table page), step, `detachDebugger`, test result.
2. `manual`: a SOAP-RFC call to a test function module from a separate HTTP client.
3. `gui`, report target: a person follows the instructions.
4. `gui`, report target: an agent drives a GUI-automation MCP server in the same client while `debug_wait` waits. This end-to-end case has never been run.
5. A breakpoint added while halted; `debug_stop`; a control run is no longer caught.
6. The untested GUI targets (transaction, SE37, SE24), each at least once. Remove their "untested" notes, or document what fails.

## Delivery

| # | Repo | Content | Depends on |
|---|---|---|---|
| A1 | adtler | #200: breakpoints (`SetBreakpoints`, scope, `RemoveBreakpoint`, `clientId`/`errorKind`) | — |
| A2 | adtler | #201: variables | — |
| A3 | adtler | #196: attach without retry (open PR) | — |
| A4 | adtler | **new issue:** debugger position (parse the listener response and the stack into program, include, line; map the include to a source URI) | — |
| A5 | adtler | **new issue:** `RunUnitTests` on an isolated session bound to one system (as `RunClass` does) | — |
| A6 | adtler | **release** | A1–A5 |
| B1 | aibap.mcp | #563: session lifecycle (open PR) | — |
| B2 | aibap.mcp | #558: `debug_run`, `debug_wait`, run state, `DebugTriggerer`, instructions, per-process IDE ID, idle limit, shutdown hook; `debug_start` and `debug_attach` removed | B1, A1, A3–A5 |
| B3 | aibap.mcp | position in `debug_step`, variable expansion, breakpoint scopes, `debug_remove_breakpoint`, cleanup in `debug_stop` | B2, A1, A2, A4 |
| B4 | aibap.mcp | #561: the guide, after the live acceptance | B2, B3 |

- **Build and pinning:** B2 and B3 are built as drafts against a pinned adtler `main` commit, which CLAUDE.md allows as the pre-release path. Before merge they are re-pinned to the release tag.
- **References to `debug_start`:** B2 updates every one in this repo (`README.md`, `docs/debugger-investigation.md`, the guide) and in the AIBAP template repository, as the CLAUDE.md "Related projects" section requires.
- **Versioning:** removing `debug_start` and `debug_attach` and adding `debug_run` and `debug_wait` changes the tool surface, so this is a **minor** bump. B3 also changes the outputs of `debug_step` and `debug_stop`. If a release is cut between B2 and B3, each one is its own minor bump.
