# get_object_dependencies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `get_object_dependencies` MCP tool that returns all objects a given ABAP object references, by querying the `WBCROSSGT` cross-reference table — the forward-direction counterpart to `where_used`.

**Architecture:** `registerSearchTools` in `tools/search.go` is widened to accept a combined `searchQueryClient` interface (embedding both `adt.SearchClient` and `adt.QueryClient`). A new `s.AddTool` call for `get_object_dependencies` is added directly after `where_used`, using `client.RunQuery` to query `WBCROSSGT`. `register.go` needs no changes — it already passes `adt.Client` which satisfies the combined interface.

**Tech Stack:** Go 1.26, `github.com/Hochfrequenz/adtler/adt` (`SearchClient`, `QueryClient`, `QueryResult`, `QueryColumn`), `github.com/mark3labs/mcp-go/mcp`

---

## File Map

| Action | File | What changes |
|--------|------|--------------|
| Modify | `tools/search.go` | Widen `registerSearchTools` signature; add `get_object_dependencies` tool |
| Modify | `tools/source_test.go` | Add `runQueryFn` field to `mockClient`; update `RunQuery` stub |
| Modify | `tools/tools_extra_test.go` | Add tests for `get_object_dependencies` |

`register.go` is **not changed** — it passes `client` (`adt.Client`) which already satisfies the new combined interface.

---

## Task 1: Widen `registerSearchTools` signature and wire up `runQueryFn` in the mock

**Files:**
- Modify: `tools/search.go`
- Modify: `tools/source_test.go`

### search.go changes

- [ ] **Step 1: Add combined interface and update function signature**

In `tools/search.go`, add the following type definition before `registerSearchTools`, and update the function signature:

```go
// searchQueryClient is the combined interface required by registerSearchTools.
// It extends adt.SearchClient with adt.QueryClient so get_object_dependencies
// can call RunQuery without changing the register.go call site.
type searchQueryClient interface {
	adt.SearchClient
	adt.QueryClient
}

func registerSearchTools(s toolAdder, client searchQueryClient) {
```

The existing `register.go` call site (`registerSearchTools(ls, client)` at line 180) passes `adt.Client`, which satisfies both `adt.SearchClient` and `adt.QueryClient` — **no change needed there**.

Also add `"fmt"` and `"strings"` to the import block in `search.go` (needed in Task 3):

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)
```

- [ ] **Step 2: Confirm it compiles**

```
cd C:\Users\JonatanMeiske\Documents\50_KI_Agenten\mcp-server-abap
go build ./...
```

Expected: no errors.

### source_test.go changes

- [ ] **Step 3: Add `runQueryFn` field to `mockClient`**

In `tools/source_test.go`, find the `mockClient` struct definition. Add one field in the same style as the other `xxxFn` fields:

```go
runQueryFn func(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error)
```

- [ ] **Step 4: Update the `RunQuery` stub to call the function**

Find this existing stub:

```go
func (m *mockClient) RunQuery(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
	return nil, nil
}
```

Replace it with:

```go
func (m *mockClient) RunQuery(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
	if m.runQueryFn != nil {
		return m.runQueryFn(ctx, sql, maxRows)
	}
	return nil, nil
}
```

- [ ] **Step 5: Run existing tests to confirm nothing broke**

```
go test ./tools/... -run TestWhereUsed -v
```

Expected: all `TestWhereUsed*` tests PASS.

- [ ] **Step 6: Commit**

```bash
git add tools/search.go tools/source_test.go
git commit -m "refactor: widen registerSearchTools to accept QueryClient for dependency tool"
```

---

## Task 2: Write failing tests

**Files:**
- Modify: `tools/tools_extra_test.go`

Add four tests at the end of `tools/tools_extra_test.go`. All should fail with "unknown tool" until Task 3 is done. Make sure `"strings"` is in the import block of `tools_extra_test.go` (it is already used there — confirm with a quick look).

- [ ] **Step 1: Add the happy-path test**

Append to `tools/tools_extra_test.go`:

```go
// --- get_object_dependencies ---

func TestGetObjectDependenciesTool(t *testing.T) {
	var gotSQL string
	var gotMaxRows int
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
			gotSQL = sql
			gotMaxRows = maxRows
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{
					{Name: "REFOBJNM"},
					{Name: "REFUSETYP"},
				},
				Rows: [][]string{
					{"/HFQ/OTHER_IFACE", "USE"},
					{"/HFQ/SOME_TABL", "USE"},
				},
			}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "CLAS",
		"object_name": "/HFQ/MY_CLASS",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}

	if !strings.Contains(gotSQL, "OBJECT = 'CLAS'") {
		t.Errorf("SQL missing object type filter, got: %s", gotSQL)
	}
	if !strings.Contains(gotSQL, "OBJ_NAME = '/HFQ/MY_CLASS'") {
		t.Errorf("SQL missing object name filter, got: %s", gotSQL)
	}
	if !strings.Contains(gotSQL, "WBCROSSGT") {
		t.Errorf("SQL missing table name, got: %s", gotSQL)
	}
	if gotMaxRows != 200 {
		t.Errorf("maxRows: got %d, want 200 (default)", gotMaxRows)
	}

	text := firstText(result)
	var out struct {
		ObjectType   string `json:"object_type"`
		ObjectName   string `json:"object_name"`
		Count        int    `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal result: %v\ntext: %q", err, text)
	}
	if out.ObjectType != "CLAS" {
		t.Errorf("object_type: got %q, want %q", out.ObjectType, "CLAS")
	}
	if out.ObjectName != "/HFQ/MY_CLASS" {
		t.Errorf("object_name: got %q, want %q", out.ObjectName, "/HFQ/MY_CLASS")
	}
	if out.Count != 2 {
		t.Errorf("count: got %d, want 2", out.Count)
	}
	if len(out.Dependencies) != 2 {
		t.Fatalf("dependencies length: got %d, want 2", len(out.Dependencies))
	}
	if out.Dependencies[0].Name != "/HFQ/OTHER_IFACE" {
		t.Errorf("dep[0].name: got %q", out.Dependencies[0].Name)
	}
	if out.Dependencies[0].UseType != "USE" {
		t.Errorf("dep[0].use_type: got %q", out.Dependencies[0].UseType)
	}
}
```

- [ ] **Step 2: Add test for custom `max_results`**

```go
func TestGetObjectDependenciesToolCustomMaxResults(t *testing.T) {
	var gotMaxRows int
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, maxRows int) (*adt.QueryResult, error) {
			gotMaxRows = maxRows
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "REFOBJNM"}, {Name: "REFUSETYP"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "ZTEST",
		"max_results": float64(50),
	})
	if gotMaxRows != 50 {
		t.Errorf("maxRows: got %d, want 50", gotMaxRows)
	}
}
```

- [ ] **Step 3: Add the empty-result test**

```go
func TestGetObjectDependenciesToolEmpty(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "REFOBJNM"}, {Name: "REFUSETYP"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_STANDALONE",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Count        int `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count != 0 {
		t.Errorf("count: got %d, want 0", out.Count)
	}
	if len(out.Dependencies) != 0 {
		t.Errorf("dependencies: got %d, want 0", len(out.Dependencies))
	}
}
```

- [ ] **Step 4: Add the error-path test**

```go
func TestGetObjectDependenciesToolError(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "query failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "CLAS",
		"object_name": "/HFQ/MY_CLASS",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}
```

- [ ] **Step 5: Add the SQL-injection escaping test**

```go
func TestGetObjectDependenciesToolSQLEscaping(t *testing.T) {
	var gotSQL string
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			gotSQL = sql
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "REFOBJNM"}, {Name: "REFUSETYP"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "O'REILLY_PROG",
	})
	if !strings.Contains(gotSQL, "O''REILLY_PROG") {
		t.Errorf("single quote not escaped in SQL, got: %s", gotSQL)
	}
}
```

- [ ] **Step 6: Run tests to confirm they fail with "unknown tool"**

```
go test ./tools/... -run TestGetObjectDependencies -v
```

Expected: FAIL — JSON-RPC error "unknown tool: get_object_dependencies".

---

## Task 3: Implement the tool

**Files:**
- Modify: `tools/search.go`

- [ ] **Step 1: Add the tool registration**

In `tools/search.go`, append the following block inside `registerSearchTools`, after the closing `})` of the `where_used` tool:

```go
	s.AddTool(mcp.NewTool("get_object_dependencies",
		mcp.WithTitleAnnotation("Get Object Dependencies"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Find all ABAP objects that a given object references (forward direction). "+
				"Counterpart to where_used, which answers the reverse question. "+
				"Queries the SAP workbench cross-reference table WBCROSSGT. "+
				"Useful for transport completeness checks: given an object in a transport, "+
				"find what it depends on.",
		),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("ABAP object type, e.g. CLAS, PROG, TABL, INTF")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("Object name, e.g. /HFQ/MY_CLASS")),
		mcp.WithNumber("max_results", mcp.Description("Maximum number of results to return (default: 200)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objType := req.GetString("object_type", "")
		objName := req.GetString("object_name", "")
		maxResults := int(req.GetFloat("max_results", 200))

		safeType := strings.ReplaceAll(objType, "'", "''")
		safeName := strings.ReplaceAll(objName, "'", "''")

		sql := fmt.Sprintf(
			"SELECT REFOBJNM, REFUSETYP FROM WBCROSSGT WHERE OBJECT = '%s' AND OBJ_NAME = '%s' ORDER BY REFOBJNM",
			safeType, safeName,
		)

		queryResult, err := client.RunQuery(ctx, sql, maxResults)
		if err != nil {
			return errorResult(err), nil
		}

		type dependency struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		}

		deps := make([]dependency, 0, len(queryResult.Rows))
		for _, row := range queryResult.Rows {
			if len(row) < 2 {
				continue
			}
			deps = append(deps, dependency{Name: row[0], UseType: row[1]})
		}

		out, _ := json.Marshal(map[string]any{
			"object_type":  objType,
			"object_name":  objName,
			"count":        len(deps),
			"dependencies": deps,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
```

- [ ] **Step 2: Run the tests**

```
go test ./tools/... -run TestGetObjectDependencies -v
```

Expected: all five `TestGetObjectDependencies*` tests PASS.

- [ ] **Step 3: Run the full test suite**

```
go test ./tools/... 2>&1 | tail -20
```

Expected: no regressions, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add tools/search.go tools/tools_extra_test.go
git commit -m "feat: add get_object_dependencies tool for forward reference lookup via WBCROSSGT"
```

---

## Self-Review

**Spec coverage:**
- ✅ New tool `get_object_dependencies` in `search.go` alongside `where_used`
- ✅ Input: `object_type`, `object_name`, optional `max_results` (default 200)
- ✅ Output: `object_type`, `object_name`, `count`, `dependencies[]{name, use_type}`
- ✅ Queries `REFOBJNM, REFUSETYP` from `WBCROSSGT` (only confirmed columns, uppercase)
- ✅ Uses `QueryClient.RunQuery` via widened `searchQueryClient` interface — no new ADT endpoint
- ✅ Empty result returns `count: 0`, `dependencies: []` — not an error
- ✅ ADT errors propagate via `errorResult`
- ✅ No changes to `register.go`
- ✅ `QueryColumn.Name` field confirmed from adtler v0.1.4 source

**Type consistency:**
- `dependency.Name` / `dependency.UseType` in Task 3 match test struct fields in Task 2
- SQL uppercase identifiers (`WBCROSSGT`, `OBJECT`, `OBJ_NAME`, `REFOBJNM`, `REFUSETYP`) consistent across plan and test assertions
