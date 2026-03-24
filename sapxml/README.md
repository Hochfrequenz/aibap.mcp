# sapxml

SAP ADT XML types and helpers. This package is designed to be extractable to a separate repository once stable.

## Rules

1. **Only verified types.** Every model must have been verified against a real SAP system. No hypothesized structures.
2. **Document verification.** Each type documents which endpoint it maps to, the verification date, and the SAP system used.
3. **No client logic.** This package contains only data types and XML helpers. No HTTP calls, no authentication, no business logic.

## Contents

- `asxxml.go` — Generic `UnmarshalASXData[T]` / `MarshalASXData[T]` for the `<asx:abap>` envelope format
- `models.go` — Verified SAP ADT response/request model types
