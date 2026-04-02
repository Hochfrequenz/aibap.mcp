# Debugger Investigation Notes (2026-03-25)

## What works (verified against hfq.sap.msp.local:8100)

- **Setting breakpoints**: POST `/debugger/breakpoints` with `syncMode="full"` persists breakpoints in SAP shared memory
- **Listener**: POST `/debugger/listeners` with `Accept: application/vnd.sap.as+xml` returns debuggee session info when a breakpoint is hit
- **Triggering execution**: The ABAP Unit test runner (`/abapunit/testruns`) executes code in the same SAP session, which triggers breakpoints
- **Attach**: POST `/debugger?method=attach&debuggeeId=<id>` successfully attaches to the debuggee, returns debug session ID, process ID, and reached breakpoints
- **Listener timeout fix**: `StartListener` must temporarily increase the HTTP client timeout beyond the SAP listener timeout (default 30s is too short for long-poll)

## What doesn't work over HTTP

- **Step/Variables/Stack**: These endpoints require `get_attached_session()` which relies on being in the **same work process** as the attach call. Over HTTP, each request gets a new work process (stateless). Eclipse ADT uses RFC (JCo) which is inherently stateful.
- **SAP GUI as trigger**: External ADT breakpoints are NOT triggered by GUI (dialog) sessions — only by HTTP/ICF sessions. This is by design (SAP message ED702: external breakpoints and GUI breakpoints are mutually exclusive).

## What doesn't trigger external breakpoints (tested 2026-03-25)

| Method | Result | Notes |
|--------|--------|-------|
| SAP GUI (manual SE38) | ❌ Breakpoint not hit | ED702: GUI deactivates external BPs |
| sapshcut.exe with password | ❌ Breakpoint not hit | Separate dialog session |
| WebGUI (`/sap/bc/gui/sap/its/webgui`) | ❌ Breakpoint not hit | Own ICF session, not shared |
| SAP GUI with reentrance ticket | ❌ Breakpoint not hit | SSO login works, but still separate dialog session |
| SADT_START_TCODE with TID/IDE_ID params | ❌ Breakpoint not hit | Parameters are for navigation, not debug binding |
| Unit test runner (HTTP) | ✅ Breakpoint hit | Same HTTP/ICF session |

## Key findings

### syncMode=full is essential
Without `syncMode="full"` in the breakpoint request, the breakpoint lives only for the duration of the HTTP request. With it, SAP calls `set_dummy_breakpoint()` which registers the external debugger in shared memory. Eclipse detected our breakpoint as a "conflicting breakpoint" — proving it persists.

### Accept header matters for listener
- `application/xml` → listener returns 406 when debuggee attaches
- `application/vnd.sap.as+xml` → listener returns full debuggee info (ASX XML with DEBUGGEE_ID, program, line, etc.)

### HTTP vs RFC — the debug context is bound to the connection layer
Eclipse ADT communicates via `SADT_REST_RFC_ENDPOINT` (RFC function module), not HTTP. The REST handler code is identical, but RFC sessions are stateful — the work process is kept for the duration of the connection. HTTP/ICF is stateless — each request gets a new work process.

The `get_attached_session()` call in `CL_TPDA_ADT_RES_ACTIONS` (step) and `CL_TPDA_ADT_RES_VARIABLES` looks up the debug session in the **current work process**, which only works when the work process is the same as the one that did the attach (RFC stateful session).

### Reentrance tickets work for SSO but not debug context
The ADT endpoint `/sap/bc/adt/security/reentranceticket` returns a MYSAPSSO2 ticket that can open a SAP GUI session without password. But the resulting GUI session is still a separate dialog process — it does NOT share the debug context with the HTTP/ADT session.

## Proven debug flow (integration tested)

1. **Set breakpoint** with `syncMode=full`, `scope=external`, `debuggingMode=user`
2. **Start listener** (long poll, blocks until breakpoint hit or timeout)
3. **Execute code** via unit test runner (same HTTP cookie jar)
4. **Listener returns** debuggee info (DEBUGGEE_ID, program, line)
5. **Attach** to debuggee session

## Next steps: RFC support

The debug context (breakpoints, listener, attach, step, variables) is bound to the **connection layer**, not to transaction parameters or SSO tickets. Eclipse uses JCo (RFC) for this — we need RFC too.

### What RFC enables
1. **Program execution**: Call `SADT_START_TCODE` via RFC embedded GUI → report runs in RFC session → breakpoints trigger
2. **Stateful debugging**: Attach + Step + Variables + Stack all share the same work process
3. **Same REST API**: `SADT_REST_RFC_ENDPOINT` accepts the same REST requests we already build — just tunneled through RFC instead of HTTP

### Implementation plan
1. Add Go SAP RFC library (`gorfc` or similar — requires SAP NW RFC SDK)
2. Implement `SADT_REST_RFC_ENDPOINT` caller — package our existing REST requests as `SADT_REST_REQUEST` structures
3. Implement program execution — call `SADT_START_TCODE` or equivalent via RFC
4. Migrate debugger operations (attach, step, variables, stack) from HTTP to RFC
5. Keep breakpoints + listener on HTTP (they work fine there)

### Alternative: custom Z function module
If RFC SDK integration is too complex, a simpler alternative:
- Deploy a Z function module (RFC-enabled) that does `SUBMIT <report> AND RETURN`
- Call it via HTTP SOAP/RFC endpoint (`/sap/bc/soap/rfc/`)
- This would execute the report in an HTTP context, triggering breakpoints
- Downside: requires deploying custom code to each SAP system
