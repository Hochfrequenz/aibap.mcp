# Design: get_object_dependencies

## Context

Issue #340: Transport completeness checks require knowing what a given ABAP object
references (forward direction). `where_used` answers the reverse. This tool closes
the gap by querying `WBCROSSGT`, SAP's workbench cross-reference table.

## Goal

Add a new MCP tool `get_object_dependencies` as a counterpart to `where_used`.
The LLM agent must be able to use it independently without prior knowledge of
`WBCROSSGT` or SAP cross-reference internals.

## Tool Definition

**Name:** `get_object_dependencies`
**Group:** `objects` (registered in `registerSearchTools` in `tools/search.go`)

### Input Parameters

| Parameter    | Type   | Required | Description                                      |
|--------------|--------|----------|--------------------------------------------------|
| object_type  | string | yes      | ABAP object type, e.g. `CLAS`, `PROG`, `TABL`   |
| object_name  | string | yes      | Object name, e.g. `/HFQ/MY_CLASS`               |
| max_results  | number | no       | Maximum rows returned (default: 200)             |

`object_uri` is intentionally not used — `WBCROSSGT` operates on type+name, and
exposing them directly is more discoverable for the LLM than parsing an ADT URI.

### Output (JSON)

```json
{
  "object_type": "CLAS",
  "object_name": "/HFQ/MY_CLASS",
  "count": 2,
  "dependencies": [
    {"name": "/HFQ/OTHER_IFACE", "use_type": "USE"},
    {"name": "/HFQ/SOME_TABL",   "use_type": "USE"}
  ]
}
```

Empty `dependencies` with `count: 0` when the object has no entries in `WBCROSSGT`
(not an error — object may be new or not yet analyzed by the workbench).

## Implementation

### Location

`tools/search.go` — added as a second `s.AddTool` call inside `registerSearchTools`,
directly after `where_used`.

### SQL Query

```sql
SELECT refobjnm, refusetyp
FROM wbcrossgt
WHERE object = '<OBJECT_TYPE>' AND obj_name = '<OBJECT_NAME>'
ORDER BY refobjnm
```

Only `refobjnm` and `refusetyp` are queried — these are the columns confirmed by
the issue SQL. `refobjtyp` is not selected as it may not exist in all SAP releases.

Built via `fmt.Sprintf`. Inputs are sanitized by replacing `'` with `''` before
interpolation (standard ABAP SQL escaping).

### Client

Uses the existing `adt.QueryClient` parameter already passed to `registerSearchTools`
via `client.RunQuery(ctx, sql, maxRows)`. No new interface methods, no new ADT
endpoints, no changes to `register.go`.

### Result Parsing

`RunQuery` returns a generic result with `Columns []string` and `Rows [][]string`.
The handler maps columns by index and returns the structured JSON above.

### Error Handling

- SAP/ADT errors from `RunQuery` → returned as tool error result (existing `errorResult` helper)
- Empty result set → `count: 0`, `dependencies: []` — not an error

## Non-Goals

- Namespace filtering (caller can filter the returned list)
- Batch mode (single object per call; can be added later if needed)
- ADT usage-references endpoint (Option B from issue #340 — separate future work)
