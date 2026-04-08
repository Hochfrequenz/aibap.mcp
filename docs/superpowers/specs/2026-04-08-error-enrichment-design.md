# Error Enrichment Design

**Issue:** #217
**Date:** 2026-04-08

## Problem

SAP ADT errors are passed through as-is. Raw errors like `ENQUEUE_FOREIGN_LOCK` or HTTP 500 are cryptic — the LLM often can't interpret them and either gives up or hallucinates a fix.

## Design

### Approach

Enrich errors centrally in `errorResult()` by appending actionable hints. The original error message is preserved; the hint is appended as `\n\nHint: ...`.

### Changes

1. **New file `tools/errors.go`** — move `errorResult()` from `source.go` here, add hint matching logic.
2. **Hint table** — pattern-match on `adt.ADTError.StatusCode` and error text substrings. First match wins. No match → error passes through unchanged.
3. **Existing inline hints stay** — the context-specific hints in `object.go` (DDIC fallback), `transport.go` (Category W), and `customizing_write.go` (SM30) catch errors before they reach `errorResult()` and remain untouched.

### Hint Table

| StatusCode | Text Pattern | Hint |
|------------|-------------|------|
| 423 | (any) | "Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport." |
| 404 | (any) | "Object not found. Check the URI spelling or use `search_objects` to find it." |
| 403 | (any) | "Authorization error. Check that the ADT user has the required S_DEVELOP authorizations." |
| 400 | "transport" | "A transport request may be required. Use `create_transport` or `get_transport_requests` to find one." |
| 409 | (any) | "Object already exists. Use `search_objects` to find it, or choose a different name." |
| 0 | "already exists" | "Object already exists. Use `search_objects` to find it, or choose a different name." |
| 500 | (any) | "SAP server error. Retry once — if it persists, check SM21 (system log) or ST22 (short dumps)." |
| 0 | "inactive" | "Activation failed — dependent objects may be inactive. Use `activate_objects` with all dependencies." |

StatusCode 0 means "any status code, match on text only".

### Output Format

```
Error: SAP ADT error 423: User SMITH is currently editing Z_REPORT

Hint: Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport.
```

### Implementation

- `errorResult(err)` checks if `err` is an `*adt.ADTError` (via `errors.As`) for status code matching.
- Falls back to string matching on `err.Error()` for non-ADTError types.
- Hints are a `[]hintRule` slice checked in order; first match wins.
- Each rule has: optional `statusCode int`, optional `textPattern string`, required `hint string`.

### What doesn't change

- Context-specific error handling in object.go, transport.go, customizing_write.go
- Middleware logging
- No new dependencies
