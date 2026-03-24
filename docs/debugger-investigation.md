# Debugger Investigation Notes (2026-03-25)

## What works (verified against hfq.sap.msp.local:8100)

- **Setting breakpoints**: POST `/debugger/breakpoints` with `syncMode="full"` persists breakpoints in SAP shared memory
- **Listener**: POST `/debugger/listeners` with `Accept: application/vnd.sap.as+xml` returns debuggee session info when a breakpoint is hit
- **Triggering execution**: The ABAP Unit test runner (`/abapunit/testruns`) executes code in the same SAP session, which triggers breakpoints
- **Attach**: POST `/debugger?method=attach&debuggeeId=<id>` successfully attaches to the debuggee, returns debug session ID, process ID, and reached breakpoints

## What doesn't work over HTTP

- **Step/Variables/Stack**: These endpoints require `get_attached_session()` which relies on being in the **same work process** as the attach call. Over HTTP, each request gets a new work process (stateless). Eclipse ADT uses RFC (JCo) which is inherently stateful.

## Key findings

### syncMode=full is essential
Without `syncMode="full"` in the breakpoint request, the breakpoint lives only for the duration of the HTTP request. With it, SAP calls `set_dummy_breakpoint()` which registers the external debugger in shared memory. Eclipse detected our breakpoint as a "conflicting breakpoint" — proving it persists.

### Accept header matters for listener
- `application/xml` → listener returns 406 when debuggee attaches
- `application/vnd.sap.as+xml` → listener returns full debuggee info (ASX XML with DEBUGGEE_ID, program, line, etc.)

### HTTP vs RFC
Eclipse ADT communicates via `SADT_REST_RFC_ENDPOINT` (RFC function module), not HTTP. The REST handler code is identical, but RFC sessions are stateful — the work process is kept for the duration of the connection. HTTP/ICF is stateless — each request gets a new work process.

The `get_attached_session()` call in `CL_TPDA_ADT_RES_ACTIONS` (step) and `CL_TPDA_ADT_RES_VARIABLES` looks up the debug session in the **current work process**, which only works when the work process is the same as the one that did the attach (RFC stateful session).

## Proven debug flow (integration tested)

1. **Set breakpoint** with `syncMode=full`, `scope=external`, `debuggingMode=user`
2. **Start listener** (long poll, blocks until breakpoint hit or timeout)
3. **Execute code** via unit test runner (same HTTP cookie jar)
4. **Listener returns** debuggee info (DEBUGGEE_ID, program, line)
5. **Attach** to debuggee session

## Next steps: RFC support

To enable step/variables/stack, we need stateful sessions via RFC:
- Use Go SAP RFC library (gorfc or similar) to call `SADT_REST_RFC_ENDPOINT`
- Package our REST requests as `SADT_REST_REQUEST` structures
- This gives us stateful sessions where attach + step + variables all share the same work process
