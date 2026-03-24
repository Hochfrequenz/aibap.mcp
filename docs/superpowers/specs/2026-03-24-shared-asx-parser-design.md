# Shared asx:abap XML Parser

**Date:** 2026-03-24
**Issue:** #31

## Problem

Multiple SAP ADT endpoints use the `<asx:abap><asx:values><DATA>...</DATA></asx:values></asx:abap>` envelope format. Each handler currently defines its own wrapper structs and unmarshalling logic. A shared generic parser eliminates this duplication.

## Design

### File: `adt/asxxml.go`

Two generic functions:

```go
// UnmarshalASXData extracts <DATA> from an asx:abap envelope
// and unmarshals its content into T.
func UnmarshalASXData[T any](data []byte) (*T, error)

// MarshalASXData wraps the given struct inside the asx:abap envelope.
func MarshalASXData[T any](source T) ([]byte, error)
```

### Internal XML structs

```go
type asxEnvelope[T any] struct {
    XMLName xml.Name      `xml:"abap"`
    Values  asxValues[T]  `xml:"values"`
}

type asxValues[T any] struct {
    Data T `xml:"DATA"`
}
```

Go's `encoding/xml` ignores namespace prefixes, so `xml:"abap"` matches `<asx:abap>`.

### Marshal output format

```xml
<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <!-- marshalled T here -->
    </DATA>
  </asx:values>
</asx:abap>
```

For marshalling, the envelope struct uses explicit namespace attributes to produce valid SAP-compatible XML.

## Testing

### File: `adt/asxxml_test.go`

Test cases using real SAP response samples from issue #31:

1. **Unmarshal lock response** — extract `LOCK_HANDLE` from lock XML
2. **Unmarshal BrowsePackage response** — extract `TREE_CONTENT` with `SEU_ADT_REPOSITORY_OBJ_NODE` elements
3. **Unmarshal transport check response** — extract `REQUESTS` with nested `CTS_REQUEST` elements
4. **Marshal transport create request** — wrap `CATEGORY`, `TARGET`, `DESCRIPTION`, `DEVCLASS` in envelope
5. **Round-trip** — marshal then unmarshal, verify equality
6. **Empty DATA** — unmarshal envelope with empty `<DATA/>`, verify zero-value result
7. **Invalid XML** — verify error returned

## Scope

- Add `adt/asxxml.go` and `adt/asxxml_test.go`
- No existing callers are changed (refactor is a follow-up)
