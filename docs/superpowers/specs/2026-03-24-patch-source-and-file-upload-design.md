# Design: patch_source, set_source_from_file, and Lock-Map

**Date:** 2026-03-24
**Status:** Draft
**Context:** In a typical refactoring session, Claude needs to edit multiple ABAP objects. Passing full source (900+ lines) via `set_source` is impractical. Two new tools solve this: `patch_source` for surgical edits and `set_source_from_file` for full replacements from disk.

## Problem

1. `set_source` requires the **entire source** as a string parameter — unusable for large programs
2. Lock handles and ETags must be manually threaded through every tool call — error-prone in multi-object sessions
3. `activate_object` only activates one object at a time — SAP supports batch activation

## Design

### 1. Server-Side Lock Map

The MCP server maintains an in-memory map of active locks:

```
map[string]lockState   // key: systemName + ":" + objectURI

type lockState struct {
    LockHandle string
    ETag       string
}
```

The key includes the system name from the active `ClientRegistry` entry to prevent cross-system lock handle collisions when using multi-system configurations.

**Lifecycle:**
- `lock_object` stores `{lockHandle, etag}` in the map after a successful lock
- `get_source` updates the `etag` in the map (if entry exists) from the response
- `patch_source`, `set_source_from_file`, `set_source` update the `etag` after a successful write
- `unlock_object` removes the entry from the map
- All tools that need a lock check the map first if no `lock_handle` parameter was provided

**Auto-lock:** If a mutating tool (`patch_source`, `set_source_from_file`, `set_source`) is called without a `lock_handle` parameter AND no entry exists in the lock map, the server automatically locks the object and stores the result. Transport is not needed at lock time — SAP only requires it at write time (`?corrNr=`). If auto-lock fails (e.g., object locked by another user), the tool returns an error immediately without attempting the write.

### 2. `patch_source` Tool

Applies one or more edit operations to an ABAP object's source without requiring the full source as input.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `object_uri` | string | yes | ADT object URI |
| `operations` | array | yes | List of edit operations (see below) |
| `transport` | string | no | Transport request for non-$TMP objects |
| `lock_handle` | string | no | Explicit lock handle (overrides lock map) |

**Operations:**

Each operation is an object with a `type` field and type-specific parameters:

```json
{"type": "insert", "after_line": 1, "content": "* New comment line"}
{"type": "replace", "from_line": 5, "to_line": 7, "content": "  NEW CODE."}
{"type": "delete", "from_line": 10, "to_line": 12}
{"type": "search_replace", "search": "old_var", "replace": "new_var", "all": true}
```

- `insert`: Insert `content` after `after_line` (0 = before first line)
- `replace`: Replace lines `from_line` through `to_line` (inclusive, 1-based) with `content`
- `delete`: Delete lines `from_line` through `to_line` (inclusive, 1-based)
- `search_replace`: Find `search` string and replace with `replace`. If `all` is true (default false), replace all occurrences. Operates on the full source text, not line-based.

**Operation ordering:** Line-based operations are sorted descending by their primary line number (`after_line` for insert, `from_line` for replace/delete) and applied bottom-to-top, so that earlier line numbers remain valid. `search_replace` operations run after all line-based operations. Overlapping line-based operations (e.g., delete 5-10 and replace 8-12) are rejected with an error.

**Flow:**

1. Look up lock map for `object_uri` (or use provided `lock_handle`)
2. If no lock: auto-lock, store in map
3. `GetSource` with current ETag
4. Apply operations to source text
5. `SetSource` with modified source, lock handle, transport, ETag
6. Update ETag in lock map
7. Return result with modified source excerpt (first/last few changed lines), new ETag, lock status

**Response:**

```json
{
  "success": true,
  "lines_changed": 3,
  "locked": true,
  "lock_handle": "...",
  "etag": "..."
}
```

### 3. `set_source_from_file` Tool

Reads source from a local file and uploads it to the SAP object.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `object_uri` | string | yes | ADT object URI |
| `file_path` | string | yes | Path to source file (absolute or relative to CWD) |
| `transport` | string | no | Transport request for non-$TMP objects |
| `lock_handle` | string | no | Explicit lock handle (overrides lock map) |

**Validation:** File must exist and be readable. Files are assumed UTF-8 encoded (matching SAP's `charset=utf-8`). No path traversal restriction — the MCP server runs locally with the user's permissions, same as any file-reading MCP tool.

**Flow:**

1. Read file from disk
2. Look up lock map (or use provided `lock_handle`)
3. If no lock: auto-lock, store in map
4. If no ETag in lock map: `GetSource` to obtain current ETag
5. `SetSource` with file content, lock handle, transport, ETag
6. Update ETag in lock map
7. Return success with line count, lock status

### 4. `activate_objects` Tool (Plural)

Replaces `activate_object` (singular). Activates one or more objects in a single SAP request.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `object_uris` | array of string | yes | One or more ADT object URIs |

**Backward compatibility:** The existing `activate_object` (singular, single `object_uri` string) remains as an alias that delegates to `activate_objects` with a single-element array.

### 5. Changes to Existing Tools

**`lock_object`** — unchanged API, but now stores `{lockHandle, etag: ""}` in the lock map. Returns the lock handle as before.

**`unlock_object`** — `lock_handle` parameter becomes **optional**. If omitted, looks up the lock map. Removes entry from map on success.

**`get_source`** — unchanged API, but updates ETag in lock map if entry exists.

**`set_source`** — `lock_handle` and `etag` become **optional**. If omitted, looks up the lock map. Updates ETag in map after success.

## Implementation Scope

### New files
- `adt/lockmap.go` — lock map type and methods (thread-safe with `sync.RWMutex`)
- `tools/patch.go` — `patch_source` tool registration and patch logic
- `tools/file_source.go` — `set_source_from_file` tool registration

### Modified files
- `adt/client.go` — `SetSource` return type changes from `error` to `(string, error)` where string is the new ETag from the response header. `ActivateObject` renamed to `ActivateObjects` accepting `[]string`. Old `ActivateObject` becomes a convenience wrapper.
- `adt/source.go` — `SetSource` captures and returns ETag from response
- `adt/activate.go` — accepts `[]string` URIs (already builds XML with slice internally)
- `adt/registry.go` — updated delegation signatures, exposes `ActiveSystemName() string` for lock map keys
- `tools/register.go` — register new tools, pass lock map to all tool registrations
- `tools/lock.go` — optional `lock_handle`, lock map integration
- `tools/source.go` — optional `lock_handle`/`etag`, lock map integration, `get_source` updates ETag in lock map at tool layer
- `tools/activation.go` — `activate_objects` with multi-URI support, keep `activate_object` alias

### Not in scope
- Persistent lock map (in-memory only, cleared on server restart — locks expire server-side in SAP anyway)
- Diff/unified patch format (too complex for ABAP, line-based operations are sufficient)
- Automatic unlock on server shutdown (SAP handles lock expiry)

## Example Session: Multi-Object Refactoring

```
Claude                              MCP Server                    SAP
  |                                    |                           |
  |-- lock_object(ZREPORT_A) -------->|-- POST ?_action=LOCK --->|
  |<-- handle: "abc123" --------------|<-- XML lock response -----|
  |                                   | [map: ZREPORT_A={abc123}] |
  |                                    |                           |
  |-- lock_object(ZREPORT_B) -------->|-- POST ?_action=LOCK --->|
  |<-- handle: "def456" --------------|<-- XML lock response -----|
  |                                   | [map: +ZREPORT_B={def456}]|
  |                                    |                           |
  |-- patch_source(ZREPORT_A,         |                           |
  |     [{search_replace:             |-- GET source/main ------>|
  |       "old_func"->"new_func",     |<-- source + ETag --------|
  |       all:true}],                 |-- PUT source/main ------>|
  |     transport:"HFQK902658")       |<-- 200 OK ---------------|
  |<-- {success, lines_changed: 5} ---|                           |
  |                                    |                           |
  |-- set_source_from_file(ZREPORT_B, |                           |
  |     "refactored_b.abap",         |-- read file               |
  |     transport:"HFQK902658")       |-- GET source (ETag) ---->|
  |                                   |-- PUT source/main ------>|
  |<-- {success, lines: 340} ---------|<-- 200 OK ---------------|
  |                                    |                           |
  |-- activate_objects(               |                           |
  |     [ZREPORT_A, ZREPORT_B]) ---->|-- POST activation ------>|
  |<-- {success, messages:[]} --------|<-- activation result -----|
  |                                    |                           |
  |-- unlock_object(ZREPORT_A) ------>|-- POST ?_action=UNLOCK ->|
  |<-- "Object unlocked" -------------|<-- 200 OK ---------------|
  |                                   | [map: -ZREPORT_A]         |
  |                                    |                           |
  |-- unlock_object(ZREPORT_B) ------>|-- POST ?_action=UNLOCK ->|
  |<-- "Object unlocked" -------------|<-- 200 OK ---------------|
  |                                   | [map: -ZREPORT_B]         |
```
