# Integration Test Fixtures

Objects that must exist on the SAP test system for integration tests to pass.

## Important: Local Only

**Integration tests do NOT run in GitHub Actions CI.** There is no SAP system reachable from
GitHub-hosted runners. They run locally only, gated by the `SAP_INTEGRATION_HOST` env var.
The `//go:build integration` tag ensures `go test ./...` in CI never includes them.

## SAP System

- Host: configured via `SAP_INTEGRATION_HOST`
- Client: configured via `SAP_INTEGRATION_CLIENT`

## Required Objects

See GitHub issue #32 for the full list of required test objects.

| Object | URI | Used By | Notes |
|--------|-----|---------|-------|
| Z_ADT_MCP_TEST (package) | `/sap/bc/adt/packages/Z_ADT_MCP_TEST` | All tests | Container package |
| Z_ADT_MCP_TEST_REPORT | `/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT` | Source, lock, activate tests | Editable report |
| ZCL_ADT_MCP_TEST_UNITS | `/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_UNITS` | Unit test tests | Has passing + failing tests |
| ZCL_ADT_MCP_TEST_NOUNITS | `/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_NOUNITS` | GetObjectInfo tests | No unit tests |
| ZIF_ADT_MCP_TEST | `/sap/bc/adt/oo/interfaces/ZIF_ADT_MCP_TEST` | Where-used tests | Implemented by ZCL_ADT_MCP_TEST_UNITS |
| Z_ADT_MCP_TEST_SYNWARN | `/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_SYNWARN` | Syntax check tests | Has intentional warnings |

## Running Integration Tests

**Locally only** (requires VPN/network access to the SAP system):

```bash
export SAP_INTEGRATION_HOST=https://srvhfuhana.sap.msp.local:44300
export SAP_INTEGRATION_USER=...
export SAP_INTEGRATION_PASSWORD=...
export SAP_INTEGRATION_CLIENT=100
make integration-test
```

These tests are **never** run in CI — the build tag `integration` is not passed by any GitHub Actions workflow.
