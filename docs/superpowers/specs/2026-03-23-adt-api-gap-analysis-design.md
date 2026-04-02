# ADT API Gap Analysis & Integration Test Plan

## Overview

Systematic comparison of our MCP server's ADT API usage against the official SAP ADT REST API surface.
Goal: identify correctness issues, missing features, and define integration tests — organized into
milestones that track how feature-complete and well-tested we are.

## Verification Principle

**No issue is created on GitHub without evidence.** Every claim about SAP ADT API behavior must be
verified against:

1. The real SAP system's ADT discovery service (`GET /sap/bc/adt/discovery`)
2. Actual endpoint responses from the real SAP at `srvhfuhana.sap.msp.local:44300`
3. Official SAP documentation (help.sap.com, SAP notes) where available

All endpoint paths, HTTP methods, request/response formats, and behaviors must be confirmed by
querying the real SAP system or citing an official SAP documentation URL. Unverified endpoints
are clearly marked as **hypothesized** — not stated as fact.

Each issue is marked **verified** or **needs verification** before submission.

## Verification Workflow

Before any issue is created on GitHub:

1. **Query discovery service** — `GET /sap/bc/adt/discovery` on real SAP to confirm which endpoints exist
2. **Test current code** — Run existing operations against real SAP to see what works vs fails
3. **Update issue text** — Replace claims with "verified: [evidence]" or "not available on this system"
4. **Drop unverifiable issues** — If we cannot prove an endpoint exists, do not create the issue

## Current Implementation

Our MCP server implements 12 tools backed by 11 `adt.Client` methods:

| Tool | Client Method | ADT Endpoint |
|------|--------------|--------------|
| `get_source` | `GetSource` | `GET {uri}/source/main` |
| `set_source` | `SetSource` | `PUT {uri}/source/main` |
| `activate_object` | `ActivateObject` | `POST /sap/bc/adt/activation/activate?method=activate&preauditRequested=true` |
| `search_objects` | `SearchObjects` | `GET /sap/bc/adt/repository/informationsystem/search?operation=quickSearch` |
| `where_used` | `WhereUsed` | `GET /sap/bc/adt/repository/informationsystem/usageReferences` |
| `browse_package` | `BrowsePackage` | `GET /sap/bc/adt/repository/nodestructure` |
| `get_object_info` | `GetObjectInfo` | `GET {objectURI}` |
| `syntax_check` | `SyntaxCheck` | `POST /sap/bc/adt/checkruns` |
| `run_unit_tests` | `RunUnitTests` | `POST /sap/bc/adt/abapunit/testruns` |
| `get_transport_requests` | `GetTransportRequests` | `GET /sap/bc/adt/cts/transportrequests` |
| `add_to_transport` | `AddToTransport` | `POST /sap/bc/adt/cts/transportrequests/{nr}/abaptransportcomponents` |
| `select_system` | (registry) | N/A (client-side) |

Infrastructure: CSRF token fetch via `GET /sap/bc/adt/discovery` with `X-CSRF-Token: Fetch`,
Basic Auth + `sap-client` header, session cookie caching, 401/403 retry with re-auth.

---

## Milestone 1: Read & Navigate

*Claude can reliably read, search, and navigate ABAP code.*

### Issue 1.1 — Investigate: BrowsePackage XML parsing may not match real nodestructure response

**Type:** Investigation + potential fix + test

**Problem:** `BrowsePackage` reuses `parseObjectReferences` which expects `<objectReference>` elements.
The `/sap/bc/adt/repository/nodestructure` endpoint may return a different XML schema (e.g.,
`<objectNode>` elements with different attribute names). If so, this causes silent empty results.

**Verification:**
`GET /sap/bc/adt/repository/nodestructure?parent_type=DEVC/K&parent_name=$LOCAL` on real SAP.
Compare response XML structure to what `parseObjectReferences` expects.

**Status:** Needs verification against SAP

**Integration test:**
- Call `BrowsePackage` on a known non-empty package
- Assert returned list is non-empty
- Assert each `ObjectInfo` has URI, Name, and Type populated

### Issue 1.2 — Test: GetSource reads source and returns ETag

**Type:** Integration test

**Verification:** `GET {uri}/source/main` with `Accept: text/plain` on a known program.

**Status:** Needs verification against SAP

**Integration test:**
- Call `GetSource` with a known program URI (e.g., `/sap/bc/adt/programs/programs/ZTEST_KNOWN`)
- Assert source is non-empty string
- Assert ETag is non-empty string
- Assert source contains expected ABAP keywords (`REPORT`, `DATA`, `WRITE`, etc.)

### Issue 1.3 — Test: SearchObjects returns matching objects

**Type:** Integration test

**Verification:** `GET /sap/bc/adt/repository/informationsystem/search?operation=quickSearch&query=Z*` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- Search for a known object by exact name
- Assert at least one result returned
- Assert the known object is in the result set
- Test with `objectType` filter and `maxResults` limit

### Issue 1.4 — Test: WhereUsed returns referencing objects

**Type:** Integration test

**Note:** Our code uses `GET` for this endpoint. Some sources suggest it may require `POST` with a
request body instead. The correct HTTP method must be verified against the real SAP.

**Verification:** Call `GET /sap/bc/adt/repository/informationsystem/usageReferences?adtObjectUri={uri}`
on real SAP. If it fails, try `POST` with the object URI in the request body.

**Status:** Needs verification against SAP

**Integration test:**
- Call `WhereUsed` on an object known to have consumers
- Assert at least one result returned
- Assert results contain URI, Type, Name fields

### Issue 1.5 — Test: BrowsePackage lists package contents (after 1.1 fix)

**Type:** Integration test (depends on issue 1.1)

**Verification:** `GET /sap/bc/adt/repository/nodestructure?parent_type=DEVC/K&parent_name={pkg}` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- Browse a known non-empty package
- Assert returned list is non-empty
- Assert objects have URI, Name, Type, PackageName

### Issue 1.6 — Test: GetObjectInfo returns object metadata

**Type:** Integration test

**Verification:** `GET {objectURI}` with `Accept: application/xml` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- Fetch info for a known class or program
- Assert Name, URI, Type are populated
- Assert Name matches expected value

### Issue 1.7 — Test: SyntaxCheck returns messages

**Type:** Integration test

**Verification:** `POST /sap/bc/adt/checkruns?adtObjectUri={uri}` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- Run syntax check on a syntactically correct object — assert no error messages
- If possible, run on an object with a known warning — assert messages returned with Type, Text, Line

### Issue 1.8 — Test: RunUnitTests returns test results

**Type:** Integration test

**Verification:** `POST /sap/bc/adt/abapunit/testruns` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- Run unit tests on a class with known test methods
- Assert `Passed + Failed > 0`
- Assert `TestCases` list is non-empty
- Assert each `TestCase` has Name and ExecutionTime

### Issue 1.9 — Test: GetTransportRequests lists transports

**Type:** Integration test

**Verification:** `GET /sap/bc/adt/cts/transportrequests` on real SAP.

**Status:** Needs verification against SAP

**Integration test:**
- List transports for the integration test user
- Assert response parses without error
- If transports exist, assert Number, Owner, Status are populated

### Issue 1.10 — Infra: Integration test harness

**Type:** Infrastructure

**Description:** Create the shared integration test infrastructure used by all other test issues.

**Design:**
- Files: `adt/*_integration_test.go` with `//go:build integration` tag
- Environment variables: `SAP_INTEGRATION_HOST`, `SAP_INTEGRATION_USER`,
  `SAP_INTEGRATION_PASSWORD`, `SAP_INTEGRATION_CLIENT`
- Shared helper: `newIntegrationClient(t *testing.T) adt.Client` — creates real client,
  calls `t.Skip("SAP_INTEGRATION_HOST not set")` if env vars missing
- Not run in normal CI (`go test ./...` excludes integration tag by default)
- Makefile target: `make integration-test` runs with `-tags integration`
- Each test is idempotent: write tests restore original state after

**Status:** No SAP verification needed (pure infrastructure)

**Acceptance criteria:** Issue 1.2 (GetSource integration test) runs successfully using this harness.
The harness is validated by the first real test that uses it.

---

## Milestone 2: Safe Edit Workflow

*Claude can lock, edit, activate, and manage transports end-to-end.*

### Issue 2.1 — Feature: Add object locking (LOCK/UNLOCK)

**Type:** Feature + fix

**Problem:** Our `SetSource` writes source without acquiring a lock first. The ADT API may
require locking before writing. The hypothesized endpoints are:
- Lock: `POST {uri}?_action=LOCK&accessMode=MODIFY` — returns a lock handle
- Unlock: `POST {uri}?_action=UNLOCK&lockHandle={handle}`

Without locking, writes may fail on systems that enforce it, or cause data loss if two editors
write concurrently.

**Hypothesized endpoints** (must be verified against real SAP):
- Lock endpoint path and query parameters
- Lock response XML format (lock handle, transport number)
- Unlock endpoint path and required parameters

**Verification:**
1. Attempt `PUT .../source/main` without prior LOCK on real SAP — does it succeed or fail?
2. Discover the lock endpoint via `/sap/bc/adt/discovery` or trial calls
3. Inspect actual response XML for lock handle format
4. Test unlock

**Status:** Needs verification against SAP

**New Client methods:** `LockObject(ctx, objectURI) (*LockResult, error)`,
`UnlockObject(ctx, objectURI, lockHandle) error`

**Design decision (to resolve during verification):** Whether `set_source` auto-locks/unlocks
internally, or whether `lock_object` / `unlock_object` are exposed as separate MCP tools.
The verification step should determine which approach is more practical by observing how the
real SAP behaves (e.g., does lock return a transport number that the user needs to see?).

**Integration test:**
- Lock a known object, assert lock handle returned
- Write source while locked, assert success
- Unlock, assert success
- Attempt write without lock, observe behavior

### Issue 2.2 — Bug?: Verify activation endpoint path

**Type:** Verification + potential fix

**Problem:** We use `/sap/bc/adt/activation/activate?method=activate&preauditRequested=true`.
Some sources suggest the path is `/sap/bc/adt/activation?method=activate&preauditRequested=true`
(without `/activate` suffix).

**Verification:**
1. Check `/sap/bc/adt/discovery` for the activation service URI
2. Call our current path on real SAP
3. Call the alternative path on real SAP
4. Compare responses

**Status:** Needs verification against SAP

**Integration test:**
- Activate a known inactive object (or re-activate an already active one)
- Assert `ActivationResult.Success == true` (or expected messages)

### Issue 2.3 — Feature: Add transport check

**Type:** Feature

**Description:** Before modifying an object, it is useful to know whether a transport request is
required. The hypothesized endpoint is `POST /sap/bc/adt/cts/transportchecks`.

**Verification:**
1. Check `/sap/bc/adt/discovery` for transport check service
2. Call endpoint with a known object URI, inspect request/response format

**Status:** Needs verification against SAP

**New Client method:** `CheckTransport(ctx, objectURI) (*TransportCheckResult, error)`

**Integration test:**
- Check transport requirement for an object in a transportable package
- Assert response indicates whether transport is needed
- Check for an object in `$TMP` — assert no transport needed

### Issue 2.4 — Feature: Add create transport request

**Type:** Feature

**Description:** Currently we can list transports and add objects to existing transports, but cannot
create new transport requests. Hypothesized endpoint: `POST /sap/bc/adt/cts/transports`.

**Verification:**
1. Check `/sap/bc/adt/discovery` for transport creation service
2. Call endpoint on real SAP, inspect required request body and response

**Status:** Needs verification against SAP

**New Client method:** `CreateTransport(ctx, description, target string) (*TransportRequest, error)`

**Integration test:**
- Create a transport with a test description
- Assert transport number returned
- Clean up: release the test transport after (delete-transport endpoint availability to be
  verified; if unavailable, releasing an empty transport is acceptable cleanup)

### Issue 2.5 — Feature: Add release transport request

**Type:** Feature

**Description:** Complete the transport lifecycle by supporting release. The endpoint is believed to
be `POST /sap/bc/adt/cts/transportrequests/{nr}/newreleasejobs` (hypothesized — must verify
against real SAP).

**Verification:**
1. Check `/sap/bc/adt/discovery` for transport release service
2. Call endpoint on real SAP with a test transport

**Status:** Needs verification against SAP

**New Client method:** `ReleaseTransport(ctx, transportNumber string) error`

**Integration test:**
- Create a transport (via 2.4), release it, assert success
- Verify transport status changes to "L" (released)

### Issue 2.6 — Test: Full edit cycle end-to-end

**Type:** Integration test (depends on 2.1, 2.2, and optionally 2.3–2.5)

**Description:** Prove the complete workflow: lock → get source → modify → set source → activate → assign transport → unlock.

**Verification:** Run the full cycle against the real SAP with a dedicated test object.

**Status:** Needs verification against SAP

**Integration test:**
- Read original source and ETag
- Lock object
- Write modified source (append a comment line, e.g., `* integration test timestamp`)
- Activate
- Optionally assign to transport
- Unlock
- Restore original source (lock → write original → activate → unlock)
- Assert each step succeeded

### Issue 2.7 — Test: SetSource with stale ETag returns error

**Type:** Integration test

**Description:** Verify optimistic locking works: writing with an outdated ETag should fail.

**Verification:** Attempt a write with stale ETag against the real SAP and observe the error response.

**Status:** Needs verification against SAP

**Integration test:**
- Get source + ETag (call it etag_v1)
- Lock object
- Write a trivial change (append `* etag test`), activating to finalize (ETag now changes)
- Unlock
- Attempt SetSource with etag_v1 (now stale)
- Assert error (expected: 412 Precondition Failed or ADT error)
- No restore needed — the trivial change from step 3 is the final state; next test run overwrites it

### Issue 2.8 — Test: AddToTransport assigns object to transport

**Type:** Integration test

**Description:** Verify that `AddToTransport` correctly assigns an object to an existing transport.

**Verification:** Call endpoint on real SAP with a known object and transport.

**Status:** Needs verification against SAP

**Integration test:**
- List transports for test user, pick one modifiable transport (or create one via 2.4 if available)
- Call `AddToTransport` with a known object URI and the transport number
- Assert no error returned
- Verify by listing transport contents (if endpoint available) or by re-listing transports

---

## Milestone 3: Advanced Tooling

*Claude has professional-grade ABAP development capabilities.*

### Issue 3.1 — Feature: Add ATC (ABAP Test Cockpit) checks

**Type:** Feature

**Description:** Static analysis beyond syntax check. ATC provides code quality checks similar to
linting. Multiple endpoints are hypothesized under `/sap/bc/adt/atc/...` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for ATC-related services
2. Identify the minimal set of endpoints needed for: run check → get results
3. Test on real SAP

**Status:** Needs verification against SAP

**New Client methods:** `RunATCCheck(ctx, objectURI string) (*ATCResult, error)` (or async variant)

**Integration test:**
- Run ATC check on a known object
- Assert results contain findings or empty list
- Assert each finding has severity, message, location

### Issue 3.2 — Feature: Add pretty-printer

**Type:** Feature

**Description:** Format ABAP source code using SAP's built-in pretty-printer.
Hypothesized endpoint: `POST /sap/bc/adt/abapsource/prettyprinter` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for prettyprinter service
2. Test on real SAP with known source

**Status:** Needs verification against SAP

**New Client method:** `PrettyPrint(ctx, source string) (string, error)`

**Integration test:**
- Submit poorly formatted ABAP source
- Assert returned source is formatted (indentation, keyword casing)

### Issue 3.3 — Feature: Add code completion

**Type:** Feature

**Description:** Provide code completion suggestions at a cursor position.
Hypothesized endpoint: `POST /sap/bc/adt/abapsource/codecompletion/proposal` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for codecompletion service
2. Test on real SAP — determine required request format (object URI + cursor position)

**Status:** Needs verification against SAP

**New Client method:** `GetCompletions(ctx, objectURI string, line, column int) ([]CompletionItem, error)`

**Integration test:**
- Request completions at a position where `cl_` prefix is typed
- Assert at least one completion suggestion returned

### Issue 3.4 — Feature: Add navigate-to-definition

**Type:** Feature

**Description:** Resolve a symbol at a given position to its definition URI.
Hypothesized endpoint: `POST /sap/bc/adt/navigation/target` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for navigation service
2. Test on real SAP with a known method call

**Status:** Needs verification against SAP

**New Client method:** `NavigateToDefinition(ctx, objectURI string, line, column int) (*ObjectInfo, error)`

**Integration test:**
- Navigate from a known method call to its definition
- Assert returned URI points to the correct class/method

### Issue 3.5 — Feature: Add list inactive objects

**Type:** Feature

**Description:** List objects pending activation.
Hypothesized endpoint: `GET /sap/bc/adt/activation/inactiveobjects` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for inactive objects service
2. Test on real SAP

**Status:** Needs verification against SAP

**Dependencies:** Integration test requires object locking and writing (Milestone 2, issue 2.1).

**New Client method:** `GetInactiveObjects(ctx) ([]ObjectInfo, error)`

**Integration test:**
- Lock and modify an object without activating (requires Milestone 2 locking), then list inactive objects
- Assert the modified object appears in the list
- Clean up: activate the object and unlock

### Issue 3.6 — Feature: Add ABAP keyword documentation

**Type:** Feature

**Description:** Retrieve built-in ABAP keyword documentation.
Hypothesized endpoint: `POST /sap/bc/adt/docu/abap/langu` (must verify via discovery).

**Verification:**
1. Check `/sap/bc/adt/discovery` for documentation service
2. Test with a known keyword (e.g., `SELECT`)

**Status:** Needs verification against SAP

**New Client method:** `GetABAPDoc(ctx, keyword string) (string, error)`

**Integration test:**
- Request docs for `SELECT`
- Assert non-empty HTML/text response

### Issue 3.7 — Feature: Add session logout

**Type:** Feature

**Description:** Cleanly terminate the SAP session on shutdown.
Endpoint: `GET /sap/public/bc/icf/logoff`.

**Verification:** Call on real SAP, verify session cookies are invalidated.

**Status:** Needs verification against SAP

**New Client method:** `Logout(ctx) error`

**Integration test:**
- Establish session (any API call)
- Call Logout
- Attempt another API call — should require re-authentication

---

## Integration Test Infrastructure

### Build tag approach

```go
//go:build integration

package adt_test
```

Tests are excluded from `go test ./...` by default. Run explicitly:

```bash
SAP_INTEGRATION_HOST=https://srvhfuhana.sap.msp.local:44300 \
SAP_INTEGRATION_USER=DEVELOPER \
SAP_INTEGRATION_PASSWORD=secret \
SAP_INTEGRATION_CLIENT=100 \
go test -tags integration -v ./adt/...
```

### Shared helper

```go
func newIntegrationClient(t *testing.T) adt.Client {
    t.Helper()
    host := os.Getenv("SAP_INTEGRATION_HOST")
    if host == "" {
        t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
    }
    cfg := config.SAPConfig{
        Host:          host,
        User:          os.Getenv("SAP_INTEGRATION_USER"),
        Password:      os.Getenv("SAP_INTEGRATION_PASSWORD"),
        Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
        TLSSkipVerify: true, // internal test system
    }
    return adt.NewClient(cfg)
}
```

### Makefile target

```makefile
integration-test:
	go test -tags integration -v -count=1 ./adt/...
```

### Test object conventions

- Tests reference objects that must exist on the test SAP (documented in each test)
- Write tests are idempotent: they restore original state after
- A `testdata/integration_objects.md` file documents required test fixtures on the SAP system

---

## Summary

| Milestone | Issues | Type Breakdown |
|-----------|--------|----------------|
| 1 — Read & Navigate | 10 | 1 investigation/fix, 8 integration tests, 1 infrastructure |
| 2 — Safe Edit Workflow | 8 | 4 features, 1 verification, 3 integration tests |
| 3 — Advanced Tooling | 7 | 7 features (each with integration test) |
| **Total** | **25** | |

All issues are **"needs verification against SAP"** until validated against the real system.
No GitHub issue will be created without evidence.
