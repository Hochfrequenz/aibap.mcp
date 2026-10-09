# Debugging ABAP with this server

The debug tools stop an ABAP run at a breakpoint and let you inspect and step
through it. A debugging session is one **debug run**: `debug_run` sets the
breakpoints, listens in the background for a run of one SAP user, and attaches
to the first run that hits a breakpoint. The tool calls return at once, so the
flow does not depend on the MCP client running tool calls in parallel.

**How this was tested.** The procedures below were tested through an MCP
client on SAP S/4HANA 2025 on-premise (SAP_BASIS 816) and SAP ERP 6.0 EHP8
(SAP_BASIS 750) in October 2026, with throwaway objects (#558). Exceptions are
noted where they apply, and so are points that differ between the releases.

## The flow

1. **`debug_run`** with the breakpoints, the `user` whose run is debugged
   (default: the logon user of the active system) and a `trigger`. It returns
   the run state with a `version` and a `status`.
2. **Start the run**, unless the server does it (see the triggers below). For
   `manual` and `gui` runs the state carries `instructions` (the steps for the
   person or caller); `listening_until` in the state is the time by which the
   run has to start.
3. **`debug_wait`** with `since_version` = the last `version` you saw. It
   returns as soon as the run changes, or after `timeout_seconds` (default 45)
   with the unchanged state. Call it again until `status` is `attached`. A
   `debug_wait` that times out does not stop the run.
4. **Inspect** while `status` is `attached`: `position` in the state shows
   program, include, line and a source excerpt. Use `debug_get_stack`,
   `debug_get_variable` and `debug_step`.
5. **End the debuggee with `debug_step` `detachDebugger`**, not
   `stepContinue`. The program runs on to its end, and the run ends with
   `end_reason: detached`.
6. **`debug_stop`** when done, also after a timeout or a failure. It stops the
   listener, removes the run's breakpoints, detaches a halted debuggee, and
   reports anything it could not remove.

A new `debug_run` first stops the previous one; there is one run per server
process.

## Rules that apply to every run

1. **Same SAP user.** Only a run made as `user` is caught. Breakpoints are set
   in user mode: they catch eligible runs of that user (for SAP GUI only in an
   enabled session, see below), and of no one else.
2. **Source URIs.** A breakpoint's `object_uri` is a source URI, e.g.
   `/sap/bc/adt/programs/programs/zreport/source/main` or
   `/sap/bc/adt/oo/classes/zcl_example/includes/testclasses`. The bare object
   URI is rejected.
3. **The breakpoint stops before its line executes.** A variable assigned on
   the breakpoint line is still initial. `debug_step` `stepOver` first, then
   read it.
4. **System programs are never hit.** A breakpoint in a program SAP marks as a
   system program never stops a run.
5. **One debug client per SAP user.** According to the SAP source (read on
   SAP_BASIS 816), setting breakpoints replaces the external-debugging
   activation of any other IDE for the same user. Do not debug the same user
   from Eclipse ADT and from this server at the same time.
6. **Do not leave a debuggee halted.** A halted program holds a SAP work
   process. After 10 minutes without a `debug_step`, `debug_get_stack`,
   `debug_get_variable` or `debug_set_breakpoint` call (`debug_wait` does not
   count), the server detaches it (`end_reason: idle_detached`).

## Run states

| `status` | Meaning |
|---|---|
| `listening` | Breakpoints are set; waiting for a run to hit one. |
| `attaching` | A run hit a breakpoint; the debugger is attaching. |
| `attached` | The debuggee is halted. Inspect and step. |
| `timeout` | The listening window passed without a hit. See `hint`. |
| `ended` | See `end_reason`. |
| `stopping`, `stopped` | `debug_stop` is cleaning up, or has. |

| `end_reason` | Meaning |
|---|---|
| `detached` | `detachDebugger`: the program ran on to its end. |
| `completed` | A step ran the program to its end (`debuggee_ended: true` in `debug_step`). |
| `terminated` | `terminateDebuggee` killed the program. |
| `no_hit` | The unit tests finished without hitting a breakpoint. |
| `attach_failed` | The attach failed; see `hint`. |
| `idle_detached` | Detached after 10 minutes without a debugger call (rule 6). |

After `timeout` or `ended`, `debug_wait` with `rearm: true` listens again with
the same breakpoints, within the run's remaining budget (`timeout_seconds` of
`debug_run`, default 300, max 600). This works for `manual` and `gui` runs; a
`unit_tests` run needs a new `debug_run`.

## Trigger `unit_tests`: ABAP Unit tests (no person needed)

`trigger: {"kind": "unit_tests"}` makes the server run the ABAP Unit tests of
`trigger.object_uri` (default: the object of the first breakpoint) about four
seconds after the listener starts. Put the breakpoint on a line a test
executes: in a test method, or in code a test calls.

- The tests run as the logon user of the active system, so `user` must be that
  user.
- `debug_run` waits up to 15 seconds and usually returns `attached` already.
- The test result arrives in `trigger.unit_tests` once the tests have
  finished, i.e. after the debuggee was detached.
- If the tests finish without a hit, the run ends with `no_hit`.
- If the test run itself fails, `trigger.state` is `failed` with
  `trigger.error`, and the state carries `manual` instructions instead.

## Trigger `manual`: someone else starts the run

`trigger: {"kind": "manual"}` when the run comes from outside: an HTTP call to
an ICF service, an RFC call of a function module, or a program someone runs.
`debug_run` returns `listening` at once, with `instructions`. Start the run
now, as `user`, before `listening_until`, then `debug_wait`.

- Tested through `debug_run`: a SOAP-RFC call of a function module. An HTTP
  call to an ICF handler was caught in earlier tests of the underlying ADT
  flow, not yet through `debug_run`.
- Use a new connection, or the first request of a stateful session.

## Trigger `gui`: a SAP GUI dialog run

`trigger: {"kind": "gui", "target": {"type": ..., "name": ..., "inputs": {...}}}`
for a program a person starts from SAP GUI. `target.type` is `report`,
`transaction`, `function_module` (SE37 test environment) or `class_method`
(`name` = `CLASS=>METHOD`, SE24 test environment). `inputs` are the
selection-screen or parameter values.

`debug_run` returns `listening` at once, with `instructions` written for the
person at the SAP GUI. **Pass the steps on in full and end your turn**; call
`debug_wait` only when the person says they have started the run. Some clients
do not show your messages while a tool call is running, so a `debug_wait`
started before the person has read the steps leaves them without instructions.

A SAP GUI session does not trigger external breakpoints by default. The person
first enables the session for ADT external debugging, then starts the program
**in the same window**. Another window of the same user is not enabled
(tested on SAP_BASIS 750; according to the SAP_BASIS 816 source the same holds
there).

- **SAP_BASIS 816:** the OK code `/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>`
  in the command field. The status bar says the debugger activation was
  resynchronized. Then `/n<transaction>` in the same window.
- **SAP_BASIS 750:** the OK code is not available there. Transaction
  `SADT_START_TCODE` instead, with **both** the transaction field
  (`D_AIE_TCODE`) and the IDE user field (`D_IDE_USER`, = `user`) filled,
  Eclipse navigation off, then F8. With only the transaction filled, the run
  was not caught. `SADT_START_TCODE` opens the transaction itself, so the
  person continues there. The screen is built for Eclipse and shows only
  technical field names; tell the person which field is which. Its fields
  cannot be passed in the command field.

The server checks which of the two the system supports and writes the matching
steps. Further points for the person:

- While the debugger is attached, the SAP GUI window stays busy. It continues
  after `detachDebugger`.
- The SE37 and SE24 test environments convert input values to upper case
  unless their upper/lower case option is set.
- In SE24, the method is started with the "Execute Method" icon in its line.
- To debug the next run, `debug_wait` `rearm: true`, then the person enables
  the session and starts the program again.

Not in the standard build: a server build that can drive SAP GUI itself
starts the run without a person;
`debug_run` then waits up to 15 seconds like a `unit_tests` run.

## Breakpoints during a run

- `debug_set_breakpoint` adds a breakpoint to the active run. While the
  debuggee is halted, it is set in the attached debugger (scope `debugger`);
  SAP drops those breakpoints when the debugger detaches, so a rearmed run
  keeps only the external ones. Otherwise all external breakpoints of the run
  are set again together with the new one, and a listening run picks it up.
  While `attaching` it is refused; call `debug_wait` first.
- `debug_remove_breakpoint` removes one by the `id` in the run state. An
  external breakpoint cannot be removed while the debuggee is attaching or
  attached: on SAP_BASIS 750 that request detaches the debugger. Remove it
  after `detachDebugger`.

## Ending a session

Use `detachDebugger`. `stepContinue` goes on to the next breakpoint; past the
end of the run it fails on both releases (SAP_BASIS 816: 400
`ExceptionInvalidData`; SAP_BASIS 750: `AdiFailed` /
`CX_TPDAPI_DEBUGGEE_ENDED`). `debug_step` reports that as
`debuggee_ended: true` with `end_reason: completed`, but after a SAP GUI
trigger `stepContinue` hung instead (SAP_BASIS 816, one run). A failed step leaves the debuggee
attached: detach it, or call `debug_stop`.

## Troubleshooting

- **`timeout`, but the program ran:** the run was made as another user, before
  or after the listening window, in a system program, or (gui) in a SAP GUI
  session or window that was not enabled. On SAP_BASIS 750, check that
  `D_IDE_USER` was filled.
- **`no_hit` with `unit_tests`:** no test executed the breakpoint line, or the
  breakpoint is in a system program.
- **`debug_get_variable` returns an initial value right after the hit:** the
  variable is assigned on the breakpoint line (rule 3). Step over first.
- **`attach_failed`:** a failed attach uses up the caught run. Start a new
  `debug_run` (for `manual` and `gui`, `debug_wait` `rearm: true` also works),
  then start the run again.

## Not tested yet

- Systems with several application servers.
- A `user` different from the logon user, for `manual` and `gui` runs.
- Releases other than SAP_BASIS 750 and 816. The server tries the OK code
  first when it cannot tell which one the system supports.
