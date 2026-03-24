# patch_source, set_source_from_file, and Lock-Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `patch_source` and `set_source_from_file` MCP tools with a server-side lock map, remove `set_source`, and support batch activation — enabling practical multi-object refactoring sessions.

**Architecture:** A thread-safe `LockMap` in the tools layer tracks `{lockHandle, etag}` per system+objectURI. New tools (`patch_source`, `set_source_from_file`) and modified tools (`lock_object`, `unlock_object`, `get_source`, `activate_objects`) use the map to eliminate manual handle/etag threading. The `adt.Client` interface gets two changes: `SetSource` returns `(string, error)` for ETag capture, and `ActivateObjects` accepts `[]string`.

**Tech Stack:** Go 1.26, mcp-go v0.45.0, `sync.RWMutex` for lock map concurrency

**Spec:** `docs/superpowers/specs/2026-03-24-patch-source-and-file-upload-design.md`

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `adt/lockmap.go` | `LockMap` type: thread-safe map with Get/Set/Delete/Clear |
| `adt/lockmap_test.go` | Unit tests for LockMap |
| `tools/patch.go` | `patch_source` tool: parse operations, apply to source, write back |
| `tools/patch_test.go` | Tests for patch_source tool |
| `tools/file_source.go` | `set_source_from_file` tool: read file, write to SAP |
| `tools/file_source_test.go` | Tests for set_source_from_file tool |

### Modified files
| File | Change |
|------|--------|
| `adt/client.go:22` | `SetSource` returns `(string, error)`, add `ActivateObjects(ctx, []string)` |
| `adt/source.go:28` | Capture and return ETag from PUT response |
| `adt/activate.go:36` | Rename to `ActivateObjects`, accept `[]string`, keep `ActivateObject` wrapper |
| `adt/registry.go:100-103` | Update `SetSource` and `ActivateObject(s)` delegation |
| `adt/registry_test.go` | Update delegation tests for new signatures |
| `tools/register.go:22` | Create LockMap, pass to tool registrations |
| `tools/lock.go` | Lock map integration, optional `lock_handle` on unlock |
| `tools/source.go` | Remove `set_source`, add lock map ETag update to `get_source` |
| `tools/activate.go` | Replace with `activate_objects` (plural), keep alias |
| `tools/source_test.go` | Update mockClient, remove set_source tests |
| `tools/tools_extra_test.go` | Update lock/unlock tests for lock map |

---

## Task 1: LockMap

**Files:**
- Create: `adt/lockmap.go`
- Create: `adt/lockmap_test.go`

- [ ] **Step 1: Write failing tests for LockMap**

```go
// adt/lockmap_test.go
package adt_test

import (
	"fmt"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestLockMapSetAndGet(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")

	state, ok := m.Get("sys:uri1")
	if !ok {
		t.Fatal("expected entry")
	}
	if state.LockHandle != "handle1" || state.ETag != "etag1" {
		t.Errorf("got %+v", state)
	}
}

func TestLockMapGetMissing(t *testing.T) {
	m := adt.NewLockMap()
	_, ok := m.Get("sys:uri1")
	if ok {
		t.Fatal("expected no entry")
	}
}

func TestLockMapUpdateETag(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")
	m.UpdateETag("sys:uri1", "etag2")

	state, _ := m.Get("sys:uri1")
	if state.ETag != "etag2" {
		t.Errorf("etag: got %q", state.ETag)
	}
}

func TestLockMapUpdateETagMissing(t *testing.T) {
	m := adt.NewLockMap()
	m.UpdateETag("sys:missing", "etag2") // should not panic
}

func TestLockMapDelete(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")
	m.Delete("sys:uri1")

	_, ok := m.Get("sys:uri1")
	if ok {
		t.Fatal("expected deleted")
	}
}

func TestLockMapConcurrent(t *testing.T) {
	m := adt.NewLockMap()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := fmt.Sprintf("sys:uri%d", n)
			m.Set(key, "handle", "etag")
			m.Get(key)
			m.UpdateETag(key, "etag2")
			m.Delete(key)
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./adt/... -run TestLockMap -v`
Expected: FAIL — `adt.NewLockMap` undefined

- [ ] **Step 3: Implement LockMap**

```go
// adt/lockmap.go
package adt

import "sync"

// LockState holds the lock handle and ETag for a locked object.
type LockState struct {
	LockHandle string
	ETag       string
}

// LockMap is a thread-safe map tracking active locks per system:objectURI.
type LockMap struct {
	mu    sync.RWMutex
	locks map[string]LockState
}

// NewLockMap creates a new empty LockMap.
func NewLockMap() *LockMap {
	return &LockMap{locks: make(map[string]LockState)}
}

// Set stores or overwrites a lock entry.
func (m *LockMap) Set(key, lockHandle, etag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locks[key] = LockState{LockHandle: lockHandle, ETag: etag}
}

// Get retrieves a lock entry. Returns false if not found.
func (m *LockMap) Get(key string) (LockState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.locks[key]
	return s, ok
}

// UpdateETag updates only the ETag for an existing entry. No-op if key is missing.
func (m *LockMap) UpdateETag(key, etag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.locks[key]; ok {
		s.ETag = etag
		m.locks[key] = s
	}
}

// Delete removes a lock entry.
func (m *LockMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, key)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./adt/... -run TestLockMap -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add adt/lockmap.go adt/lockmap_test.go
git commit -m "feat: add thread-safe LockMap for tracking active locks"
```

---

## Task 2: SetSource Returns ETag

**Files:**
- Modify: `adt/client.go:22` (interface)
- Modify: `adt/source.go:28` (implementation)
- Modify: `adt/registry.go:100` (delegation)
- Modify: `adt/registry_test.go` (delegation test)
- Modify: `tools/source_test.go` (mockClient)
- Modify: `tools/source.go` (set_source caller — will be removed later but must compile)

- [ ] **Step 1: Update mockClient in `tools/source_test.go`**

Change the `setSourceFn` field from `func(...) error` to `func(...) (string, error)` and update the `SetSource` method:

In `tools/source_test.go`, find the mockClient struct field:
```go
setSourceFn func(ctx context.Context, uri, source, lockHandle, transport, etag string) error
```
Change to:
```go
setSourceFn func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error)
```

And the method:
```go
func (m *mockClient) SetSource(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
	if m.setSourceFn != nil {
		return m.setSourceFn(ctx, uri, source, lockHandle, transport, etag)
	}
	return "new-etag", nil
}
```

- [ ] **Step 2: Update Client interface in `adt/client.go:22`**

```go
SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error)
```

- [ ] **Step 3: Update implementation in `adt/source.go`**

Replace the `SetSource` method to capture and return the response ETag:

```go
func (c *httpClient) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	headers := map[string]string{
		"Content-Type": "plain/abap; charset=utf-8",
		"If-Match":     etag,
	}
	if lockHandle != "" {
		headers["X-SAP-Lock-Handle"] = lockHandle
	}
	path := objectURI + "/source/main"
	if transport != "" {
		path += "?corrNr=" + url.QueryEscape(transport)
	}
	resp, err := c.doMutate(ctx, http.MethodPut, path,
		strings.NewReader(source),
		headers,
	)
	if err != nil {
		return "", fmt.Errorf("SetSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}
```

- [ ] **Step 4: Update registry delegation in `adt/registry.go`**

```go
func (r *ClientRegistry) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	return r.activeClient().SetSource(ctx, objectURI, source, lockHandle, transport, etag)
}
```

- [ ] **Step 5: Update `tools/source.go` set_source handler to use new return type**

Change the handler to capture the return value (even though we'll remove set_source later, it must compile now):

```go
if _, err := client.SetSource(ctx, uri, source, lockHandle, transport, etag); err != nil {
```

- [ ] **Step 6: Update set_source tests in `tools/source_test.go`**

Any test that sets `setSourceFn` must return `(string, error)` instead of `error`. E.g.:
```go
setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
    // capture params...
    return "new-etag", nil
},
```

For error tests:
```go
setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
    return "", &adt.ADTError{StatusCode: 412, Message: "ETag mismatch"}
},
```

- [ ] **Step 7: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add adt/client.go adt/source.go adt/registry.go adt/registry_test.go tools/source.go tools/source_test.go
git commit -m "feat: SetSource returns new ETag from response"
```

---

## Task 3: ActivateObjects (Plural)

**Files:**
- Modify: `adt/client.go:22` (interface)
- Modify: `adt/activate.go` (implementation)
- Modify: `adt/registry.go` (delegation)
- Modify: `adt/registry_test.go`
- Modify: `tools/activate.go` (tool registration)
- Modify: `tools/source_test.go` (mockClient)

- [ ] **Step 1: Write failing test for ActivateObjects in `adt/activate_test.go`**

Find the existing activation test and add a multi-URI test. If no `activate_test.go` exists, add the test to the appropriate test file. The test should verify that all URIs appear in the request XML.

- [ ] **Step 2: Update Client interface in `adt/client.go`**

Replace:
```go
ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error)
```
With:
```go
ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error)
```

- [ ] **Step 3: Update implementation in `adt/activate.go`**

Rename to `ActivateObjects`, accept `[]string`:

```go
func (c *httpClient) ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error) {
	objects := make([]xmlActivationObject, len(objectURIs))
	for i, uri := range objectURIs {
		objects[i] = xmlActivationObject{URI: uri}
	}
	bodyXML, err := xml.Marshal(xmlActivationRequest{
		NS:      nsADTCore,
		Objects: objects,
	})
	// ... rest unchanged
}
```

- [ ] **Step 4: Update registry delegation in `adt/registry.go`**

```go
func (r *ClientRegistry) ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error) {
	return r.activeClient().ActivateObjects(ctx, objectURIs)
}
```

- [ ] **Step 5: Update mockClient in `tools/source_test.go`**

Replace `activateObjectFn` with `activateObjectsFn`:
```go
activateObjectsFn func(ctx context.Context, uris []string) (*adt.ActivationResult, error)
```

Method:
```go
func (m *mockClient) ActivateObjects(ctx context.Context, uris []string) (*adt.ActivationResult, error) {
	if m.activateObjectsFn != nil {
		return m.activateObjectsFn(ctx, uris)
	}
	return &adt.ActivationResult{Success: true}, nil
}
```

- [ ] **Step 6: Update `tools/activate.go` — register both `activate_objects` and `activate_object`**

```go
func registerActivateTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("activate_objects",
		mcp.WithDescription("Activate one or more ABAP objects in SAP. Returns success status and any activation messages."),
		mcp.WithArray("object_uris",
			mcp.Required(),
			mcp.Description("List of ADT object URIs to activate"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawURIs := req.GetStringSlice("object_uris", nil)
		result, err := client.ActivateObjects(ctx, rawURIs)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})

	// Backward-compatible alias
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithDescription("Activate a single ABAP object in SAP (alias for activate_objects)."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		result, err := client.ActivateObjects(ctx, []string{uri})
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 7: Update activation tests in `tools/source_test.go` or `tools/tools_extra_test.go`**

Update `TestActivateObjectTool` and `TestActivateObjectToolError` to use the new `activateObjectsFn` mock field. Add `TestActivateObjectsTool` for multi-URI.

- [ ] **Step 8: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add adt/client.go adt/activate.go adt/registry.go adt/registry_test.go tools/activate.go tools/source_test.go tools/tools_extra_test.go
git commit -m "feat: ActivateObjects accepts multiple URIs for batch activation"
```

---

## Task 4: Lock Map Integration into Existing Tools

**Files:**
- Modify: `tools/register.go` — create LockMap, pass to registrations
- Modify: `tools/lock.go` — store/lookup/delete in lock map
- Modify: `tools/source.go` — remove `set_source`, update `get_source` to update lock map ETag
- Modify: `tools/source_test.go` — remove set_source tests, update get_source tests
- Modify: `tools/tools_extra_test.go` — update lock/unlock tests

- [ ] **Step 1: Update `tools/register.go`**

Add LockMap creation and pass to tool registrations. The `SystemSelector` interface already provides `ActiveName()` which we need for lock map keys.

```go
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	lockMap := adt.NewLockMap()
	registerSourceTools(s, client, lockMap, selector)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
	registerLockTools(s, client, lockMap, selector)
	registerPrettyPrinterTools(s, client)
	registerObjectTools(s, client)
	registerCompletionTools(s, client)
	registerSystemTools(s, selector)
	registerPatchTools(s, client, lockMap, selector)
	registerFileSourceTools(s, client, lockMap, selector)
}
```

Add a helper for building lock map keys:

```go
func lockKey(selector SystemSelector, objectURI string) string {
	return selector.ActiveName() + ":" + objectURI
}
```

- [ ] **Step 2: Update `tools/lock.go`**

Change signature: `func registerLockTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector)`

`lock_object` handler — store in lock map after successful lock:
```go
handle, err := client.LockObject(ctx, uri)
if err != nil {
    return errorResult(err), nil
}
lockMap.Set(lockKey(selector, uri), handle, "")
return mcp.NewToolResultText(handle), nil
```

`unlock_object` handler — `lock_handle` becomes optional, lookup from map:
```go
mcp.WithString("lock_handle",
    mcp.Description("Lock handle returned by lock_object (optional, looked up automatically)"),
),
// ...
lockHandle := req.GetString("lock_handle", "")
if lockHandle == "" {
    if state, ok := lockMap.Get(lockKey(selector, uri)); ok {
        lockHandle = state.LockHandle
    }
}
if lockHandle == "" {
    return errorResult(fmt.Errorf("no lock handle: object not locked")), nil
}
if err := client.UnlockObject(ctx, uri, lockHandle); err != nil {
    return errorResult(err), nil
}
lockMap.Delete(lockKey(selector, uri))
return mcp.NewToolResultText("Object unlocked"), nil
```

- [ ] **Step 3: Update `tools/source.go`**

Change signature: `func registerSourceTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector)`

Remove the entire `set_source` tool registration.

Update `get_source` handler to update lock map ETag:
```go
result, err := client.GetSource(ctx, uri)
if err != nil {
    return errorResult(err), nil
}
lockMap.UpdateETag(lockKey(selector, uri), result.ETag)
// ... rest unchanged
```

- [ ] **Step 4: Update test helpers in `tools/source_test.go`**

Update `newTestServer` and `newTestServerWithSelector` to create a LockMap and pass it. The mockSelector already has `ActiveName()`. Remove all `TestSetSourceTool*` tests. Add a test that verifies `get_source` updates the lock map ETag.

- [ ] **Step 5: Update `tools/tools_extra_test.go`**

Update lock/unlock tests. Verify `lock_object` stores in lock map. Verify `unlock_object` without `lock_handle` param looks up from lock map. Verify `unlock_object` deletes from lock map.

- [ ] **Step 6: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add tools/register.go tools/lock.go tools/source.go tools/source_test.go tools/tools_extra_test.go
git commit -m "feat: integrate lock map into existing tools, remove set_source"
```

---

## Task 5: patch_source Tool

**Files:**
- Create: `tools/patch.go`
- Create: `tools/patch_test.go`

- [ ] **Step 1: Write failing tests for patch operations (unit-level)**

Test the patch application logic in isolation (a pure function that takes source lines + operations → modified source):

```go
// tools/patch_test.go
package tools_test

import (
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/tools"
)

func TestApplyOpsInsert(t *testing.T) {
	source := "line1\nline2\nline3"
	ops := []tools.PatchOp{{Type: "insert", AfterLine: 1, Content: "inserted"}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "line1\ninserted\nline2\nline3" {
		t.Errorf("got:\n%s", result)
	}
}

func TestApplyOpsInsertAtZero(t *testing.T) {
	source := "line1\nline2"
	ops := []tools.PatchOp{{Type: "insert", AfterLine: 0, Content: "header"}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "header\nline1\nline2" {
		t.Errorf("got:\n%s", result)
	}
}

func TestApplyOpsReplace(t *testing.T) {
	source := "line1\nline2\nline3\nline4"
	ops := []tools.PatchOp{{Type: "replace", FromLine: 2, ToLine: 3, Content: "replaced"}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "line1\nreplaced\nline4" {
		t.Errorf("got:\n%s", result)
	}
}

func TestApplyOpsDelete(t *testing.T) {
	source := "line1\nline2\nline3\nline4"
	ops := []tools.PatchOp{{Type: "delete", FromLine: 2, ToLine: 3}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "line1\nline4" {
		t.Errorf("got:\n%s", result)
	}
}

func TestApplyOpsSearchReplace(t *testing.T) {
	source := "CALL FUNCTION old_func.\nDATA: lv_old TYPE i."
	ops := []tools.PatchOp{{Type: "search_replace", Search: "old_func", Replace: "new_func"}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "CALL FUNCTION new_func.\nDATA: lv_old TYPE i." {
		t.Errorf("got:\n%s", result)
	}
}

func TestApplyOpsSearchReplaceAll(t *testing.T) {
	source := "old old old"
	ops := []tools.PatchOp{{Type: "search_replace", Search: "old", Replace: "new", All: true}}
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result != "new new new" {
		t.Errorf("got: %s", result)
	}
}

func TestApplyOpsMultiple(t *testing.T) {
	source := "line1\nline2\nline3\nline4\nline5"
	ops := []tools.PatchOp{
		{Type: "delete", FromLine: 2, ToLine: 2},
		{Type: "insert", AfterLine: 4, Content: "inserted"},
	}
	// Sorted descending: insert at 4 first, then delete at 2
	result, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	expected := "line1\nline3\nline4\ninserted\nline5"
	if result != expected {
		t.Errorf("got:\n%s\nwant:\n%s", result, expected)
	}
}

func TestApplyOpsOverlapRejected(t *testing.T) {
	source := "line1\nline2\nline3\nline4\nline5"
	ops := []tools.PatchOp{
		{Type: "delete", FromLine: 2, ToLine: 4},
		{Type: "replace", FromLine: 3, ToLine: 5, Content: "x"},
	}
	_, err := tools.ApplyPatchOps(source, ops)
	if err == nil {
		t.Fatal("expected overlap error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tools/... -run TestApplyOps -v`
Expected: FAIL — `ApplyPatchOps` undefined

- [ ] **Step 3: Implement patch application logic**

```go
// tools/patch.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type PatchOp struct {
	Type      string `json:"type"`
	AfterLine int    `json:"after_line,omitempty"`
	FromLine  int    `json:"from_line,omitempty"`
	ToLine    int    `json:"to_line,omitempty"`
	Content   string `json:"content,omitempty"`
	Search    string `json:"search,omitempty"`
	Replace   string `json:"replace,omitempty"`
	All       bool   `json:"all,omitempty"`
}

// sortKey returns the primary line number for sorting (descending).
func (o PatchOp) sortKey() int {
	switch o.Type {
	case "insert":
		return o.AfterLine
	case "replace", "delete":
		return o.FromLine
	default:
		return 0
	}
}

// ApplyPatchOps applies patch operations to source text.
// Line-based ops are sorted descending and applied bottom-to-top.
// search_replace ops run after all line-based ops.
func ApplyPatchOps(source string, ops []PatchOp) (string, error) {
	var lineOps, searchOps []PatchOp
	for _, op := range ops {
		if op.Type == "search_replace" {
			searchOps = append(searchOps, op)
		} else {
			lineOps = append(lineOps, op)
		}
	}

	// Sort line ops descending by sort key
	sort.Slice(lineOps, func(i, j int) bool {
		return lineOps[i].sortKey() > lineOps[j].sortKey()
	})

	// Check for overlaps
	for i := 0; i < len(lineOps)-1; i++ {
		curr := lineOps[i]
		next := lineOps[i+1] // next has lower/equal line number
		var currMin int
		switch curr.Type {
		case "insert":
			continue // inserts don't occupy a range
		case "replace", "delete":
			currMin = curr.FromLine
		}
		var nextMax int
		switch next.Type {
		case "insert":
			continue
		case "replace", "delete":
			nextMax = next.ToLine
		}
		if nextMax >= currMin {
			return "", fmt.Errorf("overlapping operations: lines %d-%d and %d-%d",
				next.FromLine, next.ToLine, curr.FromLine, curr.ToLine)
		}
	}

	lines := strings.Split(source, "\n")

	// Apply line ops (already sorted descending)
	for _, op := range lineOps {
		switch op.Type {
		case "insert":
			if op.AfterLine < 0 || op.AfterLine > len(lines) {
				return "", fmt.Errorf("insert: after_line %d out of range (0-%d)", op.AfterLine, len(lines))
			}
			newLines := strings.Split(op.Content, "\n")
			result := make([]string, 0, len(lines)+len(newLines))
			result = append(result, lines[:op.AfterLine]...)
			result = append(result, newLines...)
			result = append(result, lines[op.AfterLine:]...)
			lines = result

		case "replace":
			if op.FromLine < 1 || op.ToLine > len(lines) || op.FromLine > op.ToLine {
				return "", fmt.Errorf("replace: lines %d-%d out of range (1-%d)", op.FromLine, op.ToLine, len(lines))
			}
			newLines := strings.Split(op.Content, "\n")
			result := make([]string, 0, len(lines)-op.ToLine+op.FromLine+len(newLines))
			result = append(result, lines[:op.FromLine-1]...)
			result = append(result, newLines...)
			result = append(result, lines[op.ToLine:]...)
			lines = result

		case "delete":
			if op.FromLine < 1 || op.ToLine > len(lines) || op.FromLine > op.ToLine {
				return "", fmt.Errorf("delete: lines %d-%d out of range (1-%d)", op.FromLine, op.ToLine, len(lines))
			}
			lines = append(lines[:op.FromLine-1], lines[op.ToLine:]...)

		default:
			return "", fmt.Errorf("unknown operation type: %q", op.Type)
		}
	}

	// Apply search/replace ops
	result := strings.Join(lines, "\n")
	for _, op := range searchOps {
		if op.All {
			result = strings.ReplaceAll(result, op.Search, op.Replace)
		} else {
			result = strings.Replace(result, op.Search, op.Replace, 1)
		}
	}

	return result, nil
}
```

- [ ] **Step 4: Run patch operation tests**

Run: `go test ./tools/... -run TestApplyOps -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add tools/patch.go tools/patch_test.go
git commit -m "feat: patch operation application logic with tests"
```

- [ ] **Step 6: Write failing tests for patch_source MCP tool**

```go
// Add to tools/patch_test.go

func TestPatchSourceToolSearchReplace(t *testing.T) {
	var gotURI, gotSource, gotHandle, gotTransport, gotETag string
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "line1\nold_var\nline3", ETag: "etag1"}, nil
		},
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			gotURI, gotSource, gotHandle, gotTransport, gotETag = uri, source, lockHandle, transport, etag
			return "etag2", nil
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "handle1", "etag1")
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": testObjectURI,
		"operations": []interface{}{
			map[string]interface{}{"type": "search_replace", "search": "old_var", "replace": "new_var"},
		},
		"transport": "DEVK900001",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if gotSource != "line1\nnew_var\nline3" {
		t.Errorf("source: got %q", gotSource)
	}
	if gotHandle != "handle1" {
		t.Errorf("handle: got %q", gotHandle)
	}
	if gotTransport != "DEVK900001" {
		t.Errorf("transport: got %q", gotTransport)
	}
}

func TestPatchSourceToolAutoLock(t *testing.T) {
	lockCalled := false
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			lockCalled = true
			return "auto-handle", nil
		},
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "line1\nline2", ETag: "etag1"}, nil
		},
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			return "etag2", nil
		},
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": testObjectURI,
		"operations": []interface{}{
			map[string]interface{}{"type": "insert", "after_line": 1, "content": "new"},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if !lockCalled {
		t.Error("expected auto-lock")
	}
	// Verify lock map was populated
	state, ok := lockMap.Get("dev:" + testObjectURI)
	if !ok {
		t.Fatal("expected lock map entry")
	}
	if state.LockHandle != "auto-handle" {
		t.Errorf("handle: got %q", state.LockHandle)
	}
}
```

Note: `newTestServerWithLockMap` is a new helper that creates the server with a specific lock map. Add it alongside existing test helpers.

- [ ] **Step 6.1: Write additional edge case tests**

Add to `tools/patch_test.go`:

```go
func TestPatchSourceToolExplicitLockHandle(t *testing.T) {
	var gotHandle string
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "line1", ETag: "etag1"}, nil
		},
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			gotHandle = lockHandle
			return "etag2", nil
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "map-handle", "etag1")
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri":  testObjectURI,
		"lock_handle": "explicit-handle",
		"operations":  []interface{}{map[string]interface{}{"type": "insert", "after_line": 0, "content": "new"}},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if gotHandle != "explicit-handle" {
		t.Errorf("expected explicit handle, got %q", gotHandle)
	}
}

func TestPatchSourceToolGetSourceError(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "handle1", "etag1")
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": testObjectURI,
		"operations": []interface{}{map[string]interface{}{"type": "insert", "after_line": 0, "content": "new"}},
	})
	if !result.IsError {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 7: Implement `patch_source` tool registration**

Add to `tools/patch.go`:

```go
func registerPatchTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("patch_source",
		mcp.WithDescription("Apply surgical edits to ABAP source code. Supports insert, replace, delete, and search_replace operations. Auto-locks if needed."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithArray("operations",
			mcp.Required(),
			mcp.Description(`Edit operations: {"type":"insert","after_line":N,"content":"..."}, {"type":"replace","from_line":N,"to_line":M,"content":"..."}, {"type":"delete","from_line":N,"to_line":M}, {"type":"search_replace","search":"...","replace":"...","all":false}`),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Explicit lock handle (optional, looked up from lock map)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")
		key := lockKey(selector, uri)

		// Resolve lock handle
		lockHandle := explicitHandle
		if lockHandle == "" {
			if state, ok := lockMap.Get(key); ok {
				lockHandle = state.LockHandle
			}
		}

		// Auto-lock if needed
		if lockHandle == "" {
			handle, err := client.LockObject(ctx, uri)
			if err != nil {
				return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
			}
			lockHandle = handle
			lockMap.Set(key, handle, "")
		}

		// Get current source
		sourceResult, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(key, sourceResult.ETag)

		// Parse operations
		rawOps, _ := json.Marshal(req.Params.Arguments["operations"])
		var ops []PatchOp
		if err := json.Unmarshal(rawOps, &ops); err != nil {
			return errorResult(fmt.Errorf("invalid operations: %w", err)), nil
		}

		// Apply operations
		modified, err := ApplyPatchOps(sourceResult.Source, ops)
		if err != nil {
			return errorResult(err), nil
		}

		// Write back
		newETag, err := client.SetSource(ctx, uri, modified, lockHandle, transport, sourceResult.ETag)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(key, newETag)

		out, _ := json.Marshal(map[string]interface{}{
			"success":       true,
			"line_delta": lineDelta(sourceResult.Source, modified),
			"locked":        true,
			"lock_handle":   lockHandle,
			"etag":          newETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}

func lineDelta(old, new string) int {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")
	diff := len(newLines) - len(oldLines)
	if diff < 0 {
		diff = -diff
	}
	return diff
}
```

- [ ] **Step 8: Run patch_source tests**

Run: `go test ./tools/... -run TestPatchSource -v`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add tools/patch.go tools/patch_test.go
git commit -m "feat: add patch_source MCP tool with auto-lock support"
```

---

## Task 6: set_source_from_file Tool

**Files:**
- Create: `tools/file_source.go`
- Create: `tools/file_source_test.go`

- [ ] **Step 1: Write failing tests**

```go
// tools/file_source_test.go
package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestSetSourceFromFileTool(t *testing.T) {
	// Write temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "source.abap")
	os.WriteFile(path, []byte("REPORT ztest.\n* from file"), 0644)

	var gotSource, gotHandle string
	mock := &mockClient{
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			gotSource, gotHandle = source, lockHandle
			return "etag2", nil
		},
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "old", ETag: "etag1"}, nil
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "handle1", "")
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  path,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if gotSource != "REPORT ztest.\n* from file" {
		t.Errorf("source: got %q", gotSource)
	}
	if gotHandle != "handle1" {
		t.Errorf("handle: got %q", gotHandle)
	}
}

func TestSetSourceFromFileToolMissingFile(t *testing.T) {
	mock := &mockClient{}
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "handle1", "etag1")
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  "/nonexistent/file.abap",
	})
	if !result.IsError {
		t.Fatal("expected error for missing file")
	}
}

func TestSetSourceFromFileToolAutoLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.abap")
	os.WriteFile(path, []byte("source"), 0644)

	lockCalled := false
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			lockCalled = true
			return "auto-handle", nil
		},
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "old", ETag: "etag1"}, nil
		},
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			return "etag2", nil
		},
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  path,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if !lockCalled {
		t.Error("expected auto-lock")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tools/... -run TestSetSourceFromFile -v`
Expected: FAIL — tool not registered

- [ ] **Step 3: Implement `set_source_from_file` tool**

```go
// tools/file_source.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerFileSourceTools(s *server.MCPServer, client adt.Client, lockMap *adt.LockMap, selector SystemSelector) {
	s.AddTool(mcp.NewTool("set_source_from_file",
		mcp.WithDescription("Upload ABAP source code from a local file to SAP. Auto-locks if needed."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to source file (absolute or relative to working directory)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Explicit lock handle (optional, looked up from lock map)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		filePath := req.GetString("file_path", "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")
		key := lockKey(selector, uri)

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			return errorResult(fmt.Errorf("reading file: %w", err)), nil
		}
		source := string(data)

		// Resolve lock handle
		lockHandle := explicitHandle
		if lockHandle == "" {
			if state, ok := lockMap.Get(key); ok {
				lockHandle = state.LockHandle
			}
		}

		// Auto-lock if needed
		if lockHandle == "" {
			handle, err := client.LockObject(ctx, uri)
			if err != nil {
				return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
			}
			lockHandle = handle
			lockMap.Set(key, handle, "")
		}

		// Get ETag if not in lock map
		state, _ := lockMap.Get(key)
		etag := state.ETag
		if etag == "" {
			sourceResult, err := client.GetSource(ctx, uri)
			if err != nil {
				return errorResult(err), nil
			}
			etag = sourceResult.ETag
		}

		// Write source
		newETag, err := client.SetSource(ctx, uri, source, lockHandle, transport, etag)
		if err != nil {
			return errorResult(err), nil
		}
		lockMap.UpdateETag(key, newETag)

		lineCount := len(strings.Split(source, "\n"))
		out, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"lines":       lineCount,
			"locked":      true,
			"lock_handle": lockHandle,
			"etag":        newETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

Note: Add `"strings"` to imports.

- [ ] **Step 4: Run tests**

Run: `go test ./tools/... -run TestSetSourceFromFile -v`
Expected: all PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add tools/file_source.go tools/file_source_test.go
git commit -m "feat: add set_source_from_file MCP tool"
```

---

## Task 7: Integration Verification and Cleanup

- [ ] **Step 1: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: all PASS, no data races

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...` (or `go vet ./...` if golangci-lint not available)
Expected: no errors

- [ ] **Step 3: Build and verify the binary starts**

Run: `go build -o mcp-server-abap.exe .`
Expected: binary builds without error

- [ ] **Step 4: Update `test_flow.py` to use `patch_source` instead of `set_source`**

Replace the set_source call (message id 3) with a patch_source call:
```python
# 3. Patch Source (instead of set_source with full body)
r = send_and_read(proc, {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{
    "name":"patch_source","arguments":{
        "object_uri": URI,
        "operations": [
            {"type": "insert", "after_line": 1, "content": "* Patched via MCP Server - " + str(int(time.time()))}
        ],
        "transport": TRANSPORT
    }}})
```

Remove the separate fetch session at the top (patch_source fetches source internally).

- [ ] **Step 5: Commit**

```bash
git add test_flow.py
git commit -m "test: update test_flow.py to use patch_source"
```
