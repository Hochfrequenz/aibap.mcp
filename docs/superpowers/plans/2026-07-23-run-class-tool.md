# `run_class` Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `run_class` MCP tool that executes an ABAP class via the ADT classrun endpoint and returns its console output, guarded by a class-exists pre-check and an explicit confirmation prompt.

**Architecture:** A thin handler in a new `tools/classrun.go`, registered in the `system` tool group. It builds the CLAS URI, verifies the class exists via `GetObjectInfo`, asks for confirmation via the existing `ConfirmDestructive` elicitor path, then delegates execution to `adt.Client.RunClass` (shipped in adtler v0.3.12). The success payload is the adtler `adt.ClassRunResult` struct passed straight through — no parallel DTO.

**Tech Stack:** Go, `mcp-go`, `github.com/Hochfrequenz/adtler/adt`.

**Spec:** `docs/superpowers/specs/2026-07-22-run-class-tool-design.md`

## Global Constraints

- adtler dependency floor: **v0.3.12** (first tagged release containing `RunClass` / `ClassRunClient` / `ClassRunResult`). No pseudo-versions on `main`.
- Structured results are mandatory: success via `mcp.NewToolResultJSON`, errors via `errorResult(err)` (leaves `StructuredContent` unset). No `NewToolResultText`, no `map[string]any`, no slices to `NewToolResultJSON`.
- `errorResult` takes an `error`, never a string.
- Run `gofmt -w .`, `go vet ./...`, and `go test ./...` before every commit.
- Feature branch `feat/run-class-tool` off `main` (never commit to `main`). One PR for this feature.

## Real adtler API (verified against v0.3.12, do not re-derive)

```go
// package adt
type ClassRunResult struct {
    ClassName     string `json:"class_name"`
    ConsoleOutput string `json:"console_output"`
}

type ClassRunClient interface {
    RunClass(ctx context.Context, className string) (*ClassRunResult, error)
}
// ClassRunClient is embedded in the aggregate adt.Client interface (client.go:189).
// RunClass lower-cases className, POSTs /sap/bc/adt/oo/classrun/{name}, Accept: text/plain.
// It does NOT validate existence/activeness/interface — the caller pre-checks.
// Runtime exception -> non-2xx -> *adt.ADTError (status >= 500).
// "Soft" failures (missing S_DEVELOP, interface not implemented) -> HTTP 200 with error text in body.
```

## File Structure

- **Create** `tools/classrun.go` — `registerClassRunTools`, `buildRunClassMessage`, the handler. One responsibility: the `run_class` tool.
- **Create** `tools/classrun_test.go` — unit tests against `mockClient` + `stubElicitor`.
- **Create** `tools/classrun_integration_test.go` — `//go:build integration` live test.
- **Modify** `go.mod` / `go.sum` — bump adtler to v0.3.12.
- **Modify** `tools/source_test.go` — add `runClassFn` field + `RunClass` stub to `mockClient` (required so `mockClient` still satisfies the now-wider `adt.Client`).
- **Modify** `tools/register.go` — wire `registerClassRunTools` into the `system` group.

---

### Task 1: Bump adtler to v0.3.12 and keep the build green

Bumping widens `adt.Client` with `ClassRunClient`, so `mockClient` stops satisfying `adt.Client` until it gains a `RunClass` method. This task does both and ends with a green build — no `run_class` yet.

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `tools/source_test.go:32-62` (struct fields) and after `tools/source_test.go:271` (RunQuery stub) for the new `RunClass` stub

**Interfaces:**
- Consumes: `adt.ClassRunResult`, `adt.ClassRunClient` (from adtler v0.3.12).
- Produces: `mockClient.runClassFn func(context.Context, string) (*adt.ClassRunResult, error)` and method `RunClass` — used by Task 2's unit tests.

- [ ] **Step 1: Create the feature branch**

```bash
git checkout main && git pull
git checkout -b feat/run-class-tool
```

- [ ] **Step 2: Bump adtler**

```bash
go get github.com/Hochfrequenz/adtler@v0.3.12 && go mod tidy
```

- [ ] **Step 3: Verify the bump landed and the build now fails on the mock**

Run: `grep adtler go.mod`
Expected: `github.com/Hochfrequenz/adtler v0.3.12`

Run: `go build ./... 2>&1 | head`
Expected: FAIL — `*mockClient does not implement adt.Client (missing method RunClass)` (or equivalent). This confirms the interface widened.

- [ ] **Step 4: Add the `runClassFn` field to `mockClient`**

In `tools/source_test.go`, inside the `mockClient` struct (alongside `runQueryFn`), add:

```go
	runClassFn            func(ctx context.Context, className string) (*adt.ClassRunResult, error)
```

- [ ] **Step 5: Add the `RunClass` stub method**

In `tools/source_test.go`, after the `RunQuery` method (around line 265), add:

```go
func (m *mockClient) RunClass(ctx context.Context, className string) (*adt.ClassRunResult, error) {
	if m.runClassFn != nil {
		return m.runClassFn(ctx, className)
	}
	return &adt.ClassRunResult{ClassName: className}, nil
}
```

- [ ] **Step 6: Verify the build is green and existing tests pass**

Run: `gofmt -w . && go vet ./... && go test ./...`
Expected: PASS (all existing packages). The `structured_content_shape_test.go` guardrail still passes because `run_class` is not registered yet.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum tools/source_test.go
git commit -m "chore: bump adtler to v0.3.12 (RunClass), add mockClient stub

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Implement and register the `run_class` tool (TDD)

**Files:**
- Create: `tools/classrun.go`
- Create: `tools/classrun_test.go`
- Modify: `tools/register.go:219` (the `system` group closure)

**Interfaces:**
- Consumes: `adt.SearchClient.GetObjectInfo` (GetObjectInfo lives on `SearchClient`, not `ObjectClient`), `adt.ClassRunClient.RunClass`, `ConfirmDestructive(ctx, Elicitor, string) (bool, string)`, `errorResult(error)`, `mockClient.runClassFn`, `stubElicitor{result,err,called}`, `newTestServerWithFallbackElicitor(client, fallback, elicitor)` (transport_test.go:51).
- Produces: MCP tool `run_class` with input `class_name` and output shape `adt.ClassRunResult`.

- [ ] **Step 1: Write the failing unit tests**

Create `tools/classrun_test.go`:

```go
package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunClass_HappyPath(t *testing.T) {
	var gotClass string
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			gotClass = className
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "hello from abap"}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		Action:  mcp.ElicitationResponseActionAccept,
		Content: map[string]any{"confirm": true},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}
	if gotClass != "ZCL_MY_RUNNER" {
		t.Errorf("RunClass called with %q, want ZCL_MY_RUNNER", gotClass)
	}
	if el.called != 1 {
		t.Errorf("elicitor called %d times, want 1", el.called)
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "hello from abap") {
		t.Errorf("console output missing from result: %v", res.Content)
	}
}

func TestRunClass_ClassMissing(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		getObjectFn: func(context.Context, string) (*adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_NOPE"})

	if !res.IsError {
		t.Fatal("expected error for missing class")
	}
	if runCalled {
		t.Error("RunClass must not be called when the class is missing")
	}
	if el.called != 0 {
		t.Error("elicitor must not be prompted when the class is missing")
	}
}

func TestRunClass_ConfirmationDeclined(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{Action: mcp.ElicitationResponseActionDecline}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if !res.IsError {
		t.Fatal("expected error when confirmation declined")
	}
	if runCalled {
		t.Error("RunClass must not be called when confirmation is declined")
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "run_class aborted") {
		t.Errorf("expected 'run_class aborted' in error, got: %v", res.Content)
	}
}

func TestRunClass_NilElicitorProceeds(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "ran"}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil) // nil elicitor

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("nil elicitor should proceed, got error: %v", res.Content)
	}
	if !runCalled {
		t.Error("RunClass should be called when elicitor is nil (backwards-compat)")
	}
}

func TestRunClass_RunClassError(t *testing.T) {
	mock := &mockClient{
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "boom"}
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if !res.IsError {
		t.Fatal("expected error when RunClass fails")
	}
}
```

Note on test helpers (verified against the codebase):
- `callTool(t *testing.T, s *server.MCPServer, toolName string, args map[string]interface{}) *mcp.CallToolResult` exists at `tools/testhelper_test.go:16` — use it verbatim (`map[string]any` is identical to `map[string]interface{}`).
- There is **no** `mustText` helper. The package reads result text inline via `res.Content[0].(mcp.TextContent).Text` (see `transport_test.go:66`, `object_test.go:111`, `rollback_test.go:66`). This snippet uses that inline pattern — do not introduce a `mustText` helper.
- `res.IsError` is the error flag on `*mcp.CallToolResult`. Success payloads flow through `NewToolResultJSON`, which also populates `Content[0]` as `TextContent` (the JSON fallback), so the substring checks work on the success path.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/ -run TestRunClass -v`
Expected: FAIL — `run_class` tool not registered (`callTool` returns "tool not found" / the tool is absent from `tools/list`).

- [ ] **Step 3: Implement the tool**

Create `tools/classrun.go`:

```go
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// classRunClient is the narrow client surface run_class needs: an existence
// pre-check plus the classrun execution call.
type classRunClient interface {
	adt.SearchClient // GetObjectInfo lives here, not on adt.ObjectClient
	adt.ClassRunClient
}

func registerClassRunTools(s toolAdder, client classRunClient, elicitor Elicitor) {
	s.AddTool(mcp.NewTool("run_class",
		mcp.WithTitleAnnotation("Run ABAP Class"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Execute an ABAP class that implements IF_OO_ADT_CLASSRUN (ADT 'Run as "+
				"ABAP Application') and return its console output. The class must already "+
				"exist and be active. Runs arbitrary ABAP — side effects (COMMIT WORK, "+
				"data changes, deletions) are possible.",
		),
		mcp.WithString("class_name", mcp.Required(),
			mcp.Description("Name of the global class to execute, e.g. 'ZCL_MY_RUNNER'")),
		mcp.WithOutputSchema[adt.ClassRunResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		className := req.GetString("class_name", "")
		if className == "" {
			return errorResult(fmt.Errorf("run_class: class_name is required")), nil
		}

		// Cheap, safe existence pre-check. A non-nil error is the "missing"
		// signal (same convention as the object_exists tool). No interface
		// pre-check — SEOMETAREL misses inherited interfaces; let classrun's
		// own error surface if the class is not runnable.
		//
		// Namespace-safe: "/FOO/CL_BAR" yields ".../classes//foo/cl_bar" (double
		// slash), which adtler's GetObjectInfo -> encodeNamespacePath encodes to
		// ".../classes/%2ffoo%2fcl_bar". RunClass -> doMutate encodes identically,
		// so pre-check and execution agree for namespace objects.
		uri := "/sap/bc/adt/oo/classes/" + strings.ToLower(className)
		if _, err := client.GetObjectInfo(ctx, uri); err != nil {
			return errorResult(fmt.Errorf("class %s does not exist: %w", className, err)), nil
		}

		// Confirm AFTER the existence check so a missing class fails cheaply.
		// Decline returns a plain error (matching the run_query sibling), NOT a
		// fabricated adt.ADTError{StatusCode: 400} — no HTTP call happened, and a
		// synthetic 400 would render a misleading "SAP ADT error 400" + bad-
		// request/CSRF hint to the caller.
		proceed, reason := ConfirmDestructive(ctx, elicitor, buildRunClassMessage(className))
		if !proceed {
			return errorResult(fmt.Errorf("run_class aborted: %s", reason)), nil
		}

		result, err := client.RunClass(ctx, className)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}

// buildRunClassMessage produces the class-specific risk prompt shown to the
// user before execution. Static single-arg helper — unlike buildDeleteMessage
// it needs no ctx/client (no metadata enrichment).
func buildRunClassMessage(className string) string {
	return fmt.Sprintf(
		"Class %s is about to be executed via ADT classrun. It runs arbitrary ABAP "+
			"under the configured user and may cause side effects: COMMIT WORK, data "+
			"changes, or deletions. Approve execution?",
		className,
	)
}
```

- [ ] **Step 4: Wire the tool into the `system` group**

In `tools/register.go`, extend the `system` group closure (starts at line 219):

```go
		{"system", func() {
			registerSystemTools(ls, selector)
			registerQueryTools(ls, client, elicitor)
			registerClassRunTools(ls, client, elicitor)
		}},
```

`client` is `adt.Client`, which embeds both `SearchClient` and `ClassRunClient`, so it satisfies `classRunClient`. `elicitor` is already in scope here — no signature change to `RegisterAllWithLockMap`.

- [ ] **Step 5: Run the unit tests to verify they pass**

Run: `gofmt -w . && go vet ./... && go test ./tools/ -run TestRunClass -v`
Expected: PASS — all five `TestRunClass_*` cases.

- [ ] **Step 6: Run the full suite (guardrail included)**

Run: `go test ./...`
Expected: PASS. `structured_content_shape_test.go` now exercises `run_class` automatically: the blind reflective call proceeds (nil elicitor + mock `GetObjectInfo` returns no error) to the `mockClient.RunClass` stub, which returns a valid `&adt.ClassRunResult{}` → object-shaped `structuredContent` conforming to the declared `adt.ClassRunResult` schema. No `knownOptOut` needed.

- [ ] **Step 7: Commit**

```bash
git add tools/classrun.go tools/classrun_test.go tools/register.go
git commit -m "feat: add run_class tool (ADT classrun) with confirmation

Executes an ABAP class implementing IF_OO_ADT_CLASSRUN via adtler RunClass,
after a class-exists pre-check and an explicit ConfirmDestructive prompt.
Registered in the system group (default-on). Success payload is the adtler
adt.ClassRunResult struct.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Integration test against the live fixture

Covers the MCP wrapper layer against a real system, per the repo's integration convention. Requires VPN + `Z_ADT_MCP_TEST` on the target(s).

**Files:**
- Create: `tools/classrun_integration_test.go`

**Interfaces:**
- Consumes: the live fixture class `ZCL_ADT_MCP_CLASSRUN_TST` in package `Z_ADT_MCP_TEST` (the same fixture family adtler's classrun integration test uses).
- Produces: nothing consumed downstream.

**Verified live (2026-07-23, against HFQ + S4U):**
- CLAS URI `/sap/bc/adt/oo/classes/<lowercase-name>` resolves on both systems (existence check + source read succeed).
- `ZCL_ADT_MCP_CLASSRUN_TST` **exists and is active on both HFQ and S4U** (`CLAS/OC`). The throwing fixture `ZCL_ADT_MCP_CLASSRUN_ERR` also exists on both.
- A non-existent class (`zcl_this_does_not_exist_xyz`) returns `exists:false` — the underlying `GetObjectInfo` errors, which is the handler's "missing" signal.
- **Console string is `CLASSRUN_OK` on both systems, but written differently:** S4U uses `out->write( 'CLASSRUN_OK' )`, HFQ uses `out->write_text( 'CLASSRUN_OK' )`. `write` and `write_text` format the output differently, so the assertion MUST be a substring check (`strings.Contains(..., "CLASSRUN_OK")`), never string equality — an equality assert would pass on one system and fail on the other.

- [ ] **Step 1: (already verified) fixture presence**

The fixture is confirmed present on both `hfq` and `s4u` (see "Verified live" above). No action needed unless a system was reset since 2026-07-23 — re-check with `object_exists` on `/sap/bc/adt/oo/classes/zcl_adt_mcp_classrun_tst` if in doubt.

- [ ] **Step 2: Write the integration test**

Create `tools/classrun_integration_test.go`. Follow the existing integration-test harness in `tools/` (build tag, system iteration, real client construction) — open a sibling `*_integration_test.go` and mirror its setup verbatim; the snippet below shows only the assertion body to drop into that harness:

```go
//go:build integration

package tools_test

// TestIntegrationRunClass executes the ZCL_ADT_MCP_CLASSRUN_TST fixture on each
// configured system and asserts the known console output. Mirror the setup
// (eachSystem / real client + test server construction) from the neighbouring
// *_integration_test.go files in this package.

// Assertion body (inside the per-system loop, with a registered test server `s`):
//   res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_ADT_MCP_CLASSRUN_TST"})
//   if res.IsError {
//       t.Fatalf("%s: run_class failed: %v", sysName, res.Content)
//   }
//   // Substring, NOT equality: S4U uses out->write (formatted), HFQ uses
//   // out->write_text (raw) — both contain CLASSRUN_OK but format differs.
//   if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "CLASSRUN_OK") {
//       t.Errorf("%s: unexpected console output: %v", sysName, res.Content)
//   }
```

The expected substring is `CLASSRUN_OK` (verified live on both systems). Do not switch to string equality — the `write` vs `write_text` formatting difference across HFQ/S4U would break it.

Optionally add a second case for the throwing fixture `ZCL_ADT_MCP_CLASSRUN_ERR`, asserting `res.IsError` (adtler maps the uncaught exception to a `*adt.ADTError`, status ≥ 500, which the handler forwards via `errorResult`).

- [ ] **Step 3: Run the integration test**

Run: `go test -tags integration -v -count=1 -run RunClass ./tools/...`
Expected: PASS on the systems named by `MCP_INTEGRATION_SYSTEMS` (default `hfq,s4u`), or a clean `t.Skipf` where the fixture is absent.

- [ ] **Step 4: Commit**

```bash
git add tools/classrun_integration_test.go
git commit -m "test: integration coverage for run_class against Z_ADT_MCP_TEST fixture

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Open the PR and reconcile tracking bookkeeping

**Files:** none (GitHub + docs housekeeping)

- [ ] **Step 1: Push and open the PR**

```bash
git push -u origin feat/run-class-tool
```

Open a PR titled `feat: add run_class tool (ADT classrun)`. Body must include:
- Summary of the tool and its confirmation guard.
- A Test Plan section listing the unit tests, the integration test command, and the guardrail coverage.
- `Closes #<run_class tracking issue>` (find it: `gh issue list --search "run_class"`). Cross-link the spec PR #440 and adtler #100.

- [ ] **Step 2: Reconcile the `blocked-by-adtler` tracker**

Since adtler v0.3.12 is released and consumed, `run_class` is no longer blocked. Per CLAUDE.md "Cross-Repo Issue Tracking":
- Remove the `blocked-by-adtler` label from the run_class tracking issue.
- Check off / close its bullet on the open `Next adtler release: bump to vX.Y.Z` tracker. If that tracker's remaining blockers are now all resolved, follow the "defer creation until the first new blocker arrives" rule — do not leave an empty tracker open.

Run: `gh issue list --label blocked-by-adtler`
Expected: `run_class` no longer listed (or its bullet checked off).

- [ ] **Step 3: Verify CI is green on the PR**

Run: `gh pr checks`
Expected: all required checks pass (unit tests, lint, coverage — `tools` has no enforced minimum but must still compile and pass).

---

## Self-Review Notes

- **Spec coverage:** motivation/scope (Tasks 2, wiring) ✓; `class_name` input + CLAS URI construction (Task 2 handler) ✓; `adt.ClassRunResult` wire type + `WithOutputSchema` (Task 2) ✓; `destructive=true` annotation (Task 2) ✓; confirmation via `ConfirmDestructive` + `buildRunClassMessage` (Task 2) ✓; class-exists pre-check, no interface check (Task 2 handler + comment) ✓; error handling incl. aborted / classrun failure / runtime exception (handler forwards `errorResult`, adtler maps exception → `ADTError`) ✓; structured-content guardrail, no opt-out (Task 2 Step 6) ✓; unit + integration tests (Tasks 2, 3) ✓; rollout/bump (Task 1) ✓; tracker reconciliation (Task 4) ✓.
- **Placeholders:** none left. The Task 3 fixture string was resolved to `CLASSRUN_OK` by live verification against HFQ + S4U (2026-07-23). The `callRunClass` stub was removed after review.
- **Test helpers (verified):** `callTool` exists (`testhelper_test.go:16`); `mustText` does **not** — result text is read inline via `res.Content[0].(mcp.TextContent).Text`, matching the package convention. All snippets use the inline form.
- **Type consistency:** `runClassFn` signature matches `RunClass` matches the `classRunClient` interface matches `adt.ClassRunClient`. `adt.ClassRunResult{ClassName, ConsoleOutput}` used identically in the mock stub, the handler return, and the output schema.
