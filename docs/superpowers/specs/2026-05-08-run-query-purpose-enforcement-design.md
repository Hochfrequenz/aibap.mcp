# Design: run_query SAP API Policy Enforcement via `purpose` Parameter

**Date:** 2026-05-08
**Status:** Approved

## Problem

`run_query` exposes a raw SQL SELECT interface against any SAP database table. The SAP API Policy restricts ADT API usage to development tooling only — querying business or application tables (VBAK, BKPF, KNA1, etc.) violates this scope. The current enforcement in PR #367 is documentation-only (tool description text, README, system prompt). A language model can drift into policy-violating queries without any runtime obstacle.

## Goal

Force Claude to declare an explicit, pre-approved intent before executing any query. Unknown or missing intent triggers human confirmation via the existing Elicitor pattern. Valid intent passes through without friction.

## Out of Scope

- Export tools (`export_package`, `export_packages`) — these operate on ADT package URIs and are inherently scoped to development objects.
- `get_table_fields` — lower risk; deferred.
- Table-name blocklists — not maintainable and insufficient (SAP has ~100k tables).

## Design

### 1. New `purpose` Parameter

`run_query` receives a new **required** string parameter `purpose` with a fixed enum:

| Value | Permitted use |
|---|---|
| `ddic_inspection` | DDIC metadata tables (DD01L, DD02L, DD03L, ...) |
| `customizing_review` | Customizing tables (T001, TVARVC, TDEVC, ...) |
| `transport_tracking` | Transport catalog tables (E070, E071, TADIR, ...) |
| `development_metadata` | Development object catalog (TRDIR, TADIR, PROGDIR, ...) |

The enum is declared in the JSON schema so MCP clients (and Claude) see the valid values at tool-listing time. No freetext is accepted.

### 2. Enforcement Logic

The handler in `tools/query.go` checks `purpose` before calling `client.RunQuery`:

1. **Missing or not in enum** → call `Elicitor.Elicit(ctx, ...)` with message:
   > `"run_query requires a valid purpose. Declare why this query is needed for development tooling (ddic_inspection / customizing_review / transport_tracking / development_metadata). If none applies, this query may violate the SAP API Policy."`
   User confirms → proceed. User declines → return `errorResult`.

2. **Valid value** → execute immediately, no extra round-trip.

3. **Elicitor is nil + missing/invalid purpose** → hard block via `errorResult`. No silent passthrough without an oversight mechanism.

### 3. Logging

`withLogging` in `middleware.go` logs `purpose` as an additional `slog.Attr`. No structural changes to middleware — just an additional field extracted from the request args, parallel to the existing `object_uri` extraction.

### 4. Tests

New cases in `tools/query_test.go`:

| Scenario | Expected |
|---|---|
| Missing `purpose` | Elicitor called; on decline → `IsError: true`, `RunQuery` not called |
| Invalid `purpose` (e.g. `"reporting"`) | Same as missing |
| Valid `purpose` | Elicitor not called, `RunQuery` called with SQL |
| `Elicitor = nil` + missing `purpose` | Hard block, no panic |

`tools/structured_content_shape_test.go`: add `purpose: "ddic_inspection"` to the synthesised minimum args for `run_query` (required field — test would fail without it).

## Trade-offs

**Why enum over freetext:** Freetext allows Claude to write "inspect VBAK for development purposes" and bypass intent detection. An enum makes the boundary unambiguous and self-documenting.

**Why Elicitor over hard block:** The Elicitor pattern already exists for destructive tools and preserves human-in-the-loop oversight without breaking legitimate workflows. A hard block would reject valid queries where Claude picks a slightly wrong intent label.

**Why not a table blocklist:** SAP has ~100k tables; a blocklist never achieves full coverage and creates false confidence. Intent declaration addresses the root cause (undeclared purpose) rather than the symptom (specific table names).
