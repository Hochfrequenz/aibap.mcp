Closes #

## Changes

<!-- What does this PR do? One or two sentences. -->

## Checklist

- [ ] `go test ./...` passes
- [ ] `gofmt -w .` applied
- [ ] `go vet ./...` clean
- [ ] `make lint` clean
- [ ] Integration tests run (if touching SAP-facing code)
- [ ] New tool handlers use `NewToolResultJSON` + `WithOutputSchema` (no stringly-typed returns)
