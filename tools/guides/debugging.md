# Debugging ABAP with this server

The debug tools stop a running ABAP program at an external breakpoint and let
you inspect and step through it. They do not start the program. Something else
has to run it, at the right moment, as the right SAP user. This guide describes
the triggers that are known to work and the ones that do not.

**How this was tested.** The procedures below were tested on SAP S/4HANA 2025
on-premise (SAP_BASIS 816) and SAP ERP 6.0 EHP8 (SAP_BASIS 750), with the
debug client of the library this server is built on (the same requests these
tools send), in late September and early October 2026. They have **not yet
been run end to end through an MCP client** with `debug_start` as the
listener. Points tested on one system only, or derived from reading SAP
source code, are marked as such.

## Rules that apply to every trigger

1. **Start the listener first, then trigger, with a short pause.**
   `debug_start` sets the breakpoint and then waits (up to `timeout_seconds`,
   default 60) for a program to hit it. The program has to start while
   `debug_start` is still waiting. A run that starts after `debug_start` has
   returned is never caught. The tests started the trigger about 4 seconds
   after the listener; the activation reaches the server asynchronously, so a
   trigger fired at the very same instant may be missed.
2. **Same SAP user.** The program must run under the SAP user passed as `user`
   to `debug_start`. Breakpoints are set in "user" mode: they catch eligible
   runs (see the triggers below) by that user, and only by that user. In all
   tests, this user was also the SAP user this server logs on with. A
   different `user` than the server's logon user is untested.
3. **The user is fixed per server process.** The first `debug_start` creates
   the debug session for its `user`; later calls with a different `user` keep
   using the first one. To change the user, or to recover a debug session that
   no longer answers, restart the MCP server.
4. **Use the source URI.** `object_uri` is the source URI, e.g.
   `/sap/bc/adt/programs/programs/zreport/source/main`. The bare object URI is
   rejected. `object_type` is the ADT type, e.g. `PROG/P`.
5. **The breakpoint stops before its line executes.** A variable that is
   assigned on the breakpoint line is still initial. Call
   `debug_step` with `stepOver` before reading it with `debug_get_variable`.
6. **One debug client per SAP user.** According to the SAP source (read on
   SAP_BASIS 816), setting a breakpoint replaces the external-debugging
   activation of any other IDE or listener for the same user. Do not debug the
   same user from Eclipse ADT and from this server at the same time.
7. **Always finish with `debug_stop`**, also after a timeout or a failure. It
   stops the listener.

After `debug_start` returns `status: "attached"` with a `debuggee_id`, pass that
ID to `debug_attach`. Then use `debug_get_stack`, `debug_get_variable` and
`debug_step`.

## Trigger A: ABAP Unit tests (no SAP GUI needed)

Put the breakpoint on a line that a unit test executes: in the test method of
a program's local test class (tested), or in code that test calls. While
`debug_start` is waiting, run the unit tests that execute that line
(`run_unit_tests`). The tests run as the SAP user this server logs on with;
pass that user to `debug_start`. If you do not know it, ask the person you are
working for.

- Caught 4 out of 4 times on both systems, usually within a second.
- Ending: `debug_step` `stepContinue` lets the test finish. With the library
  version this server currently ships, `debug_step` reports this as an error,
  `SAP ADT error 500 (AdiFailed)`, with a message saying the debuggee session
  was stopped (in the logon language). That error means the test ran to
  completion, not that something failed. A later version reports it as
  `debuggee_ended: true`.
- `run_unit_tests` currently gives up after 30 seconds. If you stay at the
  breakpoint longer than that, the debug session continues, but
  `run_unit_tests` returns a timeout error instead of the test result. Run
  the unit tests again afterwards to get the result.

**Client limitation.** This needs `debug_start` and `run_unit_tests` to run at
the same time. Some MCP clients run tool calls one after another, even when
they are issued together in one turn. In such a client, `run_unit_tests` only
starts after `debug_start` has already timed out, and nothing is caught:
`debug_start` returns `timeout` while the unit tests pass normally. There is no
workaround inside such a client yet; use trigger B with a person at the SAP
GUI instead.

## Trigger B: a program run from SAP GUI

A SAP GUI session does not trigger external breakpoints by default. The GUI
session that runs the program has to be enabled for ADT external debugging,
and the program has to run **in that same GUI session**. Another session
(window) of the same user was not caught (tested on SAP_BASIS 750).

`debug_start` blocks while it waits, so you cannot give instructions once it
is running. Do it in this order:

1. **Brief the person first**: the program to run, and the exact enable step
   for their release (below). Tell them to wait until you say the listener is
   running, then to do the enable step and start the program in the same GUI
   window. If your client shows the person your message before the tool call
   runs, that is enough; otherwise ask them to confirm they are ready.
2. **Call `debug_start`** with a generous `timeout_seconds`, e.g. 180 to 300,
   since the steps are done by hand. Your MCP client may have its own,
   shorter limit for a single tool call.
3. **The person enables the GUI session**, after the breakpoint is set:
   - **SAP_BASIS 816 (S/4HANA 2025):** enter the OK code
     `/H_REACTIVATE_EXTD_DBG KIND=USER USER=<user>` in the command field and
     press Enter. The status bar shows "Debugger activation with external IDE
     was resynchronized".
   - **SAP_BASIS 750 (ECC 6.0 EHP8):** the OK code is not available there
     ("This function is not possible"). Run transaction `SADT_START_TCODE`
     instead, with:
     - Transaction (`D_AIE_TCODE`) = `SE38`
     - IDE user (`D_IDE_USER`) = `<user>`
     - Request user (`D_REQUEST_USER`) = `<user>`
     - Eclipse navigation: unchecked

     Press Enter. This lands in SE38, in the now-enabled session. Fill in
     `D_IDE_USER`: according to the SAP source, the enable step is skipped
     when it is empty (not tested).
   - Other releases: untested. Try the OK code first; if the system answers
     "This function is not possible", use `SADT_START_TCODE`.
4. **The person starts the program** in the same window (e.g. `/nSE38`,
   program name, F8; on SAP_BASIS 750 they are already in SE38). The GUI
   then appears to hang; that is the program waiting at the breakpoint.
5. `debug_start` returns `attached`. Continue with `debug_attach`.

The tests enabled the session again before every run. Whether one enable step
is enough for several runs has not been tested.

**End a GUI-triggered session with `debug_step` `detachDebugger`**, not
`stepContinue`. `detachDebugger` returned within about a second on both
systems, and the program then ran to completion in the GUI. `stepContinue`
hung for 30 seconds in this case, and afterwards the debug session no longer
answered (see rule 3 for recovery).

## What does not trigger

Do not spend attempts on these:

- A SAP GUI session that was not enabled as described in trigger B.
- A different GUI session of the same user than the one that was enabled
  (SAP_BASIS 750).
- `run_class` (an `IF_OO_ADT_CLASSRUN` class): not caught in the one attempt
  made (SAP_BASIS 750).
- Any trigger started after `debug_start` has returned.

## Not tested yet

- The whole flow end to end through an MCP client (see the top of this guide).
- Breakpoints in global classes (only programs and their local test classes
  were tested).
- A `user` different from the SAP user this server logs on with.
- Systems with several application servers.
- `SADT_START_TCODE` with the field values above on S/4HANA.

## Troubleshooting

- **`debug_start` returns `timeout`, but the program ran:** the program ran
  before or after the listener was waiting, under a different SAP user, or in
  a GUI session that was not enabled. Check rules 1 and 2 first.
- **`debug_attach` fails with `500 AdiFailed`:** do not retry it; the pending
  debuggee is used up by the failed attempt. Call `debug_stop`, then start
  over with `debug_start` and a new trigger.
- **`debug_get_variable` returns an empty value right after the catch:** the
  variable is assigned on the breakpoint line (rule 5). Step over first.
- **A step call hangs for about 30 seconds and the session stops answering:**
  see "End a GUI-triggered session" above and rule 3.
