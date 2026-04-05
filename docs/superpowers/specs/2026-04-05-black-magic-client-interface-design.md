# BlackMagicClient Interface Design

**Date:** 2026-04-05
**Status:** Approved
**Issue:** #225

## Purpose

Define a minimal interface in the ADT MCP server for fallback operations where
ADT REST endpoints are not available. The interface is implementation-agnostic —
how the fallback works is not the concern of the MCP server.

## Interface

```go
// BlackMagicClient provides fallback operations for cases where ADT REST
// endpoints are not available. Injected separately from the main Client.
// Pass nil where no fallback is available.
type BlackMagicClient interface {
    ReleaseTransportFallback(ctx context.Context, transportNumber string) error
}
```

## Integration

`BlackMagicClient` is a separate, optional interface — not part of the composite
`Client`. It is injected into tool handlers that need fallback behavior.

The tool handler tries REST first (`TransportClient.ReleaseTransport`). If that
fails, it tries `BlackMagicClient.ReleaseTransportFallback` before giving up.

## Implementation

Provided by a private module. Not part of this repository.
