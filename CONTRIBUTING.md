# Contributing to mcp-server-abap

## Before you start

- Only pick up **unassigned** issues. Assign yourself before writing any code.
- One PR per issue. Don't bundle unrelated changes.
- Always use feature branches (`feat/`, `fix/`, `test/`, `refactor/`, `docs/`, `chore/`). Never commit directly to `main`.

## Development setup

```bash
go build -o mcp-server-abap .
go test ./...
gofmt -w .
go vet ./...
make lint   # runs golangci-lint with dupl/goconst/gocyclo
```

All four must pass before committing.

## Tests

- Unit tests: `go test ./...` — always pass before committing.
- Integration tests require VPN access to our SAP systems and the `Z_ADT_MCP_TEST` package. They are gated behind `//go:build integration` and not required for community contributions.
- Coverage thresholds are enforced per package in CI (see `CLAUDE.md` for details).

## Adding a new tool

See the "Adding a New Tool" section in `CLAUDE.md` for the full checklist, including mandatory structured output rules.

## When a fix needs adtler

Most SAP-touching fixes belong in [adtler](https://github.com/Hochfrequenz/adtler), not here. If you find that a fix requires an adtler change:

1. Open or link an issue there.
2. Label the issue here `blocked-by-adtler`.
3. Add a copy-pastable reproducer (tool call + arguments JSON, target system, expected vs. observed output) to the issue body.
4. Add the issue to the open "Next adtler release" tracking issue.

## Pull requests

- Link to the issue in the PR description (`Closes #N`).
- Run `gofmt`, `go vet ./...`, and `go test ./...` before pushing.
- CI must be green before requesting review.

## Code style

- Follow the patterns in `CLAUDE.md`. In particular: no stringly-typed tool results, structured output for all handlers, no silent parameter defaults for required params.
