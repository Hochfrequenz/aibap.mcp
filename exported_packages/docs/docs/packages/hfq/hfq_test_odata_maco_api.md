# Package: HFQ / TEST_ODATA_MACO_API

**Description**: Test zum Aufrufen der MaKo-Cloud-API (Test for calling the MaKo Cloud API)
**Original language**: German (D)
**Number of objects**: 2 (1 report, 1 package definition)

## Executive Summary

This is a minimal one-file test package containing a single ABAP report that makes an HTTP GET request to the MaKo (Marktkommunikation / market communication) Cloud API via an RFC destination and prints the HTTP response code and reason phrase. It has no production logic and serves as a connectivity or proof-of-concept test. *Inferred from the package name prefix `TEST_` and the trivial implementation.*

---

## Reports

### `/HFQ/TEST_MACO_API`

**Description**: Standalone test report for the MaKo Cloud (GISA) API.

**Behavior (verified from implementation):**

1. Creates an HTTP client via `CL_HTTP_CLIENT=>CREATE_BY_DESTINATION` using the RFC destination `'MACO_CLOUD_GISA'`. If destination is not found, exception handling is by EXCEPTIONS (no explicit error handling code — the program continues silently on failure since no `IF SY-SUBRC` check follows).
2. Sets the HTTP method to `GET`.
3. Sets the request URI to `'/MessagePayloads'` (OData entity set for message payloads).
4. Sends the request and receives the response.
5. Reads the response HTTP status code (`LV_RESP_CODE`) and reason string (`LV_RESPONSE_REASON`).
6. Writes both to the screen: `|{ lv_resp_code }, { lv_response_reason }|`.

**Dependencies:**
- RFC destination `MACO_CLOUD_GISA` must be configured in the target SAP system (SM59).
- The MaKo Cloud API endpoint `/MessagePayloads` is expected to be reachable at that destination.

**No selection screen, no parameters, no error handling beyond exception declarations.**
