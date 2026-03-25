# Package: HFQ / KOMDATA_CODE11

**Description**: Versand Partin Nachricht ohne Inhalt (Sending PARTIN message without content)
**Original language**: German (D)
**Number of objects**: 3 (1 class, 1 report, 1 package definition)

## Executive Summary

This package implements the dispatch of a PARTIN message with code 11 — the German energy market mechanism for deactivating a communication sheet by sending it without content. The report `/HFQ/RP_CODE11_SND` (created April 2025 by Hochfrequenz) validates that the selected service provider belongs to the user's own system, checks that no code-11 has already been sent prior to the requested validity date, truncates the predecessor version in `/HFQ/PTCOMV`, creates a new version entry, and triggers the standard PARTIN mass-send process (`/HFQ/TRIG_PARTIN_MAS`) with the code-11 flag set. The helper class `/HFQ/CL_CODE11` encapsulates the database manipulation of `/HFQ/PTCOMV`.

---

## Classes

### `/HFQ/CL_CODE11`

**Modifiers**: `PUBLIC FINAL CREATE PUBLIC`

**Class-level attributes:**
- `GV_MTEXT TYPE STRING` (protected class data — message text buffer)

**Public class methods:**

#### `UPDATE_PTCOMV`

```
CLASS-METHODS UPDATE_PTCOMV
  IMPORTING
    IV_SERVICEID          TYPE /HFQ/PTCOMV-SERVICEID
    IV_SERVICEID_RECEIVER TYPE /HFQ/PTCOMV-SERVICEID_RECEIVER
    IV_VALIDITY_DATE      TYPE /HFQ/DE_KD_VON
  EXPORTING
    ES_PTCOMV_NEW         TYPE /HFQ/PTCOMV
    EV_CODE11_PRE_VALID_SENT TYPE BOOLEAN
    EV_VALIDITY_DATE_OLD  TYPE /HFQ/DE_KD_BIS
  RAISING
    /HFQ/CX_PARTIN_ERROR
```

**Behavior (verified from implementation):**
1. Selects `MAX(VERSION)` from `/HFQ/PTCOMV` where `SERVICEID = IV_SERVICEID` and `SERVICEID_RECEIVER = IV_SERVICEID_RECEIVER` and `STATUS = GC_VSTATUS_SENT`.
2. Loads that sent version via `/HFQ/CL_PARTIN_DB=>SELECT_PTCOMV`.
3. **Guard check**: If the existing sent version's `BIS` (validity end) is already less than `IV_VALIDITY_DATE`, a code-11 was previously sent before this date. Sets `EV_CODE11_PRE_VALID_SENT = ABAP_TRUE`, exports `EV_VALIDITY_DATE_OLD = ls_ptcomv_old-bis`, and returns immediately without making changes.
4. Otherwise, updates the existing (last-sent) record: sets `BIS = IV_VALIDITY_DATE - 1` (truncates by one day), updates `AEDAT`/`AENAM`.
5. Creates a new `/HFQ/PTCOMV` entry: `VERSION = old_version + 1`, `PREV_VERSION = old_version`, `VON = IV_VALIDITY_DATE`, `BIS = IV_VALIDITY_DATE`, sets `ERDAT`/`ERNAM`/`AEDAT`/`AENAM`.
6. Writes both records via `/HFQ/CL_PARTIN_DB=>UPDATE_PTCOMV`.
7. Exports `ES_PTCOMV_NEW` with the newly created entry.

---

## Reports

### `/HFQ/RP_CODE11_SND`

**Created**: Hochfrequenz Unternehmensberatung GmbH, April 2025

**Selection screen:**

| Parameter | Type | Description |
|---|---|---|
| `P_SRVCID` | `/HFQ/PTCOMV-SERVICEID` | Service provider ID |
| `P_DATUM` | `DATS` | Validity date for the code-11 deactivation |

**Behavior (verified from implementation):**

1. **Own-system check**: Calls `/HFQ/CL_PARTIN_HELPER=>CHECK_IF_OWN_SERVICE(IV_SERVICE_ID = P_SRVCID)`. If the result is `ABAP_FALSE`, issues info message `I056(/HFQ/MSG_KOMDATA)` ("Daten des fremden Serviceanbieters &1 dürfen nicht verschickt werden") and returns.

2. **Database update**: Calls `/HFQ/CL_CODE11=>UPDATE_PTCOMV` with `IV_SERVICEID = P_SRVCID`, `IV_SERVICEID_RECEIVER = ''` (empty = mass send), `IV_VALIDITY_DATE = P_DATUM`.

3. **Pre-validity guard**: If `EV_CODE11_PRE_VALID_SENT = ABAP_TRUE`, issues error message `E117(/HFQ/MSG_KOMDATA)` ("Versand nach dem &1 nicht mehr zulässig") with `GV_VALIDITY_DATE_OLD` and returns.

4. **Process trigger**: Calls function module `/HFQ/TRIG_PARTIN_MAS` with:
   - `IV_VERSION = GS_PTCOMV_NEW-VERSION`
   - `IV_SERVICE_SEN = P_SRVCID`
   - `IV_CODE11_STATUS = ABAP_TRUE`

5. **Status update**: Sets `GS_PTCOMV_NEW-STATUS = GC_VSTATUS_SENT` and persists via `/HFQ/CL_PARTIN_DB=>UPDATE_PTCOMV`.

6. **Success popup**: Displays `POPUP_TO_DISPLAY_TEXT` with "Versand angestoßen und Version abgegrenzt" (send triggered and version delimited).

7. **Error handling**: On `/HFQ/CX_PARTIN_ERROR`, issues `E070(/HFQ/MSG_KOMDATA)` ("Versenden fehlgeschlagen").
