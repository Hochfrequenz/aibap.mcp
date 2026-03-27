# ADT Client / MCP Layer Separation

**Date:** 2026-03-27
**Goal:** Clean internal separation of the ADT client library from the MCP tool layer, so that a future repo split is trivial.

## Status Quo

The codebase has 6 internal packages:

```
main.go
  ├─ config/      (YAML config)
  ├─ logging/     (slog setup)
  ├─ auth/        (OAuth2 tokens)
  ├─ cmd/         (login CLI)
  ├─ adtmodel/    (XML marshalling types)
  ├─ adt/         (HTTP client + SAP operations)
  │   └─ custexport/
  └─ tools/       (MCP tool handlers)
```

**What's already clean:**
- `tools/` imports `adt.Client` as interface — no concrete type access
- `adt/` never imports `tools/`
- No circular dependencies

**What's not clean:**

1. **`adtmodel/` is a top-level package** but is exclusively an implementation detail of `adt/`. It's never imported by anything outside `adt/`. Making it top-level exposes internal XML types to library consumers.

2. **Business logic lives in `tools/`** instead of `adt/`:
   - `tools/patch.go`: `ApplyPatchOps()` + 6 helper functions (line-based patch engine with overlap detection, search-and-replace)
   - `tools/export.go`: `extractZIPToDir()` (with Zip Slip protection), `writeExport()`, `matchesAnyPattern()`, `parsePatternList()`
   - `tools/file_source.go` + `tools/patch.go`: duplicated lock+ETag orchestration pattern (auto-lock → ETag lookup → fallback to GetSource)
   - `tools/customizing.go`: table name parsing, worker count validation

3. **`adt.Client` is a 26-method monolith.** Every consumer must implement all 26 methods even if they only need source operations. This makes the interface hard to mock and hard to consume as a library.

## Design

### Phase 1: Move `adtmodel/` under `adt/`

**Rename** `adtmodel/` → `adt/adtxml/` (internal sub-package).

Why `adtxml` not `internal/adtxml`: Go's `internal` convention prevents import from outside the parent module. Since we're in a single module today and want a future split where `adt/` becomes its own module, `internal` would lock out the MCP layer. But `adtxml` is unexported by convention — no `adt.Client` method signature references `adtxml` types, so consumers never need it. When we later split repos, we can decide whether to make it `internal` in the ADT module.

**Scope:**
- Create `adt/adtxml/` with all files from `adtmodel/`
- Update package declaration in all moved files
- Update imports in 12 `adt/*.go` files + `adt/query_test.go`
- Delete `adtmodel/`
- No functional changes, no API changes

**Validation:** All existing tests pass. `tools/` has zero `adtmodel` imports — nothing breaks.

### Phase 2: Move business logic from `tools/` to `adt/`

Move domain logic out of the MCP layer. The `tools/` package should contain only: MCP parameter parsing, JSON marshaling, MCP response building, and thin delegation to `adt`.

#### 2a: Patch engine → `adt/patch.go`

Move from `tools/patch.go`:
- `ApplyPatchOps(source string, ops []PatchOp) (string, error)`
- `PatchOp` type (currently in `tools/`)
- Helper functions: `lineDelta`, `primaryKey`, `opStartLine`, `opEndLine`, `splitLines`, `joinLines`, `applyLineOp`

Move `tools/patch_test.go` unit tests → `adt/patch_test.go`.

`tools/patch.go` keeps only `registerPatchTools()` which parses MCP params, calls `adt.ApplyPatchOps()`, and returns MCP results.

#### 2b: Export utilities → `adt/export.go` (extend existing)

Move from `tools/export.go`:
- `extractZIPToDir(zipData []byte, dir string) error`
- `writeExport(data []byte, outputDir, packageName string, asFolder bool) (string, int, error)`
- `matchesAnyPattern(name string, patterns []string) bool`
- `parsePatternList(raw string) ([]string, error)`

Move corresponding tests from `tools/export_test.go` → `adt/export_test.go`.

`tools/export.go` keeps only `registerExportTools()`.

#### 2c: Lock+ETag orchestration

The pattern "resolve lock handle → get ETag → do operation" appears in both `tools/patch.go` and `tools/file_source.go`. Extract to `adt/lockmap.go` (extend existing).

Also move `lockKey(selector SystemSelector, objectURI string) string` from `tools/register.go` — it's domain logic (key derivation for lock state), not an MCP concern.

New methods on `LockMap` use narrow interfaces (not the full `Client`) to preserve the principle of least privilege:

```go
// ResolveLock returns the lock handle for the given key.
// If no lock exists in the map and autoLock is true, it acquires one via locker.LockObject().
func (m *LockMap) ResolveLock(ctx context.Context, locker LockClient, key, objectURI, explicitHandle string) (string, error)

// ResolveETag returns the ETag for the given key.
// If not cached, falls back to reader.GetSource() to fetch it.
func (m *LockMap) ResolveETag(ctx context.Context, reader SourceClient, key, objectURI string) (string, error)
```

**Note:** This adds a dependency from `LockMap` to `LockClient`/`SourceClient`. Currently `LockMap` is a pure data structure. This is a deliberate trade-off to eliminate duplicated orchestration logic. The narrow interfaces keep the coupling minimal.

This eliminates the duplicated 15-line lock resolution blocks in `tools/patch.go` and `tools/file_source.go`.

#### 2d: Customizing config helpers

Move from `tools/customizing.go`:
- Table name parsing (split comma-separated, trim, uppercase) → `adt/custexport/config.go`
- Worker count capping → `adt/custexport/config.go`

These are small but they're domain validation, not MCP concerns.

### Phase 3: Split `adt.Client` into focused interfaces

Replace the single 26-method interface with composable role interfaces. The concrete `httpClient` and `ClientRegistry` still implement all of them.

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

// Client is the full ADT client combining all capabilities.
// Kept for convenience — consumers who need everything can use this.
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

**Changes to `tools/`:** Each `register*Tools()` function narrows its parameter type from `adt.Client` to the minimal interface it needs. For example:
- `registerSourceTools(s toolAdder, client adt.SourceClient, ...)`
- `registerSearchTools(s toolAdder, client adt.SearchClient)`
- `registerPatchTools(s toolAdder, client interface{ adt.SourceClient; adt.LockClient }, ...)`

`RegisterAll()` still accepts `adt.Client` (the full composite) and passes it to each register function — Go's structural typing handles this automatically.

**Changes to tests:** Mock structs in `tools/source_test.go` can implement only the interfaces they need instead of all 26 methods. This is optional — the existing full mock still works.

### Phase 4: Prepare for repo split (optional, deferred)

Not in scope now, but the previous phases make this trivial later:
- Move `adt/`, `adt/adtxml/`, `adt/custexport/`, `config/`, `auth/` into a new `go-sap-adt` module
- The MCP server imports it as `github.com/Hochfrequenz/go-sap-adt`
- `logging/` and `cmd/` stay in the MCP server repo

## PR Plan

| PR | Phase | Scope | Risk |
|----|-------|-------|------|
| 1 | 1 | Move `adtmodel/` → `adt/adtxml/` | Low — pure rename, zero functional change |
| 2 | 2a | Move patch engine to `adt/` | Low — move + re-export |
| 3 | 2b | Move export utilities to `adt/` | Low — move + re-export |
| 4 | 2c+2d | Lock orchestration + customizing helpers | Medium — new API on LockMap |
| 5 | 3 | Split Client interface | Medium — many file touches, but Go structural typing means no runtime changes |

Each PR is independently mergeable. Order matters: PR 1 before 2-4, PR 2-4 before 5.

**Validation per PR:** `go test ./...`, `go vet ./...`, and `golangci-lint run` must all pass.

## Out of Scope

- Repo split (Phase 4) — deferred
- Debugger session management refactoring — the singleton pattern in `tools/debugger.go` is MCP-specific (shared session per server instance). Moving it to adt/ would couple adt/ to MCP lifecycle concerns.
- Renaming packages or changing public API signatures beyond the interface split
- Adding new functionality
