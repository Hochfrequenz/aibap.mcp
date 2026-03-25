# Package: HFQ / KOMDATA_API

**Description**: Kommunikation der API Daten (Communication of API data)
**Original language**: German (D)
**Number of objects**: 20 (5 classes, 1 interface, 1 BAdI enhancement spot, 2 function groups, 1 report, 1 transaction, 2 transparent tables, 1 table maintenance object, 1 message class, 2 table types, 1 namespace object)

---

## Executive Summary

This package implements the outbound and inbound handling of API communication parameters (Kommunikationsparameter) exchanged between energy market participants via AS4/PARTIN messages in the German energy market (EDIFACT-based process framework `/IDXGC/`).

The domain problem: each energy service provider ("Serviceanbieter") publishes API endpoint credentials — a target URI, a certificate issuer, and a certificate subject — to its market partners. These are versioned and transmitted via the PARTIN (Party Information) message type using the existing `/IDXGC/` process framework. The package covers:

1. **Outbound (Versand)**: A user starts the transaction `/HFQ/KOMDATA_API`, sees an ALV list of versioned API credential records, and initiates either a mass send (Massenversand) to all eligible receivers or a single-receiver send (Einzelversand). The function module `/HFQ/TRIG_KOMDATA_API_MAS` triggers the PARTIN sending process.
2. **Inbound (Empfang)**: Incoming PARTIN messages carrying API data are received and stored. The class `/HFQ/CL_KOMDATA_API_SAVE` (inheriting from `/IDXGC/CL_PROCESS_STEP_DATA`) handles persistence after inbound process step execution. A BAdI (`/HFQ/BADI_KOMDATA_API_RCV`) provides customer extension points for consistency checks and custom update logic.
3. **Versioning**: All records in `/HFQ/KOMDATA_API` carry a version number. The currently active version is determined by `DETERMINE_VERSION`. When a new version arrives or is created, the predecessor's validity end date (BIS) is set to `new_start_date - 1` (time-slice delimitation).
4. **Customizing**: The table `/HFQ/KD_API_SND` controls which sender service-type / receiver service-type combinations are allowed to send API PARTIN messages. It is maintained via SM30-style table maintenance (function group `/HFQ/FG_KD_API`, transaction linked to view `/HFQ/KD_API_SNDS`).

---

## Classes

### `/HFQ/CL_KOMDATA_API_HELPER`

**Description**: Datenbereitstellungsklasse für Steuerbefehle (Data provision class for control commands)
**Visibility**: PUBLIC
**Instantiation**: PUBLIC (all methods are static class methods)

Central utility class. All methods are CLASS-METHODS (static). Acts as the database access layer for `/HFQ/KOMDATA_API`.

| Method | Signature summary | Purpose |
|--------|-------------------|---------|
| `DETERMINE_VERSION` | `IV_SERVICEID`, `IV_SID_EMPFANG`, optional `IV_DATE`, `IV_STATUS` → `RS_KOMDATA_API` | Reads all records for a service provider + receiver, optionally filters by validity date and status, sorts descending by version, returns the highest matching row. |
| `SELECT_KOMDATA_API` | `IV_SERVICEID`, `IV_SERVICEID_RECEIVER`, `IV_VERSION` → `RS_KOMDATA_API` | Selects a single row by full primary key. |
| `SELECT_KOMDATA_API_RANGES` | `IT_SERVICEID` (ranges), `IT_SID_EMPFANG` (ranges), optional `IV_DATE` → `RT_KOMDATA_API` | Multi-row read supporting ranges; optionally filters on validity date range (`VON <= date <= BIS`). |
| `UPDATE_KOMDATA_API` | `CT_KOMDATA_API` (CHANGING) raises `/HFQ/CX_PARTIN_ERROR` | `MODIFY /hfq/komdata_api FROM TABLE`. Raises exception on failure (message e072). |
| `DELETE_KOMDATA_API` | `IT_TO_DELETE` | Deletes rows; guards: only own service providers, only records with status READY (not yet sent). |
| `DELIMIT_TIMESLICES` | `IV_SERVICEID`, `IV_DATE`, `IV_SID_EMPFANG` | Finds the current SENT version and sets its BIS = `IV_DATE - 1`. Used on outbound send. |
| `DELIMIT_TIMESLICES_IMPORT` | `IV_SERVICEID`, `IV_DATE`, `IV_SID_EMPFANG` | Same logic but targets status RECEIVED. Used when a new inbound version is saved. |
| `API_SEND_USER_ERRORS` | `IT_SELECTED`, optional `IV_STATUS` → `EV_BENUTZERFEHLER` | Validates user selection before sending: exactly one row selected, must be own service provider, must be the current highest version. Sets `EV_BENUTZERFEHLER = abap_true` and issues an info message on any violation. |
| `OWN_SERVPROV` | → `ET_SERVICEID_FILTER` | Selects own service providers (`ESERVPROV~OWN_LOG_SYS = true`) that are also registered in `/HFQ/KD_API_SND` as senders (JOIN via `/HFQ/INTCODES`). |

Protected attribute: `GV_MTEXT TYPE STRING` (used to receive message texts before raising exceptions).

---

### `/HFQ/CL_KOMDATA_API_STEUERN`

**Description**: Steuerklasse für API-Datenhandling (Control class for API data handling)
**Visibility**: PUBLIC
**Instantiation**: PUBLIC (instance class with constructor)

ALV-based UI controller for transaction `/HFQ/KOMDATA_API`. Constructed with a service provider range and a validity date; loads matching records from `/HFQ/KOMDATA_API` into `GT_KOMDATA_API_ALV`. Builds a `CL_SALV_TABLE` display with four custom toolbar buttons.

| Method | Purpose |
|--------|---------|
| `CONSTRUCTOR( IT_SERVICEID, IV_DATE )` | Stores selection parameters; populates `GT_KOMDATA_API_ALV` via `SELECT_KOMDATA_API_RANGES`. |
| `ALV( )` | Builds the `CL_SALV_TABLE`, sets standard functions, optimizes columns, adds custom buttons: "Neue Version erstellen", "Massenversand", "Einzelversand", "Eintrag löschen". Registers `HANDLE_USER_COMMAND` as event handler. |
| `HANDLE_USER_COMMAND( E_SALV_FUNCTION )` | Dispatches on button name: |
| | `Delete_Eintrag` — calls `DELETE_KOMDATA_API`, refreshes ALV. |
| | `Neue_Version_erstellen` — opens `POPUP_GET_VALUES` dialog collecting Serviceanbieter, URI, Issuer, Subject, Von-date; validates own service provider, checks that the new start date is not before the current highest version's start date; writes new row with `version = current + 1`, `status = READY`, `BIS = 99991231`; refreshes ALV. |
| | `Massenversand` — validates selection via `API_SEND_USER_ERRORS`; checks status is not already SENT; calls `/HFQ/TRIG_KOMDATA_API_MAS` with empty receiver list (sends to all); delimits predecessor timeslice; updates status to SENT; refreshes ALV. |
| | `Einzelversand` — validates selection; requires status = SENT (mass send must have run first); shows popup to collect specific receivers; validates each receiver against `/HFQ/KD_API_SND` sender/receiver combinations; calls `/HFQ/TRIG_KOMDATA_API_MAS` with the filtered receiver list; refreshes ALV. |

Protected data: `GT_KOMDATA_API_ALV TYPE /HFQ/T_KD_KOMDATA_API`, `GO_ALV TYPE REF TO CL_SALV_TABLE`, `GT_SERVICEIDS TYPE ISU_RANGES_TAB`, `GV_GUELTIGKEITSDATUM TYPE DATS`.
Class constant: `GC_DEAKTIVIEREN TYPE SALV_DE_FUNCTION VALUE 'DEAK'`.

---

### `/HFQ/CL_KOMDATA_API_SAVE`

**Description**: Speichern der Daten des Steuerbefehls (Saving the data of the control command)
**Visibility**: PUBLIC
**Superclass**: `/IDXGC/CL_PROCESS_STEP_DATA`

Inbound persistence handler. Plugged into the `/IDXGC/` process framework as the "save" step of the inbound PARTIN process. Redefines the `PROCESS` method.

**`PROCESS` logic**:
1. Retrieves `ref_to_msg` entry with qualifier `AGK` (current version reference) from the process step data.
2. Fetches step 0010 data via the process document API to read `DOCUMENT_STATUS`.
3. If `DOCUMENT_STATUS = '11'` (deactivation indicator): calls `/HFQ/CL_PARTIN_DB=>DELIMIT_TIMESLICES` and returns — no new record is written.
4. Otherwise: extracts URI (`Z17`), issuer (`Z24`), subject (`Z23`) from `MSGCOMMENTS`; determines sender party function qualifier; builds an `/HFQ/KOMDATA_API` record with `STATUS = RECEIVED`, `BIS = 99991231`.
5. Calls `DELIMIT_TIMESLICES_IMPORT` to close the previous received version.
6. Calls `UPDATE_KOMDATA_API` to persist the new record.

---

### `/HFQ/CL_KOMDATA_API_RCV_CHK`

**Description**: Prüfmethoden des API PARTIN Empfang (Validation methods for API PARTIN receipt)
**Visibility**: PUBLIC
**Instantiation**: PUBLIC (all methods are static)

Thin wrapper that invokes the BAdI `/HFQ/BADI_KOMDATA_API_RCV` for two check scenarios used by the inbound process framework.

| Method | Invoked BAdI method | Fallback behaviour |
|--------|--------------------|--------------------|
| `CONSISTENT_DATA( IS_PROCESS_STEP_KEY, ET_CHECK_RESULT, CR_DATA, CR_DATA_LOG )` | `CONSISTENT_DATA` | If BAdI not implemented or utility error: appends `GC_CR_OK`. If result is still empty after BAdI call: sets `GC_CR_ERROR`. |
| `CUSTOM_BADI( IS_PROCESS_STEP_KEY, ET_CHECK_RESULT, CR_DATA, CR_DATA_LOG )` | `UPDATE_API` | If BAdI not implemented: appends `GC_CR_NOT_IMPLEMENTED`. If result empty: sets `GC_CR_ERROR`. |

Both methods use `GET BADI / CALL BADI` with `TRY/CATCH CX_BADI_NOT_IMPLEMENTED`.

---

### `/HFQ/CL_BADI_KOMDATA_API_RCV`

**Description**: Fallback für API-Empfang BadI (Fallback for API receive BAdI)
**Visibility**: PUBLIC
**Implements**: `IF_BADI_INTERFACE`, `/HFQ/IF_BADI_KOMDATA_API_RCV`

Default / fallback implementation for the BAdI `/HFQ/BADI_KOMDATA_API_RCV`. Registered as the default class in the enhancement spot.

| Method | Behaviour |
|--------|-----------|
| `CONSISTENT_DATA` | Logs message 024 ("technical problem during consistency check") to process log. Checks that exactly 3 `MSGCOMMENTS` are present and all are non-empty (URI, issuer, subject). Checks that the incoming version number (from `REF_TO_MSG[AGK]`) is greater than the current stored version. Checks that the incoming start date is not before the current version's start date. Appends `GC_CR_OK` if all checks pass, otherwise `GC_CR_INCONSISTANT`. |
| `UPDATE_API` | Logs message 025, then appends `GC_CR_NOT_IMPLEMENTED` — no customer implementation in the fallback. |

---

## Interfaces

### `/HFQ/IF_BADI_KOMDATA_API_RCV`

**Description**: für API Empfangs-BadI (for API receive BAdI)
**Implements**: `IF_BADI_INTERFACE`

BAdI interface defining two class-methods:

| Method | Parameters | Purpose |
|--------|-----------|---------|
| `CONSISTENT_DATA` | `IS_PROCESS_STEP_KEY` (importing); `CT_CHECK_RESULT`, `CR_DATA`, `CR_DATA_LOG` (changing) | Customer extension point: validate consistency of received API PARTIN data. Should append `GC_CR_OK`, `GC_CR_INCONSISTANT`, or `GC_CR_ERROR` to `CT_CHECK_RESULT`. |
| `UPDATE_API` | same signature | Customer extension point: perform custom actions when API data is received and passes consistency check. Should append `GC_CR_OK`, `GC_CR_ERROR`, or `GC_CR_NOT_IMPLEMENTED`. |

---

## Function Groups

### `/HFQ/FG_KOMDATA_API`

Process-triggering function group for API PARTIN send. Contains two function modules.

#### `/HFQ/FB_KOMDATA_API_SND`

**Author**: Hochfrequenz, Feb. 2024 (SCHULZR)

Triggers the Einzelversand (single-receiver send) API Kommunikationsparameter process.

| Parameter | Direction | Type | Description |
|-----------|-----------|------|-------------|
| `IS_PROCESS_STEP_ALL` | Import | `/IDXGC/S_PROC_STEP_DATA_ALL` | Source process step data (direction/data filled by caller) |
| `IV_SERVICE_REC` | Import | `SERVICE_PROV` | Receiver service provider |
| `IV_SERVICE_SEN` | Import | `SERVICE_PROV` | Sender service provider |
| `ES_PROC_STEP_ALL` | Export | `/IDXGC/S_PROC_STEP_DATA_ALL` | Process step data of the triggered process |

Logic: reads process config for `GC_PROCID_KD_API_SND`, looks up BMID for `AMID = GC_AMID_37007`, builds process data with `DIRECTION = 2` (outbound), sets `OWN_SERVPROV = IV_SERVICE_SEN`, `ASSOC_SERVPROV = IV_SERVICE_REC`, calls `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS`, commits, returns the triggered step's data.

#### `/HFQ/TRIG_KOMDATA_API_MAS`

**Author**: Hochfrequenz, Feb. 2024 (SCHULZR)

Triggers the Massenversand (mass send) API Kommunikationsparameter process.

| Parameter | Direction | Type | Description |
|-----------|-----------|------|-------------|
| `IV_ISSUER` | Import | `TEXT100` | Certificate issuer |
| `IV_SUBJECT` | Import | `TEXT100` | Certificate subject |
| `IV_API` | Import | `TEXT100` | Target URI |
| `IV_VERSION_OLD` | Import | `/HFQ/DE_KD_VERSION` | Previous version number |
| `IV_VERSION` | Import | `/HFQ/DE_KD_VERSION` | Current version number |
| `IV_SERVICE_SEN` | Import | `SERVICE_PROV` | Sender service provider |
| `IV_DATE` | Import | `DATUM` | Validity start date of the version |
| `IT_RECEIVER` | Import | `/HFQ/TT_SERVICEID` | List of specific receivers (empty = mass send to all) |
| `ES_PROC_STEP_ALL` | Export | `/IDXGC/S_PROC_STEP_DATA_ALL` | Triggered process step data |
| `/HFQ/CX_PARTIN_ERROR` | Exception | | Raised on process start failure |

Logic: reads process config for `GC_PROCID_KOMDAT_MAS`, looks up BMID for `GC_AMID_37007`, populates `REF_TO_MSG` with current version (qualifier `AGK`) and optionally previous version (qualifier `ACW`), populates `MSGCOMMENTS` with URI (qualifier `Z17`), subject (`Z23`), issuer (`Z24`), adds each entry of `IT_RECEIVER` as `MARKETPARTNER_ADD` with `PARTY_FUNC_QUAL = 'REC'`. Validates that previous version < current version. Starts the process and returns step data.

---

### `/HFQ/FG_KD_API`

Table maintenance function group for the customizing view `/HFQ/KD_API_SND`. SAP-generated via SM30 view maintenance framework.

Contains two generated function modules:

| Function module | Purpose |
|-----------------|---------|
| `TABLEFRAME_/HFQ/FG_KD_API` | Overview/list screen (screen 2000) controller — delegates to `PERFORM TABLEFRAME`. |
| `TABLEPROC_/HFQ/FG_KD_API` | Table processing — delegates to `PERFORM TABLEPROC`. |

Two screens are defined:
- **Screen 0001** (detail): Fields `STYPE_SENDER`, `STYPE_RECEIVER` with dropdown support.
- **Screen 2000** (list): Table control `TCTRL_/HFQ/KD_API_SND` with columns `STYPE_SENDER`, `STYPE_RECEIVER`.

---

## Reports

### `/HFQ/RP_API_ALV`

**Transaction**: `/HFQ/KOMDATA_API` ("Versand API Kommunikationsparameter")
**Description**: Entry-point report for the API communications parameter management UI.

**Selection screen** (Block B1):
- `SO_SA_ID`: Service provider ID (ranges, based on `/HFQ/KOMDATA_API-SERVICEID`)
- `P_DATUM`: Validity date (`DATUM`)

**START-OF-SELECTION**: Instantiates `/HFQ/CL_KOMDATA_API_STEUERN` with the selection values and calls `->ALV( )`.

---

## Tables / Data Definitions

### `/HFQ/KOMDATA_API`

**Description**: API Informationen für Steuerbefehle anderer Serviceanbieter (API information for control commands of other service providers)
**Table class**: TRANSP (transparent table)
**Table category**: Application table (APPL0), client-dependent, language-dependent
**Buffering**: None

Primary key: `MANDT` + `SERVICEID` + `SERVICEID_RECEIVER` + `VERSION`

| Field | Key | Data element / type | Description |
|-------|-----|---------------------|-------------|
| `MANDT` | X | `MANDT` | Client |
| `SERVICEID` | X | `SERVICE_PROV` | Service provider (sender of the API credentials) |
| `SERVICEID_RECEIVER` | X | `/HFQ/DE_KD_SERVICEID_EMPFANG` | Receiving service provider |
| `VERSION` | X | `/HFQ/DE_KD_VERSION` | Version number |
| `PREV_VERSION` | | `/HFQ/DE_KD_PREV_VERSION` | Previous version number |
| `.INCLUDE /HFQ/S_KD_VALIDITY` | | structure include | Validity period (VON, BIS, STATUS) — *Inferred from usage in code: `ls_komdata_api-von`, `ls_komdata_api-bis`, `ls_komdata_api-status`* |
| `ISSUER` | | `TEXT100` | Certificate issuer (Zertifikatsaussteller) |
| `SUBJECT` | | `TEXT100` | Certificate subject/user (Zertifikatsnutzer) |
| `API` | | `TEXT100` | Target URI (Zieladresse) |
| `.INCLUDE ISU_ADMINL` | | structure include | Admin fields: `ERDAT`, `ERNAM`, `AEDAT`, `AENAM` |

---

### `/HFQ/KD_API_SND`

**Description**: An welche Servicearten API PARTIN gesendet wird (To which service types API PARTIN is sent)
**Table class**: TRANSP (transparent table), client-dependent
**Table category**: Customizing (CONTFLAG=C), maintained via SM30 view maintenance
**Buffering**: None

Primary key: `MANDT` + `STYPE_SENDER` + `STYPE_RECEIVER`

| Field | Key | Data element | Description |
|-------|-----|-------------|-------------|
| `MANDT` | X | `MANDT` | Client |
| `STYPE_SENDER` | X | `/HFQ/DE_KD_INTCODES` | Internal code (Intcode) of the sending service type |
| `STYPE_RECEIVER` | X | `/HFQ/DE_KD_INTCODES` | Internal code of the receiving service type |

This table governs which sender/receiver service-type combinations are permitted to exchange API PARTIN messages. Referenced in `OWN_SERVPROV` and the Einzelversand receiver validation in `HANDLE_USER_COMMAND`.

---

## Other Objects

### Enhancement Spot: `/HFQ/ES_KOMDATA_API`

**Type**: ENHS / BAdI definition (`BADI_DEF`)
**BAdI name**: `/HFQ/BADI_KOMDATA_API_RCV`
**Interface**: `/HFQ/IF_BADI_KOMDATA_API_RCV`
**Context mode**: N (no context / filter-independent)
**Default class**: `/HFQ/CL_BADI_KOMDATA_API_RCV`
**Fallback class active**: yes (`USE_FALLBACK_CLASS = X`)
**Short text**: Kundenerweiterungen für den Empfang von API Daten (Customer extensions for the receipt of API data)
**Spot short text**: Erweiterungsspot für PARTIN API

The BAdI provides two extension points for customers: `CONSISTENT_DATA` (consistency validation of incoming PARTIN) and `UPDATE_API` (custom post-processing after receipt). The fallback class provides a default implementation of the consistency check; `UPDATE_API` in the fallback returns `NOT_IMPLEMENTED`.

---

### Table Maintenance Object: `/HFQ/KD_API_SNDS`

**Type**: TOBJ (table maintenance object), object type S (view)
**Category**: CUST (customizing)
**Transport**: `OBJTRANSP = 2`
**Linked view**: `/HFQ/KD_API_SND`
**Maintenance function group**: `/HFQ/FG_KD_API`
**List screen**: 2000 / Detail screen: 0001

---

### Message Class: `/HFQ/KOMDATA_API`

**Description**: Nachrichtenklasse für AS4 Versand (Message class for AS4 send)
**Master language**: German

Selected messages (all in German):

| Nr. | Text |
|-----|------|
| 000 | Serviceanbieter &1 nicht gefunden |
| 004 | Die neue Version &1 muss mindestens 1 größer sein, als die Alte &2 |
| 006 | Bitte warten. Versandprozess wird verarbeitet. |
| 007 | Versenden abgeschlossen. |
| 009 | Es darf nur eine Zeile ausgewählt sein |
| 010 | Es dürfen nur die Daten vom eigenen SA gesendet werden |
| 011 | Es darf nur die aktuellste Version verschickt werden |
| 012 | Es wurde zu diesen Daten bereits ein Massenversand gestartet |
| 013 | Es muss zu diesen Daten zunächst ein Massenversand gestartet werden |
| 014 | Datenanlegen nur für eigene, versandberechtigte Serviceanbieter erlaubt |
| 016 | &1 liegt vor dem Beginndatum (&2) der derzeitig höchsten Version |
| 017 | Bitte alle Felder ausfüllen |
| 018 | Bitte mindestens einen Empfänger eingeben |
| 019 | An folgende Serviceanbieter kann oder darf nicht gesendet werden: |
| 021 | Es besteht potentiell eine Inkonsistenz der eingegangenen Daten. |
| 022 | Daten löschen nur für eigene Serviceanbieter erlaubt |
| 023 | Daten löschen nur für noch nicht versendete Daten erlaubt |
| 024 | Es ist ein technisches Problem bei der Konsitenzprüfung aufgetreten |
| 025 | Es ist ein technisches Problem in der Methode UPDATE_API aufgetreten |

---

### Table Types

| Type name | Row type | Description |
|-----------|----------|-------------|
| `/HFQ/T_KD_KOMDATA_API` | `/HFQ/KOMDATA_API` (structure) | Internal table of KOMDATA_API rows; standard table, default key |
| `/HFQ/T_SERVICEIDS` | `SERVICEID` (CHAR 10) | Internal table of service provider IDs; standard table, default key |

---

### Transaction

| Code | Program | Screen | Description |
|------|---------|--------|-------------|
| `/HFQ/KOMDATA_API` | `/HFQ/RP_API_ALV` | 1000 | Versand API Kommunikationsparameter |

---

### Namespace Object

| Object | Description |
|--------|-------------|
| `/HFQ/` namespace | Owner: Hochfrequenz |
