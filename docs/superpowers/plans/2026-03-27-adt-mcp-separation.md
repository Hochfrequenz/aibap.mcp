# ADT Client / MCP Layer Separation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Separate the ADT client library from the MCP tool layer so a future repo split is trivial.

**Architecture:** Three phases — (1) internalize XML types, (2) move business logic from tools/ to adt/, (3) split the monolithic Client interface into focused role interfaces. Each phase is one PR.

**Tech Stack:** Go 1.26, mcp-go, SAP ADT REST API

**Spec:** `docs/superpowers/specs/2026-03-27-adt-mcp-separation-design.md`

---

## Task 1: Move `adtmodel/` → `adt/adtxml/` (PR 1)

**Files:**
- Move: `adtmodel/*.go` (14 files) → `adt/adtxml/*.go`
- Modify: 12 files in `adt/` that import `adtmodel` + `adt/query_test.go`
- Delete: `adtmodel/` directory

This is a pure mechanical rename. No logic changes.

- [ ] **Step 1: Create `adt/adtxml/` directory and move files**

```bash
mkdir -p adt/adtxml
git mv adtmodel/asxxml.go adt/adtxml/asxxml.go
git mv adtmodel/asxxml_test.go adt/adtxml/asxxml_test.go
git mv adtmodel/activation.go adt/adtxml/activation.go
git mv adtmodel/atc.go adt/adtxml/atc.go
git mv adtmodel/completion.go adt/adtxml/completion.go
git mv adtmodel/datapreview.go adt/adtxml/datapreview.go
git mv adtmodel/datapreview_test.go adt/adtxml/datapreview_test.go
git mv adtmodel/debugger.go adt/adtxml/debugger.go
git mv adtmodel/models.go adt/adtxml/models.go
git mv adtmodel/object.go adt/adtxml/object.go
git mv adtmodel/search.go adt/adtxml/search.go
git mv adtmodel/syntaxcheck.go adt/adtxml/syntaxcheck.go
git mv adtmodel/transport.go adt/adtxml/transport.go
git mv adtmodel/unittest.go adt/adtxml/unittest.go
```

- [ ] **Step 2: Update package declarations in all moved files**

In every `adt/adtxml/*.go` file, change:
```go
package adtmodel
```
to:
```go
package adtxml
```

There are 14 files (12 source + 2 test). Use find-and-replace.

- [ ] **Step 3: Update imports in `adt/` package**

In these 12 files, replace every occurrence of:
```go
"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
```
with:
```go
"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
```

And replace all `adtmodel.` references with `adtxml.`:

Files to update:
- `adt/activate.go`
- `adt/atc.go`
- `adt/completion.go`
- `adt/debugger.go`
- `adt/lock.go`
- `adt/object.go`
- `adt/query.go`
- `adt/query_test.go`
- `adt/repository.go`
- `adt/search.go`
- `adt/syntaxcheck.go`
- `adt/transport.go`
- `adt/unittest.go`

- [ ] **Step 4: Delete empty `adtmodel/` directory**

```bash
rmdir adtmodel
```

(Should be empty after git mv.)

- [ ] **Step 5: Verify**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: All pass, zero functional change.

- [ ] **Step 6: Commit**

```bash
git add adt/adtxml/ adt/activate.go adt/atc.go adt/completion.go adt/debugger.go adt/lock.go adt/object.go adt/query.go adt/query_test.go adt/repository.go adt/search.go adt/syntaxcheck.go adt/transport.go adt/unittest.go
git commit -m "refactor: move adtmodel/ → adt/adtxml/

The XML marshalling types are an implementation detail of the adt
package, never imported outside adt/. Moving them under adt/ makes
this explicit and prepares for a future repo split."
```

---

## Task 2: Move patch engine to `adt/` (PR 2)

**Files:**
- Create: `adt/patch.go` (type + functions from `tools/patch.go`)
- Create: `adt/patch_test.go` (unit tests from `tools/patch_test.go`)
- Modify: `tools/patch.go` (remove moved code, update to call `adt.ApplyPatchOps`)
- Modify: `tools/patch_test.go` (remove moved tests, update remaining to use `adt.PatchOp`)

- [ ] **Step 1: Create `adt/patch.go` with the patch engine**

Move from `tools/patch.go` lines 12-198:
- `PatchOp` struct (lines 23-39)
- `ApplyPatchOps()` (lines 64-122)
- All helpers: `lineDelta`, `primaryKey`, `opStartLine`, `opEndLine`, `splitLines`, `joinLines`, `applyLineOp`

The new file should have `package adt` and export `PatchOp` and `ApplyPatchOps`. All helpers stay unexported.

```go
package adt

// PatchOp describes a single patch operation on ABAP source code.
type PatchOp struct {
    Type      string `json:"type"`                 // "insert", "replace", "delete", "search_replace"
    AfterLine int    `json:"after_line,omitempty"`  // insert: line after which to insert
    Content   string `json:"content,omitempty"`     // insert/replace: new content
    FromLine  int    `json:"from_line,omitempty"`   // replace/delete: start line
    ToLine    int    `json:"to_line,omitempty"`     // replace/delete: end line
    Search    string `json:"search,omitempty"`      // search_replace: text to find
    Replace   string `json:"replace,omitempty"`     // search_replace: replacement text
    All       bool   `json:"all,omitempty"`         // search_replace: replace all occurrences
}

// ApplyPatchOps applies a list of patch operations to source code.
// Line-based operations (insert/replace/delete) are applied bottom-to-top.
// Search-replace operations are applied last, in order.
func ApplyPatchOps(source string, ops []PatchOp) (string, error) {
    // ... (exact code from tools/patch.go lines 64-122)
}

// ... all helper functions (unexported)
```

- [ ] **Step 2: Create `adt/patch_test.go` with unit tests**

Move from `tools/patch_test.go`:
- `TestApplyOpsInsert`
- `TestApplyOpsInsertAtZero`
- `TestApplyOpsReplace`
- `TestApplyOpsDelete`
- `TestApplyOpsSearchReplace`
- `TestApplyOpsSearchReplaceAll`
- `TestApplyOpsMultiple`
- `TestApplyOpsOverlapRejected`

Change `package tools_test` to `package adt_test`. Update references from `PatchOp` to `adt.PatchOp` and `ApplyPatchOps` to `adt.ApplyPatchOps`.

- [ ] **Step 3: Update `tools/patch.go`**

Remove all moved code. Keep only `registerPatchTools()`. Replace internal references:
- `PatchOp` → `adt.PatchOp`
- `ApplyPatchOps(...)` → `adt.ApplyPatchOps(...)`

The `import` block should now include `"github.com/Hochfrequenz/mcp-server-abap/adt"` (it already does).

- [ ] **Step 4: Update `tools/patch_test.go`**

Remove the 8 moved unit tests. Keep the integration tests that use the mock client:
- `TestPatchSourceToolSearchReplace`
- `TestPatchSourceToolAutoLock`
- `TestPatchSourceToolExplicitLockHandle`
- `TestPatchSourceToolGetSourceError`

Update any `PatchOp` references in remaining tests to `adt.PatchOp`.

- [ ] **Step 5: Verify**

```bash
go build ./...
go test ./adt/ -run "TestApplyOps" -v
go test ./tools/ -run "TestPatchSource" -v
go vet ./...
```

Expected: All pass. Patch unit tests now run under `adt/`, integration tests under `tools/`.

- [ ] **Step 6: Commit**

```bash
git add adt/patch.go adt/patch_test.go tools/patch.go tools/patch_test.go
git commit -m "refactor: move patch engine from tools/ to adt/

PatchOp type and ApplyPatchOps() are domain logic for source code
manipulation, not MCP concerns. tools/patch.go now delegates to
adt.ApplyPatchOps()."
```

---

## Task 3: Move export utilities to `adt/` (PR 3)

**Files:**
- Modify: `adt/export.go` (add moved functions)
- Create: `adt/export_test.go` (moved tests)
- Modify: `tools/export.go` (remove moved code, call `adt.*`)
- Modify: `tools/export_test.go` (remove moved tests)

- [ ] **Step 1: Add export utility functions to `adt/export.go`**

Append to the existing `adt/export.go` (which already has `ExportPackage` method). Add these as package-level functions:

From `tools/export.go`:
- `ExtractZIPToDir(data []byte, dir string) error` (lines 18-59) — **export** (capitalize)
- `WriteExport(data []byte, outputDir, packageName string, asFolder bool) (string, int, error)` (lines 61-79) — **export**
- `MatchesAnyPattern(name string, patterns []string) bool` (lines 81-93) — **export**
- `ParsePatternList(s string) ([]string, error)` (lines 95-113) — **export**

These need the same imports as before: `archive/zip`, `bytes`, `fmt`, `io`, `os`, `path/filepath`, `strings`.

- [ ] **Step 2: Create `adt/export_test.go` with moved tests**

Move from `tools/export_test.go`:
- `TestMatchesAnyPattern`
- `TestParsePatternList`
- `TestParsePatternList_Invalid`
- `TestExtractZIPToDir`
- `TestExtractZIPToDir_ZipSlip`
- `TestWriteExport_ZIP`
- `TestWriteExport_Folder`
- `createTestZIP` helper

Change `package tools_test` to `package adt_test`. Update function references to use `adt.` prefix.

- [ ] **Step 3: Update `tools/export.go`**

Remove the 4 moved functions. Update `registerExportTools()` to call:
- `adt.WriteExport(...)` instead of `writeExport(...)`
- `adt.MatchesAnyPattern(...)` instead of `matchesAnyPattern(...)`
- `adt.ParsePatternList(...)` instead of `parsePatternList(...)`

Keep `formatLabel()` in `tools/export.go` — it's MCP presentation logic.

- [ ] **Step 4: Update `tools/export_test.go`**

Remove the moved tests. Keep `TestFilteringLogic` (it tests MCP-level filtering logic). **Important:** `TestFilteringLogic` calls `matchesAnyPattern` directly (unexported). After the move, update all calls to `adt.MatchesAnyPattern` (exported). The `createTestZIP` helper may need to be duplicated or both test files can import a shared helper — simplest is to keep a copy in each test file.

- [ ] **Step 5: Verify**

```bash
go build ./...
go test ./adt/ -run "TestMatchesAny|TestParsePattern|TestExtractZIP|TestWriteExport" -v
go test ./tools/ -run "TestFiltering" -v
go vet ./...
```

Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add adt/export.go adt/export_test.go tools/export.go tools/export_test.go
git commit -m "refactor: move export utilities from tools/ to adt/

ZIP extraction, pattern matching, and file writing are domain logic,
not MCP concerns. tools/export.go now delegates to adt package
functions."
```

---

## Task 4: Lock orchestration + customizing helpers (PR 4)

**Files:**
- Modify: `adt/lockmap.go` (add `ResolveLock`, `ResolveETag`, `LockKey`)
- Create: `adt/lockmap_test.go` (new tests for orchestration methods)
- Create: `adt/custexport/config.go` (table parsing, worker capping)
- Create: `adt/custexport/config_test.go`
- Modify: `tools/register.go` (remove `lockKey`)
- Modify: `tools/patch.go` (use `adt.LockMap.ResolveLock`/`ResolveETag`)
- Modify: `tools/file_source.go` (same)
- Modify: `tools/source.go` (use `adt.LockKey`)
- Modify: `tools/customizing.go` (use `custexport.ParseTableList`, `custexport.ClampWorkers`)

### Sub-task 4a: Lock orchestration

- [ ] **Step 1: Define narrow interfaces in `adt/lockmap.go`**

Add before the `LockMap` struct (these will be formalized in Phase 3, but we need them now for `ResolveLock`/`ResolveETag`):

```go
// LockClient can lock and unlock ABAP objects.
type LockClient interface {
    LockObject(ctx context.Context, objectURI string) (string, error)
}

// SourceReader can read ABAP source code.
type SourceReader interface {
    GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
}
```

- [ ] **Step 2: Add `LockKey` function to `adt/lockmap.go`**

```go
// LockKey returns a system-qualified key for the lock map.
func LockKey(systemName, objectURI string) string {
    return systemName + ":" + objectURI
}
```

- [ ] **Step 3: Add `ResolveLock` method to `LockMap`**

```go
// ResolveLock returns a lock handle. Priority: explicitHandle > cached > auto-lock.
// If auto-locking succeeds, the lock is stored in the map with the given key.
func (m *LockMap) ResolveLock(ctx context.Context, locker LockClient, key, objectURI, explicitHandle string) (string, error) {
    if explicitHandle != "" {
        return explicitHandle, nil
    }
    if state, ok := m.Get(key); ok && state.LockHandle != "" {
        return state.LockHandle, nil
    }
    handle, err := locker.LockObject(ctx, objectURI)
    if err != nil {
        return "", fmt.Errorf("auto-lock %s: %w", objectURI, err)
    }
    m.Set(key, handle, "")
    return handle, nil
}
```

- [ ] **Step 4: Add `ResolveETag` method to `LockMap`**

```go
// ResolveETag returns a cached ETag or fetches one via GetSource.
func (m *LockMap) ResolveETag(ctx context.Context, reader SourceReader, key, objectURI string) (string, error) {
    if state, ok := m.Get(key); ok && state.ETag != "" {
        return state.ETag, nil
    }
    result, err := reader.GetSource(ctx, objectURI)
    if err != nil {
        return "", fmt.Errorf("fetch ETag for %s: %w", objectURI, err)
    }
    m.UpdateETag(key, result.ETag)
    return result.ETag, nil
}
```

- [ ] **Step 5: Write tests in `adt/lockmap_test.go`**

```go
package adt_test

// Test ResolveLock with explicit handle (no client call)
// Test ResolveLock with cached handle (no client call)
// Test ResolveLock with auto-lock (client called, stored in map)
// Test ResolveLock auto-lock error
// Test ResolveETag with cached ETag (no client call)
// Test ResolveETag fetches from client when not cached
// Test ResolveETag client error
// Test LockKey format
```

- [ ] **Step 6: Run tests**

```bash
go test ./adt/ -run "TestResolveLock|TestResolveETag|TestLockKey" -v
```

Expected: All pass.

- [ ] **Step 7: Update `tools/register.go`**

Remove `lockKey` function (lines 23-26). It's replaced by `adt.LockKey`.

- [ ] **Step 8: Update `tools/patch.go` to use new orchestration**

Replace the lock resolution block (lines ~238-255) with:
```go
key := adt.LockKey(selector.ActiveName(), uri)
handle, err := lockMap.ResolveLock(ctx, client, key, uri, explicitHandle)
```

Replace ETag resolution (lines ~257-262) with:
```go
etag, err := lockMap.ResolveETag(ctx, client, key, uri)
```

- [ ] **Step 9: Update `tools/file_source.go` similarly**

Replace lock resolution (lines ~45-61) and ETag resolution (lines ~63-72) with calls to `adt.LockKey`, `lockMap.ResolveLock`, `lockMap.ResolveETag`.

- [ ] **Step 10: Update `tools/source.go`**

Replace `lockKey(selector, uri)` on line 25 with `adt.LockKey(selector.ActiveName(), uri)`.

- [ ] **Step 11: Update `tools/lock.go`**

Replace `lockKey(selector, uri)` calls with `adt.LockKey(selector.ActiveName(), uri)`.

- [ ] **Step 12: Verify**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: All pass.

- [ ] **Step 13: Commit**

```bash
git add adt/lockmap.go adt/lockmap_test.go tools/register.go tools/patch.go tools/file_source.go tools/source.go tools/lock.go
git commit -m "refactor: move lock orchestration from tools/ to adt/

Add ResolveLock, ResolveETag, LockKey to adt.LockMap. Eliminates
duplicated lock/ETag resolution in tools/patch.go and
tools/file_source.go."
```

### Sub-task 4b: Customizing config helpers

- [ ] **Step 14: Create `adt/custexport/config.go`**

```go
package custexport

import "strings"

// MaxWorkers is the upper bound for concurrent export workers.
const MaxWorkers = 40

// DefaultWorkers is the default number of concurrent export workers.
const DefaultWorkers = 20

// ParseTableList splits a comma-separated table list, trims whitespace,
// and uppercases each entry. Empty entries are skipped.
func ParseTableList(s string) []string {
    if s == "" {
        return nil
    }
    var tables []string
    for _, t := range strings.Split(s, ",") {
        t = strings.TrimSpace(t)
        if t != "" {
            tables = append(tables, strings.ToUpper(t))
        }
    }
    return tables
}

// ClampWorkers ensures the worker count is within [1, MaxWorkers].
func ClampWorkers(n int) int {
    if n < 1 {
        return DefaultWorkers
    }
    if n > MaxWorkers {
        return MaxWorkers
    }
    return n
}
```

- [ ] **Step 15: Create `adt/custexport/config_test.go`**

```go
package custexport_test

import (
    "testing"
    "github.com/Hochfrequenz/mcp-server-abap/adt/custexport"
)

func TestParseTableList(t *testing.T) {
    tests := []struct {
        input string
        want  []string
    }{
        {"", nil},
        {"T001", []string{"T001"}},
        {"t001, t002 , T003", []string{"T001", "T002", "T003"}},
        {" , , ", nil},
    }
    for _, tt := range tests {
        got := custexport.ParseTableList(tt.input)
        // compare got vs tt.want
    }
}

func TestClampWorkers(t *testing.T) {
    tests := []struct{ input, want int }{
        {0, 20}, {-1, 20}, {1, 1}, {20, 20}, {40, 40}, {41, 40}, {100, 40},
    }
    for _, tt := range tests {
        if got := custexport.ClampWorkers(tt.input); got != tt.want {
            t.Errorf("ClampWorkers(%d) = %d, want %d", tt.input, got, tt.want)
        }
    }
}
```

- [ ] **Step 16: Update `tools/customizing.go`**

Replace inline table parsing (lines 60-69) with:
```go
tables := custexport.ParseTableList(tablesStr)
```

Replace worker capping (lines 71-74) with:
```go
workers = custexport.ClampWorkers(workers)
```

- [ ] **Step 17: Verify and commit**

```bash
go test ./adt/custexport/ -run "TestParseTableList|TestClampWorkers" -v
go test ./tools/ -v
go vet ./...
git add adt/custexport/config.go adt/custexport/config_test.go tools/customizing.go
git commit -m "refactor: move customizing helpers to adt/custexport/

Table name parsing and worker count capping are domain validation,
not MCP concerns."
```

---

## Task 5: Split `adt.Client` into focused interfaces (PR 5)

**Files:**
- Modify: `adt/client.go` (define role interfaces, redefine Client as composite)
- Modify: `tools/register.go` (narrow parameter types)
- Modify: all `tools/*.go` register functions (narrow `adt.Client` → specific interface)
- Modify: `adt/lockmap.go` (replace `LockClient`/`SourceReader` with `LockClient`/`SourceClient` from new interfaces)

- [ ] **Step 1: Define role interfaces in `adt/client.go`**

Add before the current `Client` interface definition (line 19). Insert the 8 role interfaces from the spec:

```go
// SourceClient reads and writes ABAP source code.
type SourceClient interface {
    GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
    SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error)
    PrettyPrint(ctx context.Context, source string) (string, error)
    GetCompletions(ctx context.Context, objectURI, source string, line, column int) ([]CompletionItem, error)
}

// ObjectClient manages ABAP object lifecycle.
type ObjectClient interface {
    CreateObject(ctx context.Context, objectType, name, packageName, description, transport string) error
    CreatePackage(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error
    DeleteObject(ctx context.Context, objectURI, lockHandle, transport string) error
    ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error)
}

// LockClient handles object locking.
type LockClient interface {
    LockObject(ctx context.Context, objectURI string) (string, error)
    UnlockObject(ctx context.Context, objectURI, lockHandle string) error
}

// SearchClient provides object discovery.
type SearchClient interface {
    SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error)
    WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error)
    BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error)
    GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error)
}

// QualityClient runs checks and tests.
type QualityClient interface {
    SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error)
    RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error)
    RunATCCheck(ctx context.Context, objectURIs []string, checkVariant string) (*ATCResult, error)
    GetATCCustomizing(ctx context.Context) (*ATCCustomizingResult, error)
}

// TransportClient manages CTS transports.
type TransportClient interface {
    CheckTransport(ctx context.Context, pgmID, object, objectName string) (*TransportCheckResult, error)
    CreateTransport(ctx context.Context, category, target, description, devClass string) (string, error)
    ReleaseTransport(ctx context.Context, transportNumber string) error
    GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error)
    AddToTransport(ctx context.Context, objectURI, transport string) error
}

// ExportClient handles package exports.
type ExportClient interface {
    ExportPackage(ctx context.Context, packageName string) ([]byte, error)
}

// QueryClient runs data queries.
type QueryClient interface {
    RunQuery(ctx context.Context, sql string, maxRows int) (*QueryResult, error)
}

// SystemClient provides system metadata.
type SystemClient interface {
    SystemInfo() (host, client string)
}
```

- [ ] **Step 2: Replace the `Client` interface with a composite**

Replace the existing 26-method interface (lines 19-47) with:

```go
// Client is the full ADT client combining all capabilities.
type Client interface {
    SourceClient
    ObjectClient
    LockClient
    SearchClient
    QualityClient
    TransportClient
    ExportClient
    QueryClient
    SystemClient
}
```

- [ ] **Step 3: Update `adt/lockmap.go`**

Remove the local `LockClient` and `SourceReader` interfaces added in Task 4. They are now replaced by the proper `adt.LockClient` and `adt.SourceClient` interfaces. Update `ResolveLock` and `ResolveETag` signatures:

```go
func (m *LockMap) ResolveLock(ctx context.Context, locker LockClient, ...) (string, error)
func (m *LockMap) ResolveETag(ctx context.Context, reader SourceClient, ...) (string, error)
```

Note: `SourceClient` includes `GetSource` so `SourceReader` is no longer needed.

- [ ] **Step 4: Verify compilation and tests**

```bash
go build ./...
go test ./...
```

Expected: All pass — the composite `Client` is structurally identical to the old one.

- [ ] **Step 5: Narrow tool registration signatures**

Update each `register*Tools` function in `tools/` to accept the minimal interface:

| Function | File | Old param | New param |
|----------|------|-----------|-----------|
| `registerSourceTools` | `tools/source.go` | `adt.Client` | `adt.SourceClient` |
| `registerActivateTools` | `tools/activate.go` | `adt.Client` | `adt.ObjectClient` |
| `registerSearchTools` | `tools/search.go` | `adt.Client` | `adt.SearchClient` |
| `registerRepositoryTools` | `tools/repository.go` | `adt.Client` | `adt.SearchClient` |
| `registerSyntaxCheckTools` | `tools/syntaxcheck.go` | `adt.Client` | `adt.QualityClient` |
| `registerUnitTestTools` | `tools/unittest.go` | `adt.Client` | `adt.QualityClient` |
| `registerTransportTools` | `tools/transport.go` | `adt.Client` | `adt.TransportClient` |
| `registerLockTools` | `tools/lock.go` | `adt.Client` | `adt.LockClient` |
| `registerPatchTools` | `tools/patch.go` | `adt.Client` | needs `SourceClient` + `LockClient` |
| `registerPrettyPrinterTools` | `tools/prettyprinter.go` | `adt.Client` | `adt.SourceClient` |
| `registerObjectTools` | `tools/object.go` | `adt.Client` | `adt.ObjectClient` |
| `registerCompletionTools` | `tools/completion.go` | `adt.Client` | `adt.SourceClient` |
| `registerSystemTools` | `tools/system.go` | uses `SystemSelector` | no change |
| `registerATCTools` | `tools/atc.go` | `adt.Client` | `adt.QualityClient` |
| `registerFileSourceTools` | `tools/file_source.go` | `adt.Client` | needs `SourceClient` + `LockClient` |
| `registerDebuggerTools` | `tools/debugger.go` | `adt.Client` | `adt.Client` (uses `adt.NewDebugSession`) |
| `registerExportTools` | `tools/export.go` | `adt.Client` | needs `ExportClient` + `SearchClient` |
| `registerCustomizingTools` | `tools/customizing.go` | `adt.Client` | `adt.Client` (custexport.RunExport takes full Client) |

For functions that need multiple interfaces, use inline composition:
```go
func registerPatchTools(s toolAdder, client interface {
    adt.SourceClient
    adt.LockClient
}, lockMap *adt.LockMap, selector SystemSelector) {
```

`RegisterAllWithLockMap` still accepts `adt.Client` and passes it to all — Go structural typing handles the widening.

- [ ] **Step 6: Verify**

```bash
go build ./...
go test ./...
go vet ./...
```

Expected: All pass — no runtime behavior change, only type narrowing.

- [ ] **Step 7: Commit**

```bash
git add adt/client.go adt/lockmap.go tools/source.go tools/activate.go tools/search.go tools/repository.go tools/syntaxcheck.go tools/unittest.go tools/transport.go tools/lock.go tools/patch.go tools/prettyprinter.go tools/object.go tools/completion.go tools/atc.go tools/file_source.go tools/debugger.go tools/export.go tools/customizing.go
git commit -m "refactor: split adt.Client into 8 focused role interfaces

SourceClient, ObjectClient, LockClient, SearchClient, QualityClient,
TransportClient, ExportClient, QueryClient, SystemClient. The composite
Client interface is preserved for consumers who need everything.

Each tool registration function now accepts the minimal interface it
needs, making dependencies explicit and mocking easier."
```

---

## Final Verification

After all 5 PRs are merged:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run
```

The resulting package structure should be:

```
main.go
  ├─ config/
  ├─ logging/
  ├─ auth/
  ├─ cmd/
  ├─ adt/              ← clean library, no MCP dependencies
  │   ├─ adtxml/       ← internal XML types
  │   └─ custexport/   ← customizing export
  └─ tools/            ← thin MCP wrappers only
```
