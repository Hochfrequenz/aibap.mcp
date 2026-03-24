# Debugger Investigation Notes (2026-03-25)

## What works

- **Setting breakpoints via API**: POST `/sap/bc/adt/debugger/breakpoints` with `scope=external`, `debuggingMode=user` returns a valid breakpoint ID
- **XML format**: `adtcore:uri`, `adtcore:type`, `adtcore:name` attributes are correct (custom MarshalXML needed)
- **Listener endpoint**: POST `/sap/bc/adt/debugger/listeners` blocks correctly until timeout
- **Breakpoint validation**: SAP correctly rejects non-executable lines (DATA declarations, REPORT statement)

## What doesn't work yet

- **Breakpoints don't persist**: GET after SET returns empty body. The breakpoint is a session-scoped breakpoint, not a true external breakpoint
- **Listener doesn't catch events**: Even when a GUI external breakpoint triggers the GUI debugger, the ADT listener gets nothing
- **No program execution**: The ADT HTTP session can't execute ABAP programs — there's no `SUBMIT` equivalent via REST

## Key findings from SAP source code

### CL_TPDA_ADT_RES_BREAKPOINTS
- `scope=external` → calls `init_static()` → `get_static_bp_services()`
- `debuggingMode=user` → calls `set_external_bp_context_user(i_ide_user=sy-uname, i_request_user=<user>)`
- Breakpoint is created via `ref_bp_factory->create_line_breakpoint()` then `submit()` then `initialize()`
- Despite returning an ID, the breakpoint doesn't survive beyond the HTTP request

### CL_TPDA_ADT_RES_LISTENERS
- `debuggingMode=user` → calls `start_listener_for_user(i_request_user, i_ide_user, i_ide_id, i_timeout)`
- This is a blocking call (long poll)
- On success, calls `post_get_dbgee_sessions` to return session info
- Timeout returns empty body (0 bytes), no error

### SADT_REST_RFC_ENDPOINT
- RFC-enabled function module that handles ALL Eclipse ADT requests
- Takes `SADT_REST_REQUEST` structure (URI, headers, body) and returns `SADT_REST_RESPONSE`
- Eclipse uses JCo (Java Connector) to call this via RFC, not HTTP
- The FM delegates to the same REST handler classes as HTTP/ICF
- Can redirect to different app server instances via `X-sap-adt-server-instance` header

## Open questions

1. **How does Eclipse set a TRUE external breakpoint?** Our API call creates a session-scoped BP. Eclipse must do something different.
2. **Does Eclipse use RFC instead of HTTP?** The `SADT_REST_RFC_ENDPOINT` FM suggests yes, but the handler code is the same.
3. **Is the listener meant to work with GUI-set external breakpoints?** Our test says no — GUI debugger and ADT listener are independent.

## Next steps

- Install Eclipse ADT and capture network/RFC traffic to see the exact flow
- Compare Eclipse's breakpoint request with ours
- Check if Eclipse uses different parameters or a different endpoint sequence
