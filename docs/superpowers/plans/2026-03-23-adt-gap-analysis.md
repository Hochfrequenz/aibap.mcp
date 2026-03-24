# ADT API Gap Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create GitHub milestones and verified issues that map our MCP server's ADT API coverage against the real SAP system, including integration test designs per issue.

**Architecture:** Query the real SAP discovery service to verify which endpoints exist, then create milestones and issues on GitHub. Each issue includes verified endpoint details and integration test descriptions. Issues that cannot be verified are created with an explicit "needs manual verification" label.

**Tech Stack:** `gh` CLI for GitHub operations, `curl` for SAP endpoint verification, Go for integration test harness.

---

## File Structure

No production code changes in this plan. Outputs are:

- **GitHub milestones** (3): created via `gh` CLI
- **GitHub issues** (25): created via `gh` CLI, assigned to milestones
- **Integration test harness**: `adt/integration_test_helpers_test.go` (build-tagged)
- **Makefile update**: add `integration-test` target
- **Test fixture docs**: `testdata/integration_objects.md`

---

### Task 1: Fetch and Record ADT Discovery Service

**Purpose:** Get the authoritative list of available ADT endpoints from the real SAP. This is the evidence base for all subsequent issues.

**Files:**
- Create: `docs/superpowers/evidence/discovery-response.xml` (raw discovery response)
- Create: `docs/superpowers/evidence/discovery-endpoints.md` (parsed endpoint summary)

- [ ] **Step 1: Fetch the discovery service response**

The SAP system is at `https://srvhfuhana.sap.msp.local:44300`. We need credentials.
Ask the user for SAP credentials (user/password/client) if not already set as env vars.

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: Fetch" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/discovery" \
  -o docs/superpowers/evidence/discovery-response.xml
```

Expected: XML Atom feed listing all available ADT services with their URIs.

- [ ] **Step 2: Parse and document the available endpoints**

Read the discovery XML and create a markdown summary listing every service URI.
Group by functional area (source, activation, search, transport, etc.).
Mark which endpoints our code currently uses vs which are new.

Save to `docs/superpowers/evidence/discovery-endpoints.md`.

- [ ] **Step 3: Commit the evidence**

```bash
git add docs/superpowers/evidence/
git commit -m "docs: record SAP ADT discovery service response as verification evidence"
```

---

### Task 2: Verify Current Endpoints Against Real SAP

**Purpose:** Test each of our 11 existing client methods against the real SAP to identify what works and what's broken.

**Files:**
- Modify: `docs/superpowers/evidence/discovery-endpoints.md` (add verification results)

- [ ] **Step 1: Test GetSource**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: text/plain" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/programs/programs/RS_HELLO_WORLD/source/main"
```

Record: HTTP status, whether source is returned, whether ETag header is present.
If RS_HELLO_WORLD doesn't exist, try another standard program (e.g., SAPMSYST or RSPARAM).

- [ ] **Step 2: Test SearchObjects**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: application/xml" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/repository/informationsystem/search?operation=quickSearch&query=CL_*&maxResults=5"
```

Record: HTTP status, response XML structure, element/attribute names.

- [ ] **Step 3: Test WhereUsed (verify GET vs POST)**

```bash
# Try GET first (our current implementation)
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: application/xml" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/repository/informationsystem/usageReferences?adtObjectUri=/sap/bc/adt/programs/programs/RS_HELLO_WORLD"
```

If GET fails (4xx), try POST with the URI in the body. Record which method works.

- [ ] **Step 4: Test BrowsePackage (critical — suspected XML mismatch)**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: application/xml" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/repository/nodestructure?parent_type=DEVC/K&parent_name=%24LOCAL"
```

Record: HTTP status, **full XML structure** (element names, attribute names).
Compare against what `parseObjectReferences` expects (`<objectReference>` with `uri`, `type`, `name`, `description`, `packageName` attributes).

- [ ] **Step 5: Test GetObjectInfo**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: application/xml" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/programs/programs/RS_HELLO_WORLD"
```

Record: HTTP status, response XML structure.

- [ ] **Step 6: Test SyntaxCheck**

First fetch a CSRF token, then:

```bash
# Fetch CSRF token
CSRF=$(curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: Fetch" \
  -D - \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/discovery" \
  -o /dev/null 2>&1 | grep -i "x-csrf-token:" | tr -d '\r' | awk '{print $2}')

# Run syntax check
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/xml" \
  -H "Accept: application/xml" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/checkruns?adtObjectUri=/sap/bc/adt/programs/programs/RS_HELLO_WORLD"
```

Record: HTTP status, response XML structure.

- [ ] **Step 7: Test RunUnitTests**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/xml" \
  -H "Accept: application/xml" \
  -X POST \
  -d '<?xml version="1.0" encoding="UTF-8"?><aunit:run xmlns:aunit="http://www.sap.com/adt/aunit" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:timeout="30000"><adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/RS_HELLO_WORLD"/></aunit:run>' \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/abapunit/testruns"
```

Record: HTTP status, response XML structure (or "no tests found" response).

- [ ] **Step 8: Test GetTransportRequests**

```bash
curl -sk \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "Accept: application/xml" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/cts/transportrequests?user=$SAP_USER&status=D"
```

Record: HTTP status, response XML structure.

- [ ] **Step 9: Test Activation endpoint path**

Test both our path and the alternative:

```bash
# Our current path
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/xml" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/activation/activate?method=activate&preauditRequested=true" \
  -d '<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/RS_HELLO_WORLD" adtcore:name="RS_HELLO_WORLD"/></adtcore:objectReferences>'

# Alternative path
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/xml" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/activation?method=activate&preauditRequested=true" \
  -d '<?xml version="1.0" encoding="UTF-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/RS_HELLO_WORLD" adtcore:name="RS_HELLO_WORLD"/></adtcore:objectReferences>'
```

Record: which path returns 200 vs 404.

- [ ] **Step 10: Document all verification results**

Update `docs/superpowers/evidence/discovery-endpoints.md` with a results table:

| Endpoint | Method | Status | Response OK? | Notes |
|----------|--------|--------|-------------|-------|

- [ ] **Step 11: Commit verification results**

```bash
git add docs/superpowers/evidence/
git commit -m "docs: record endpoint verification results against real SAP"
```

---

### Task 3: Verify Hypothesized New Endpoints

**Purpose:** Check the discovery service and real SAP for the endpoints we want to add in Milestones 2 and 3.

**Files:**
- Modify: `docs/superpowers/evidence/discovery-endpoints.md` (add new endpoint verification)

- [ ] **Step 1: Check for locking endpoints in discovery XML**

Search the discovery response for `lock`, `_action`, or related terms.
Try the hypothesized lock call:

```bash
curl -sk -v \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/programs/programs/RS_HELLO_WORLD?_action=LOCK&accessMode=MODIFY"
```

Record: HTTP status, response body (lock handle format or error).

- [ ] **Step 2: Check for transport management endpoints**

Search discovery XML for `cts`, `transport`. Try:

```bash
# Transport check
curl -sk -v \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/xml" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/cts/transportchecks"

# Create transport (just check if endpoint exists, don't actually create)
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  -H "X-CSRF-Token: $CSRF" \
  -X POST \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/cts/transports"
```

Record: HTTP status for each. 404 = doesn't exist. 400/415 = exists but needs correct body.

- [ ] **Step 3: Check for Milestone 3 endpoints**

Search discovery XML and probe each:

```bash
# ATC
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/atc/customizing"

# Pretty printer
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/abapsource/prettyprinter/settings"

# Code completion
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/abapsource/codecompletion"

# Navigation
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/navigation/target"

# Inactive objects
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/activation/inactiveobjects"

# ABAP docs
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/bc/adt/docu/abap/langu"

# Logoff
curl -sk -o /dev/null -w "%{http_code}" \
  -u "$SAP_USER:$SAP_PASSWORD" \
  -H "sap-client: $SAP_CLIENT" \
  "https://srvhfuhana.sap.msp.local:44300/sap/public/bc/icf/logoff"
```

Record HTTP status for each. Categorize:
- **Available** (200/401/403): endpoint exists, issue can be verified
- **Unavailable** (404): endpoint not on this system, issue gets "not available" label
- **Needs body** (400/415): endpoint exists but needs correct request format

- [ ] **Step 4: Document all new endpoint verification results**

Add results to `docs/superpowers/evidence/discovery-endpoints.md`.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/evidence/
git commit -m "docs: verify hypothesized new ADT endpoints against real SAP"
```

---

### Task 4: Create GitHub Milestones

**Purpose:** Create the 3 milestones on GitHub.

- [ ] **Step 1: Create Milestone 1**

```bash
gh milestone create "Read & Navigate" \
  --description "Claude can reliably read, search, and navigate ABAP code. Includes verification of all read-only ADT operations and integration test infrastructure." \
  --repo Hochfrequenz/mcp-server-abap
```

- [ ] **Step 2: Create Milestone 2**

```bash
gh milestone create "Safe Edit Workflow" \
  --description "Claude can lock, edit, activate, and manage transports end-to-end. Adds object locking, verifies activation, and completes the transport lifecycle." \
  --repo Hochfrequenz/mcp-server-abap
```

- [ ] **Step 3: Create Milestone 3**

```bash
gh milestone create "Advanced Tooling" \
  --description "Claude has professional-grade ABAP development capabilities. Adds ATC checks, pretty-printer, code completion, navigation, and more." \
  --repo Hochfrequenz/mcp-server-abap
```

- [ ] **Step 4: Verify milestones exist**

```bash
gh milestone list --repo Hochfrequenz/mcp-server-abap
```

Expected: 3 milestones listed.

- [ ] **Step 5: Commit (nothing to commit — milestones are on GitHub)**

No file changes. Just verify.

---

### Task 5: Create Milestone 1 Issues on GitHub

**Purpose:** Create all 10 issues for "Read & Navigate". Each issue body is based on the spec but updated with verification evidence from Task 2.

**Important:** Before creating each issue, check the evidence from Task 2. If an endpoint was verified as working, include the evidence. If it was found broken, include the actual error. If untested, mark as "needs manual verification".

- [ ] **Step 1: Create issue 1.10 — Integration test harness (infra, no SAP verification needed)**

```bash
gh issue create \
  --repo Hochfrequenz/mcp-server-abap \
  --milestone "Read & Navigate" \
  --title "infra: integration test harness with build tag and env-var config" \
  --label "infrastructure" \
  --body "$(cat <<'ISSUE_EOF'
## Description

Create shared integration test infrastructure for all SAP ADT integration tests.

## Design

- Files: `adt/integration_helpers_test.go` with `//go:build integration` tag
- Environment variables: `SAP_INTEGRATION_HOST`, `SAP_INTEGRATION_USER`,
  `SAP_INTEGRATION_PASSWORD`, `SAP_INTEGRATION_CLIENT`
- Shared helper: `newIntegrationClient(t *testing.T) adt.Client` — creates real client,
  calls `t.Skip("SAP_INTEGRATION_HOST not set")` if env vars missing
- Not run in normal CI (`go test ./...` excludes integration tag by default)
- Makefile target: `make integration-test` runs with `-tags integration`
- Document required test fixtures in `testdata/integration_objects.md`

## Acceptance Criteria

- [ ] `go test ./...` does NOT run integration tests
- [ ] `go test -tags integration ./adt/...` runs integration tests (skips if env not set)
- [ ] `make integration-test` target exists
- [ ] `testdata/integration_objects.md` documents required SAP test fixtures

## Integration Test

Validated by the first integration test that uses the harness (GetSource, issue #N+1).
ISSUE_EOF
)"
```

- [ ] **Step 2: Create issue 1.1 — Investigate BrowsePackage XML parsing**

Use the actual XML response from Task 2 Step 4 as evidence in the issue body.
If the response uses `<objectNode>` instead of `<objectReference>`, this is a confirmed bug.
If it uses `<objectReference>`, close this as "not a bug".

```bash
gh issue create \
  --repo Hochfrequenz/mcp-server-abap \
  --milestone "Read & Navigate" \
  --title "investigate: BrowsePackage XML parsing may not match nodestructure response" \
  --label "bug?" \
  --body "$(cat <<'ISSUE_EOF'
## Description

`BrowsePackage` reuses `parseObjectReferences` which expects `<objectReference>` elements.
The `/sap/bc/adt/repository/nodestructure` endpoint may return a different XML schema.

## Evidence

[INSERT: Actual XML response from Task 2 Step 4]
[INSERT: Comparison with what parseObjectReferences expects]

## Verification

- [ ] Confirmed: response XML uses [element name] with [attribute names]
- [ ] Our parser expects: `<objectReference>` with `uri`, `type`, `name`, `description`, `packageName`
- [ ] Match: YES / NO

## Fix (if needed)

Add a dedicated `parseNodeStructure` function that handles the actual XML schema.

## Integration Test

- Call `BrowsePackage` on a known non-empty package (e.g., `$LOCAL` or [package from evidence])
- Assert returned list is non-empty
- Assert each `ObjectInfo` has URI, Name, and Type populated
ISSUE_EOF
)"
```

- [ ] **Step 3: Create issues 1.2–1.9 — Integration tests for each read operation**

Create one issue per operation. Each issue body follows this template, filled in with evidence from Task 2:

```
## Description
Integration test for [operation] against real SAP.

## Evidence from Verification
[INSERT: HTTP status, response format observed in Task 2]

## Integration Test
- [specific test steps from spec]
- [expected assertions]

## Acceptance Criteria
- [ ] Integration test passes against real SAP
- [ ] Test skips gracefully when SAP is unavailable
```

Create all 8 issues (1.2 through 1.9) for: GetSource, SearchObjects, WhereUsed,
BrowsePackage, GetObjectInfo, SyntaxCheck, RunUnitTests, GetTransportRequests.

- [ ] **Step 4: Verify all Milestone 1 issues**

```bash
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Read & Navigate"
```

Expected: 10 issues listed.

- [ ] **Step 5: Commit any local file changes**

```bash
git add -A && git commit -m "docs: update evidence with issue cross-references" || echo "nothing to commit"
```

---

### Task 6: Create Milestone 2 Issues on GitHub

**Purpose:** Create all 8 issues for "Safe Edit Workflow". Use evidence from Task 3 for new endpoints.

- [ ] **Step 1: Create issue 2.1 — Object locking**

Use evidence from Task 3 Step 1 (lock endpoint probe).

```bash
gh issue create \
  --repo Hochfrequenz/mcp-server-abap \
  --milestone "Safe Edit Workflow" \
  --title "feat: add object locking (LOCK/UNLOCK) before SetSource" \
  --label "enhancement" \
  --body "$(cat <<'ISSUE_EOF'
## Description

Our `SetSource` writes source without acquiring a lock first. The ADT API requires
locking before writing to prevent concurrent edit conflicts.

## Evidence

[INSERT: Lock endpoint probe result from Task 3 Step 1]
[INSERT: Lock response format or error message]

## Design

New Client methods:
- `LockObject(ctx, objectURI) (*LockResult, error)`
- `UnlockObject(ctx, objectURI, lockHandle) error`

Design decision (based on verification): whether `set_source` auto-locks or
`lock_object`/`unlock_object` are separate MCP tools.

## Integration Test

- Lock a known object, assert lock handle returned
- Write source while locked, assert success
- Unlock, assert success
- Attempt write without lock, observe behavior (document whether SAP rejects or allows it)

## Acceptance Criteria

- [ ] LockObject returns a lock handle
- [ ] UnlockObject releases the lock
- [ ] Integration test covers lock → write → unlock cycle
ISSUE_EOF
)"
```

- [ ] **Step 2: Create issue 2.2 — Verify activation path**

Use evidence from Task 2 Step 9.

- [ ] **Step 3: Create issues 2.3–2.5 — Transport check, create, release**

Use evidence from Task 3 Step 2. If endpoint returned 404, mark issue as
"endpoint not available on test system — may require ICF service activation".

- [ ] **Step 4: Create issues 2.6–2.8 — Integration tests**

Full edit cycle, stale ETag test, AddToTransport test.

- [ ] **Step 5: Verify all Milestone 2 issues**

```bash
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Safe Edit Workflow"
```

Expected: 8 issues listed.

---

### Task 7: Create Milestone 3 Issues on GitHub

**Purpose:** Create all 7 issues for "Advanced Tooling". Use evidence from Task 3 Step 3.

- [ ] **Step 1: Create issues 3.1–3.7**

For each feature (ATC, pretty-printer, code completion, navigation, inactive objects, ABAP docs, logout):

- If the endpoint probe returned a non-404 status → label as "enhancement", include evidence
- If the endpoint probe returned 404 → label as "enhancement,needs-icf-activation", note that the
  ICF service may need to be activated on the SAP system before this feature can be implemented

Each issue body follows the spec template with verified evidence inserted.

- [ ] **Step 2: Verify all Milestone 3 issues**

```bash
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Advanced Tooling"
```

Expected: 7 issues listed.

---

### Task 8: Create Integration Test Harness (Issue 1.10)

**Purpose:** Implement the integration test infrastructure so subsequent issues can build on it.

**Files:**
- Create: `adt/integration_helpers_test.go`
- Modify: `Makefile`
- Create: `testdata/integration_objects.md`

- [ ] **Step 1: Write the integration test helper**

```go
//go:build integration

package adt_test

import (
	"os"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func newIntegrationClient(t *testing.T) adt.Client {
	t.Helper()
	host := os.Getenv("SAP_INTEGRATION_HOST")
	if host == "" {
		t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
	}
	user := os.Getenv("SAP_INTEGRATION_USER")
	if user == "" {
		t.Fatal("SAP_INTEGRATION_USER must be set when SAP_INTEGRATION_HOST is set")
	}
	password := os.Getenv("SAP_INTEGRATION_PASSWORD")
	if password == "" {
		t.Fatal("SAP_INTEGRATION_PASSWORD must be set when SAP_INTEGRATION_HOST is set")
	}
	cfg := config.SAPConfig{
		Host:          host,
		User:          user,
		Password:      password,
		Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
		TLSSkipVerify: true,
	}
	return adt.NewClient(cfg)
}
```

- [ ] **Step 2: Run normal tests to ensure build tag excludes integration tests**

```bash
go test ./adt/... -v
```

Expected: integration tests NOT listed in output.

- [ ] **Step 3: Add Makefile target**

Add to `Makefile`:

```makefile
integration-test:
	go test -tags integration -v -count=1 ./adt/...
```

- [ ] **Step 4: Create test fixture documentation**

Create `testdata/integration_objects.md`:

```markdown
# Integration Test Fixtures

Objects that must exist on the SAP test system for integration tests to pass.

## Required Objects

| Object | URI | Used By | Notes |
|--------|-----|---------|-------|
| [to be filled after Task 2 verification] | | | |

## SAP System

- Host: configured via `SAP_INTEGRATION_HOST`
- Client: configured via `SAP_INTEGRATION_CLIENT`

## Running

```bash
export SAP_INTEGRATION_HOST=https://srvhfuhana.sap.msp.local:44300
export SAP_INTEGRATION_USER=...
export SAP_INTEGRATION_PASSWORD=...
export SAP_INTEGRATION_CLIENT=...
make integration-test
```
```

- [ ] **Step 5: Run integration tests (should skip if env not set)**

```bash
go test -tags integration ./adt/... -v
```

Expected: all tests skipped with "SAP_INTEGRATION_HOST not set".

- [ ] **Step 6: Commit**

```bash
git add adt/integration_helpers_test.go Makefile testdata/integration_objects.md
git commit -m "infra: add integration test harness with build tag and env-var config"
```

---

### Task 9: Final Summary and Push

**Purpose:** Push the branch and create a summary.

- [ ] **Step 1: Review all created milestones and issues**

```bash
echo "=== Milestones ==="
gh milestone list --repo Hochfrequenz/mcp-server-abap

echo "=== Milestone 1: Read & Navigate ==="
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Read & Navigate"

echo "=== Milestone 2: Safe Edit Workflow ==="
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Safe Edit Workflow"

echo "=== Milestone 3: Advanced Tooling ==="
gh issue list --repo Hochfrequenz/mcp-server-abap --milestone "Advanced Tooling"
```

- [ ] **Step 2: Push branch**

```bash
git push -u origin feat/adt-gap-analysis
```

- [ ] **Step 3: Summarize for the user**

Print summary: number of milestones, issues per milestone, any endpoints that were
unavailable (404) and might need ICF service activation.
