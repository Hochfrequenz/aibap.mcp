# run_query Purpose Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a required `purpose` enum parameter to `run_query` that forces Claude to declare intent, triggering human confirmation via the Elicitor pattern when the value is missing or unknown.

**Architecture:** `registerQueryTools` receives an `Elicitor`, validates `purpose` against a fixed set of four values before calling `RunQuery`, and delegates to `ConfirmDestructive` when invalid/missing (hard-blocks when no elicitor is wired). Middleware logs `purpose` as a structured attribute. `synthesizeArgs` in the shape test is extended to pick the first enum value from the schema so the reflective test exercises the query success path.

**Tech Stack:** Go, `mcp-go` (mcp.Tool, mcp.ToolOption, mcp.CallToolRequest), `slog`, existing `ConfirmDestructive`/`Elicitor` pattern.

---

### Task 1: Write failing tests for purpose enforcement

**Files:**
- Create: `tools/query_test.go`

- [ ] **Step 1: Create the test file**

```go
package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunQuery_ValidPurpose_CallsRunQuery(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			called = true
			if sql != "SELECT * FROM DD01L" {
				t.Errorf("unexpected sql: %q", sql)
			}
			return &adt.QueryResult{Columns: []string{"DOMNAME"}, Rows: [][]string{{"CHAR10"}}}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql":     "SELECT * FROM DD01L",
		"purpose": "ddic_inspection",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("RunQuery was not called")
	}
}

func TestRunQuery_MissingPurpose_ElicitorAccepts_CallsRunQuery(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql": "SELECT * FROM DD01L",
	})
	if result.IsError {
		t.Fatalf("expected success after elicitor accept, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("RunQuery was not called after elicitor accept")
	}
	if el.called != 1 {
		t.Errorf("expected 1 elicitor call, got %d", el.called)
	}
}

func TestRunQuery_InvalidPurpose_ElicitorDeclines_ReturnsError(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql":     "SELECT * FROM VBAK",
		"purpose": "reporting",
	})
	if !result.IsError {
		t.Fatal("expected error when elicitor declines")
	}
	if called {
		t.Fatal("RunQuery must not be called when elicitor declines")
	}
}

func TestRunQuery_MissingPurpose_NilElicitor_HardBlock(t *testing.T) {
	called := false
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			called = true
			return &adt.QueryResult{}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil)
	result := callTool(t, s, "run_query", map[string]interface{}{
		"sql": "SELECT * FROM VBAK",
	})
	if !result.IsError {
		t.Fatal("expected hard block when elicitor is nil and purpose is missing")
	}
	if called {
		t.Fatal("RunQuery must not be called on hard block")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```
go test ./tools/... -run "TestRunQuery_" -v
```

Expected: FAIL — `run_query` does not have a `purpose` parameter yet, so `TestRunQuery_ValidPurpose_CallsRunQuery` succeeds (no enforcement), and `TestRunQuery_MissingPurpose_NilElicitor_HardBlock` fails because RunQuery gets called instead of being blocked.

- [ ] **Step 3: Commit the failing tests**

```
git add tools/query_test.go
git commit -m "test: add failing tests for run_query purpose enforcement"
```

---

### Task 2: Add `purpose` parameter and enforcement to `registerQueryTools`

**Files:**
- Modify: `tools/query.go`
- Modify: `tools/register.go`

- [ ] **Step 1: Rewrite `tools/query.go`**

Replace the entire file content:

```go
package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validQueryPurposes lists the only values accepted for the run_query
// "purpose" parameter. Callers outside this set must obtain explicit user
// confirmation via the Elicitor before the query is executed.
var validQueryPurposes = map[string]bool{
	"ddic_inspection":      true,
	"customizing_review":   true,
	"transport_tracking":   true,
	"development_metadata": true,
}

func registerQueryTools(s toolAdder, client adt.QueryClient, elicitor Elicitor) {
	s.AddTool(mcp.NewTool("run_query",
		mcp.WithTitleAnnotation("Run SQL Query"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Execute a SELECT query on SAP database tables. Returns columns and rows. "+
				"Use standard ABAP SQL syntax (e.g. 'SELECT BUKRS, BUTXT FROM T001 ORDER BY BUKRS'). "+
				"Only SELECT statements are supported — no INSERT, UPDATE, or DELETE. "+
				"SAP API Policy: This tool is intended for development tooling only. "+
				"You MUST declare the purpose of the query via the 'purpose' parameter. "+
				"Valid values: ddic_inspection, customizing_review, transport_tracking, development_metadata. "+
				"Queries outside these categories may violate the SAP API Policy "+
				"(https://help.sap.com/doc/sap-api-policy/latest/en-US/API_Policy_latest.pdf).",
		),
		withQueryPurposeParam(),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL SELECT statement, e.g. 'SELECT BUKRS, BUTXT FROM T001'")),
		mcp.WithNumber("max_rows", mcp.Description("Maximum number of rows to return (default: 100)")),
		mcp.WithOutputSchema[adt.QueryResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		purpose := req.GetString("purpose", "")
		if !validQueryPurposes[purpose] {
			if elicitor == nil {
				return errorResult(fmt.Errorf(
					"run_query blocked: 'purpose' is missing or not a recognised development-tooling value. "+
						"Valid values: ddic_inspection, customizing_review, transport_tracking, development_metadata. "+
						"Querying tables outside this scope may violate the SAP API Policy",
				)), nil
			}
			proceed, reason := ConfirmDestructive(ctx, elicitor,
				"run_query requires a valid purpose. Declare why this query is needed for development tooling "+
					"(ddic_inspection / customizing_review / transport_tracking / development_metadata). "+
					"If none applies, this query may violate the SAP API Policy.")
			if !proceed {
				return errorResult(fmt.Errorf("run_query aborted: %s", reason)), nil
			}
		}

		sql := req.GetString("sql", "")
		maxRows := int(req.GetFloat("max_rows", 100))
		result, err := client.RunQuery(ctx, sql, maxRows)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(result)
	})
}

// withQueryPurposeParam adds the "purpose" parameter with a JSON Schema enum
// to the run_query tool definition so MCP clients (and Claude) see the valid
// values at tool-listing time.
func withQueryPurposeParam() mcp.ToolOption {
	return func(t *mcp.Tool) {
		t.InputSchema.Properties["purpose"] = map[string]any{
			"type": "string",
			"description": "Declared reason for this query — must be one of the approved development-tooling categories. " +
				"ddic_inspection: reading DDIC metadata tables (DD01L, DD02L, …). " +
				"customizing_review: reading Customizing tables (T001, TVARVC, …). " +
				"transport_tracking: reading transport catalog tables (E070, E071, …). " +
				"development_metadata: reading development object catalog tables (TRDIR, TADIR, PROGDIR, …).",
			"enum": []any{
				"ddic_inspection",
				"customizing_review",
				"transport_tracking",
				"development_metadata",
			},
		}
		t.InputSchema.Required = append(t.InputSchema.Required, "purpose")
	}
}
```

- [ ] **Step 2: Update `tools/register.go` — pass elicitor to `registerQueryTools`**

In `RegisterAllWithLockMap`, find the `"system"` group registration and change:

```go
// before
{"system", func() {
    registerSystemTools(ls, selector)
    registerQueryTools(ls, client)
}},
```

```go
// after
{"system", func() {
    registerSystemTools(ls, selector)
    registerQueryTools(ls, client, elicitor)
}},
```

- [ ] **Step 3: Run the tests to confirm they pass**

```
go test ./tools/... -run "TestRunQuery_" -v
```

Expected: all four `TestRunQuery_*` tests PASS.

- [ ] **Step 4: Run the full test suite**

```
go test ./... 
```

Expected: all tests PASS. Fix any compilation errors before continuing.

- [ ] **Step 5: Commit**

```
git add tools/query.go tools/register.go
git commit -m "feat: enforce purpose parameter on run_query via Elicitor pattern"
```

---

### Task 3: Log `purpose` in the middleware

**Files:**
- Modify: `tools/middleware.go`

- [ ] **Step 1: Add purpose logging to `withLogging`**

In `withLogging`, after the existing `object_uri` block, add:

```go
// before (existing block):
if uri := req.GetString(paramObjectURI, ""); uri != "" {
    attrs = append(attrs, slog.String("object_uri", uri))
}
```

```go
// after:
if uri := req.GetString(paramObjectURI, ""); uri != "" {
    attrs = append(attrs, slog.String("object_uri", uri))
}
if purpose := req.GetString("purpose", ""); purpose != "" {
    attrs = append(attrs, slog.String("purpose", purpose))
}
```

- [ ] **Step 2: Run tests**

```
go test ./tools/... -run "TestMiddleware" -v
```

Expected: existing middleware tests PASS (no change to their behavior).

- [ ] **Step 3: Commit**

```
git add tools/middleware.go
git commit -m "feat: log 'purpose' attribute in tool call middleware"
```

---

### Task 4: Extend `synthesizeArgs` to pick first enum value

**Files:**
- Modify: `tools/structured_content_shape_test.go`

This ensures the reflective shape test exercises the `run_query` success path (valid purpose → RunQuery called) rather than the hard-block path.

- [ ] **Step 1: Add enum-aware default to `defaultValueFromSchema`**

In `defaultValueFromSchema`, add an `enum` check before the type switch:

```go
// before:
func defaultValueFromSchema(schema map[string]any) any {
	if schema == nil {
		return "x"
	}
	// oneOf: pick the first branch's default.
	if oneOf, ok := schema["oneOf"].([]any); ok && len(oneOf) > 0 {
		if first, ok := oneOf[0].(map[string]any); ok {
			return defaultValueFromSchema(first)
		}
	}
	switch t, _ := schema["type"].(string); t {
```

```go
// after:
func defaultValueFromSchema(schema map[string]any) any {
	if schema == nil {
		return "x"
	}
	// enum: pick the first allowed value rather than a generic "x".
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	// oneOf: pick the first branch's default.
	if oneOf, ok := schema["oneOf"].([]any); ok && len(oneOf) > 0 {
		if first, ok := oneOf[0].(map[string]any); ok {
			return defaultValueFromSchema(first)
		}
	}
	switch t, _ := schema["type"].(string); t {
```

- [ ] **Step 2: Run the shape test**

```
go test ./tools/... -run "TestStructuredContentIsObject" -v
```

Expected: `run_query` subtest PASS (synthesized `purpose` is now `"ddic_inspection"`, which is valid, so RunQuery is called and returns a success result with a conforming `structuredContent`).

- [ ] **Step 3: Run the full test suite**

```
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```
git add tools/structured_content_shape_test.go
git commit -m "test: synthesize first enum value in reflective shape test"
```

---

### Known Limitations (no code change required)

- **Elicitation not supported:** `ConfirmDestructive` silently passes through when the MCP client returns `ErrElicitationNotSupported` or `ErrNoActiveSession`. This is a backwards-compatibility contract shared by all elicitor usages in this repo, not a regression introduced here. Enforcement is best-effort on clients that don't support elicitation.
- **`RegisterAll` wrapper:** `RegisterAll` (used in some test harnesses) calls `RegisterAllWithLockMap` with `nil` as the elicitor. After this change, any `run_query` call via `RegisterAll` without a valid `purpose` will hard-block. This is intentional — `RegisterAll` is a convenience wrapper; production binaries use `RegisterAllWithLockMap` directly.

---

### Task 5: Run linter and format check

- [ ] **Step 1: Format**

```
gofmt -w tools/query.go tools/query_test.go tools/middleware.go tools/structured_content_shape_test.go
```

- [ ] **Step 2: Vet**

```
go vet ./...
```

Expected: no output (no issues).

- [ ] **Step 3: Lint (if golangci-lint is available)**

```
make lint
```

Expected: no new lint findings introduced by this change.
