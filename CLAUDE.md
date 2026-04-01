# CLAUDE.md

## Testing

- **Test-driven**: Write unit tests before or alongside implementation, not after.
- **Unit tests**: `go test ./...` — must always pass before committing.
- **Integration tests**: `go test ./adt/ -tags integration` — run against a real SAP system. Require `SAP_INTEGRATION_*` env vars from `.env`.
- **Transport tests**: `go test ./adt/ -tags 'integration transport'` — create, release, and modify transports on SAP. **Only run when explicitly requested** — these leave artifacts on the system.
- **Never run transport tests automatically** as part of a general integration test run.

## Workflow

- One PR per issue. Don't bundle unrelated changes.
- Always use feature branches (`feat/`, `fix/`, `test/`, `refactor/`), never commit directly to `main`.
- Only pick up **unassigned** issues. Assign yourself before starting work.
- Run `gofmt` and `go vet ./...` before committing.

## SAP ADT

- Credentials live in `~/.config/sap-mcp/systems.json` (never commit, never put in plain text in commands).
- Config format: see [sap-mcp-config](https://github.com/Hochfrequenz/sap-mcp-config).
- Override config path via `SAP_CONFIG_FILE` env var.
- S4 systems require HTTPS (secure cookie flag breaks HTTP — see #108).
- ECC systems may not have all endpoints (e.g. `/sap/bc/adt/packages` is S4-only).
