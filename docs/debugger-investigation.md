# Debugger notes for contributors

How to *use* the debug tools is in the debugging guide, which the server ships
as the MCP resource `sap-adt://guides/debugging` (source:
[`tools/guides/debugging.md`](../tools/guides/debugging.md)). The design of the
run model (`debug_run` / `debug_wait`) is in
[`docs/superpowers/specs/2026-10-06-debug-run-design.md`](superpowers/specs/2026-10-06-debug-run-design.md).
This file records how the ADT debugger behaves underneath, for anyone changing
the debug tools or debugging an ADT handler. Status as of 2026-10-09.

Observed behaviour was tested on SAP S/4HANA 2025 on-premise (SAP_BASIS 816)
and SAP ERP 6.0 EHP8 (SAP_BASIS 750), through this server and with adtler's
debug client (#435, #513, #558, Hochfrequenz/adtler#200). Details that come
from reading the ABAP handler source on SAP_BASIS 816 are marked as such.

## The flow over HTTP

1. **Set the breakpoints** (`POST /sap/bc/adt/debugger/breakpoints`),
   `scope="external"`, `debuggingMode="user"`, with the **complete list in one
   request**. Sent one at a time, a second breakpoint replaces the first on
   SAP_BASIS 816 and is added on SAP_BASIS 750; an empty list clears all on
   816 and nothing on 750. Only the complete list behaves the same on both
   (Hochfrequenz/adtler#200). `syncMode` does not change this.
2. **Start the listener** (`POST /sap/bc/adt/debugger/listeners`, long poll)
   with `Accept: application/vnd.sap.as+xml`. With `application/xml` the
   listener answers 406 when a debuggee arrives.
3. **Trigger** the run while the listener waits: an ABAP Unit run over ADT, an
   HTTP or RFC call, or a SAP GUI session enabled for external debugging.
4. **Attach** (`POST /sap/bc/adt/debugger?method=attach&debuggeeId=<id>`),
   then stack, variables and steps. These run in one stateful ADT session
   (`X-sap-adt-sessiontype: stateful`), which adtler's `DebugSession`
   maintains. Step, stack and variable requests work over HTTP in that session;
   RFC is not needed.

The server runs the listener and the attach in a goroutine per run, so no tool
call blocks while it waits (#558).

## Session rules found live

- **SAP serialises requests within one stateful session.** On SAP_BASIS 816 an
  external breakpoint request on the run's debug session hung while the
  listener was waiting on the same session. External breakpoint requests made
  while listening therefore go through a separate session with the same user,
  terminal ID and IDE ID.
- **An external breakpoint request while the debuggee is halted detaches the
  debugger** on SAP_BASIS 750 (the next step fails with `AdiFailed`,
  `CX_TPDAPI_NOT_ATTACHED`). While halted, breakpoints are set with
  `scope="debugger"` in the stateful session instead, and external ones are
  not removed. SAP drops debugger-scope breakpoints on detach.
- **Breakpoints outlive the session.** The listener's end does not remove
  them; `debug_stop` deletes each one.

## Corrections to the 2026-03 notes

Earlier versions of this file stated four things that turned out wrong:

- *"External breakpoints only trigger in HTTP/ICF sessions, not SAP GUI
  (message ED702)."* A SAP GUI session triggers them once it has been enabled
  for ADT external debugging and the program runs in that same session. ED702
  is a conflict check between GUI and external breakpoints, not a routing rule.
  `SADT_START_TCODE` *does* enable the session on SAP_BASIS 750, provided the
  IDE user field (`D_IDE_USER`) is filled.
- *"Step, variables and stack do not work over HTTP; RFC is required."* They
  work in a stateful HTTP session.
- *"ABAP Unit runs cannot be debugged from MCP."* An ABAP Unit run over ADT is
  caught reliably. The earlier misses came from the MCP client running the
  listener and the unit tests one after the other, so the tests only started
  after the listener had timed out (#558).
- *"`syncMode="full"` persists breakpoints."* SAP ignores it for external
  breakpoints; see the flow above.

## Server-side details (from reading the ABAP handler source, SAP_BASIS 816)

- **User-mode breakpoints** are stored per SAP user. Activating them replaces
  the external-debugging activation of any other IDE for the same user. The
  activation reaches the ICF layer asynchronously, so a trigger fired in the
  same instant may miss; the server starts its own triggers about four seconds
  after the listener.
- **ABAP Unit over ADT** runs the tests in a separate task of the same user
  (run mode "external"); that task is eligible for external debugging.
- **SAP GUI** sessions are enabled per session. The enabling step stamps the
  current GUI session; another session of the same user stays unaffected.
- **Attach failures** surface as HTTP 500 `AdiFailed` (exception class
  `CX_TPDA_ADT_SER_ADI_FAILED`). The server consumes the pending debuggee
  before it attaches, so retrying an attach after such a failure cannot
  succeed (#513, Hochfrequenz/adtler#196).
- **Ending:** past the end of a run, `stepContinue` answers SAP_BASIS 816 with
  400 `ExceptionInvalidData` and SAP_BASIS 750 with `AdiFailed` /
  `CX_TPDAPI_DEBUGGEE_ENDED`. adtler maps the 750 answer to
  `DebuggeeEndedError`; this server treats the 816 answer as the end of the
  run when no debuggee remains. After a
  SAP GUI trigger, `stepContinue` hung until the client timeout;
  `detachDebugger` ended the session within about a second.

## Open questions

- Systems with several application servers: debugger requests after attach
  likely need the `X-sap-adt-server-instance` header naming the debuggee's
  server (untested, #513).
- Whether a SAP GUI shortcut can prefill the `SADT_START_TCODE` fields.
