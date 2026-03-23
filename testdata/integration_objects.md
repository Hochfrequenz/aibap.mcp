# Integration Test Fixtures

Objects that must exist on the SAP test system for integration tests to pass.

## SAP System

- Host: configured via `SAP_INTEGRATION_HOST`
- Client: configured via `SAP_INTEGRATION_CLIENT`

## Required Objects

See GitHub issue #32 for the full list of required test objects.

| Object | URI | Used By | Notes |
|--------|-----|---------|-------|
| ZMCP_TEST (package) | `/sap/bc/adt/packages/ZMCP_TEST` | All tests | Container package |
| ZMCP_TEST_REPORT | `/sap/bc/adt/programs/programs/ZMCP_TEST_REPORT` | Source, lock, activate tests | Editable report |
| ZCL_MCP_TEST_WITH_UNITS | `/sap/bc/adt/oo/classes/ZCL_MCP_TEST_WITH_UNITS` | Unit test tests | Has passing + failing tests |
| ZCL_MCP_TEST_NO_UNITS | `/sap/bc/adt/oo/classes/ZCL_MCP_TEST_NO_UNITS` | GetObjectInfo tests | No unit tests |
| ZIF_MCP_TEST | `/sap/bc/adt/oo/interfaces/ZIF_MCP_TEST` | Where-used tests | Implemented by ZCL_MCP_TEST_WITH_UNITS |
| ZMCP_TEST_SYNTAX_WARN | `/sap/bc/adt/programs/programs/ZMCP_TEST_SYNTAX_WARN` | Syntax check tests | Has intentional warnings |

## Running Integration Tests

```bash
export SAP_INTEGRATION_HOST=https://srvhfuhana.sap.msp.local:44300
export SAP_INTEGRATION_USER=...
export SAP_INTEGRATION_PASSWORD=...
export SAP_INTEGRATION_CLIENT=100
make integration-test
```
