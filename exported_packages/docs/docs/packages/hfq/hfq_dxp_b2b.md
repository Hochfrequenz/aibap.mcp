# Package: HFQ / DXP_B2B

**Description**: Business-to-Business Functionalities
**Original language**: English (E)
**Number of objects**: 38 files across 7 classes, 1 interface, 2 transparent tables, 3 structure types, 1 report, 1 transaction, 3 domains, 6 data elements, 1 message class, 1 namespace descriptor

---

## Executive Summary

This package implements the end-to-end B2B (Business-to-Business) RLM (Registrierte Leistungsmessung — metered load profile) process for energy supply contracts. The core concern is managing the lifecycle of B2B sales documents from creation through forecast profile acquisition and final transaction event export.

The overall process flow is:

1. A B2B sales document with distribution channel `Y2` and metering procedure `E01` (RLM) is created in SAP SD.
2. `CL_SALES_ORDER_B2B` detects the relevant document, resolves connected sales orders via an external root ID (Dimater ID pattern `DI-\d{6}_\d_\d{4}`), and creates or reuses an EDM forecast profile.
3. The status record in `/HFQ/DXP_T_B2B` moves through a defined state machine (see status lifecycle below).
4. A workflow event (`TRIGGER_VALUE_REQUEST`) is raised via `CL_FORECAST_EVENT`; the event handler `CL_FCPROF_RECEIVER` picks it up and calls `CL_SALES_ORDER_B2B->HANDLE_PROFILE_REQUEST`.
5. `CL_FORECAST_REQEST` sends an HTTP GET to an RFC destination (configured via customizing), receives a JSON payload (`{ "success": true, "result": { "from": "...", "to": "...", "valuesInKWh": [...] } }`), and uploads the profile values to IS-U EDM via `BAPI_ISUPROFILE_IMPORT`.
6. Once all connected sales documents have status `50` (Forecast Values saved), a transaction event is created via `CL_EVENT_HANDLER` (defined outside this package).
7. `CL_PROFVAL_GETTER` implements the `IF_VALUE_GETTER` interface and handles exporting profile values for downstream data export pipelines.
8. Transaction `/HFQ/DXP_B2B_RPT` allows manual re-triggering of a stalled B2B processing step for a given sales document.

**Status lifecycle** (domain `/HFQ/DXP_D_B2B_STATUS`, type NUMC 2):

| Code | Meaning |
|------|---------|
| 00 | Item Creation failed |
| 10 | New Item created |
| 20 | Forecast Values Request triggered |
| 30 | Forecast Request failed |
| 40 | Forecast Value Upload failed |
| 50 | Forecast Values saved |
| 90 | Preliminary process completed |
| 95 | Transaction cancelled (pending reversal) |
| 97 | Reversal completed |

**Key external dependency**: Dimater (external system) — accessed via an RFC HTTP destination whose name is assembled as `<PARAMETER_B2B_FORECAST_RFC>_<SYSID><MANDT>`. The system identifies connected sales orders by the external reference ID pattern `DI-\d{6}_\d_\d{4}` stored in `VBAK-EXT_REF_DOC_ID`.

---

## Classes

### `/HFQ/DXP_CL_B2B` — Business-to-Business Status Persistence

**File**: `#hfq#dxp_cl_b2b.clas.abap` / `.clas.xml`
**Instantiation**: `CREATE PRIVATE` — singleton per sales document, buffered in the class-level sorted table `GT_BUFFER`.
**Message class**: `/HFQ/DXP_MC_B2B`

Central entity class. Encapsulates one row of `/HFQ/DXP_T_B2B`. All B2B status reads and writes go through this class.

**Public constants**:

| Constant | Value | Description |
|----------|-------|-------------|
| `GC_DOMNAME_B2B_STATUS` | `/HFQ/DXP_D_B2B_STATUS` | Domain name used for status validation |
| `GC_SALES_CHANNEL_B2B` | `Y2` | Distribution channel identifying B2B orders |
| `GC_B2B_STATUS-creation_error` | `00` | Item creation failed |
| `GC_B2B_STATUS-new` | `10` | New item |
| `GC_B2B_STATUS-forecast_requested` | `20` | Forecast request triggered |
| `GC_B2B_STATUS-forecast_request_error` | `30` | Forecast request failed |
| `GC_B2B_STATUS-forecast_upload_error` | `40` | Upload failed |
| `GC_B2B_STATUS-forecast_saved` | `50` | Forecast saved |
| `GC_B2B_STATUS-done` | `90` | Process completed |
| `GC_B2B_STATUS-cancel` | `95` | Cancellation pending |
| `GC_B2B_STATUS-cancelled` | `97` | Cancellation complete |
| `GC_PROPERTY-market_location` | `MARKET_LOCATION` | Property name for set_property |
| `GC_PROPERTY-forecast_profile` | `FORECAST_PROFILE` | Property name for set_property |

**Public methods**:

| Method | Signature | Description |
|--------|-----------|-------------|
| `GET_INSTANCE` (static) | `IV_SALES_DOCUMENT_ID`, `IV_NEW` (default ABAP_FALSE) → `RR_INSTANCE` | Returns buffered instance or creates new one; raises `/HFQ/DXP_CX_GENERAL_ERROR` if existence in DB contradicts `IV_NEW` |
| `SEARCH` (static) | `IS_SEARCH_PARAMETERS` (type `TY_S_SEARCH_PARAMETERS`) → `RR_INSTANCE` | Queries `/HFQ/DXP_T_B2B` by status, forecast profile, market location; filters by key date range and division; raises error if no match found |
| `GET_DATA` | → `RS_DATA` (`/HFQ/DXP_S_B2B_DATA`) | Returns data sub-structure (market location + forecast profile) |
| `GET_DATE_FROM` / `GET_DATE_TO` | → `RV_DATE` | Reads contract start/end date from sales order configuration attributes `M_VERTR_VERTRAGSLAUFZEIT_VON` / `_BIS` (stored as float, converted via `CTCV_CONVERT_FLOAT_TO_DATE`) |
| `GET_DIVISION` | → `RV_DIVISION` (SPARTE) | Reads `VBAK-SPART` on demand |
| `GET_EXTERNAL_DOCUMENT_ID` | → `RV_EXTERNAL_REFERENCE_ID` (SD_EXT_REF_DOC_ID) | Reads `VBAK-EXT_REF_DOC_ID` on demand |
| `GET_SALES_DOC_ID` | → `RV_SALES_DOCUMENT_ID` | Returns the key |
| `GET_STATUS` | → `RV_STATUS` | Returns current B2B status |
| `IS_B2B` | → `RV_BOOL` | True if distribution channel = `Y2` |
| `IS_RLM` | → `RV_BOOL` | True if custom config attribute `M_RPROD_ZAEHLVERFAHREN` = `E01` (metering procedure E01) |
| `SAVE` | `IV_FORCE_PROCESS` (default ABAP_FALSE) | Inserts (new) or updates (existing) the DB row; calls `PROCESS_NEXT_STEP` if data changed or forced |
| `SET_PROPERTY` | `IV_PROPERTY`, `IV_VALUE` | Sets a field in `GS_DATA_ALL-DATA` by dynamic field assignment |
| `SET_STATUS` | `IV_STATUS` | Validates the value against domain `/HFQ/DXP_D_B2B_STATUS` before setting |
| `SET_RESPONSE_CODE` | `IV_RESPONSE_STATUS_CODE` | Stores the HTTP response code received from Dimater |

**Private after-save dispatch** (`PROCESS_NEXT_STEP` / `GET_AFTER_SAVE_METHOD`):

| Status at save time | Method called |
|---------------------|---------------|
| 10 / 30 / 40 (new / request error / upload error) | `CREATE_REQUEST_EVENT` |
| 50 (forecast saved) | `CREATE_TRANSACTION_EVENT` |
| 95 (cancel) | `REVERSE_TRANSACTION_EVENT` |

`CREATE_REQUEST_EVENT` and `CREATE_TRANSACTION_EVENT` / `REVERSE_TRANSACTION_EVENT` all guard themselves to only act on the **parent** sales document (identified via `CL_DB_MSO_ACCESS->GET_PARENT_SALES_DOC`).

`CAN_TRANSACTION_BE_CREATED` blocks transaction event creation until all connected sales documents have reached status >= 50.

**Constructor** uses `BAPISDORDER_GETDETAILEDLIST` to load custom configuration values (characteristic values, stored in `GT_CUSTOM_VALUES` of type `IBAPICUVALM`), which feed `GET_DATE_FROM`, `GET_DATE_TO`, `IS_RLM`, and `COMPLETE_DATA`.

---

### `/HFQ/DXP_CL_DB_MSO_ACCESS` — Database Access for `/HFQ/DXP_T_MSO`

**File**: `#hfq#dxp_cl_db_mso_access.clas.abap` / `.clas.xml`
**Instantiation**: `CREATE PRIVATE` — application-level singleton via `GET_INSTANCE`.
**Message class**: `/HFQ/DXP_MC_B2B`

Manages the multi-sales-order (MSO) mapping table `/HFQ/DXP_T_MSO`. This table links connected sales documents (sharing the same external root ID, e.g., all year-segments of one Dimater contract) and tracks their parent-child relationships and USC process modes.

**Public methods**:

| Method | Signature | Description |
|--------|-----------|-------------|
| `GET_INSTANCE` (static) | → `RR_INSTANCE` | Returns singleton |
| `GET_CONNECTED_MSO_ENTRIES` | `IV_SALES_DOC` → `RT_MSO` (`TY_T_MSO`) | All MSO rows sharing the same external root ID |
| `GET_CONNECTED_SALES_DOCS` | `IV_SALES_DOC` → `RT_SALES_DOCS` (`TY_T_SALES_DOC_IDS`) | All child sales document IDs for the same external root ID |
| `GET_EXT_ROOT_ID` | `IV_SALES_DOC` → `RV_EXT_ROOT_ID` | Looks up cached/DB entry and returns its root ID |
| `GET_PARENT_SALES_DOC` | `IV_SALES_DOC` → `RV_PARENT_SALES_DOC` | Returns `PARENT_SALES_DOC_ID` from the MSO entry |
| `GET_USC_MODE_OF_FIRST_SDOC` | `IV_SALES_DOC` → `RV_USC_MODE` | Returns `NEW_CNTR` if any connected MSO entry has USC mode `NEW_CNTR`, else `CHNG_PRD` |
| `GET_USC_MODE_OF_SALES_DOC` | `IV_SALES_DOC` → `RV_USC_MODE` | Same logic as above (identical implementation) |
| `INSERT_ENTRIES` | `IT_SALES_DOC_USC_MODE` | Inserts new MSO rows; first entry of a group becomes its own parent |
| `CHECK_ENTRY_EXISTS` | `IV_SALES_DOC` → `RV_BOOL` | Returns false (no exception) if no entry exists |

**External root ID determination** (`DETERMINE_EXT_ROOT_ID`, private): Reads `VBAK-EXT_REF_DOC_ID` via `CL_WZRE_READ_VBAK`. If the ID matches the PCRE `DI-\d{6}_\d_\d{4}` (Dimater ID format with year suffix), strips the last 4 characters (the year) to derive the root ID; otherwise uses the full value as-is.

**Buffering**: All reads are cached in `GT_MSO_BUFFER` (public, sorted table), keyed by `EXT_ROOT_ID` + `CHILD_SALES_DOC_ID`.

---

### `/HFQ/DXP_CL_FCPROF_RECEIVER` — Receives and Processes Forecast Profile Value Requests

**File**: `#hfq#dxp_cl_fcprof_receiver.clas.abap` / `.clas.xml`
**Instantiation**: `CREATE PUBLIC`
**Interface implemented**: `BI_EVENT_HANDLER_STATIC`

Workflow event handler. Called by the SAP Business Workflow engine when the event `TRIGGER_VALUE_REQUEST` is raised on the class `/HFQ/DXP_CL_FORECAST_EVENT`.

The single method `BI_EVENT_HANDLER_STATIC~ON_EVENT`:
1. Reads the `SALES_DOCUMENT_ID` parameter from the event container (constant `GC_EVT_PARAM_SALES_DOCUMENT_ID` = `'SALES_DOCUMENT_ID'`).
2. Calls `/HFQ/DXP_CL_SALES_ORDER_B2B=>GET_INSTANCE( )->HANDLE_PROFILE_REQUEST( lv_sales_document_id )`.
3. Translates any `/HFQ/DXP_CX_GENERAL_ERROR` into `CX_BO_ERROR`.

---

### `/HFQ/DXP_CL_FORECAST_EVENT` — Event Class for Profile Value Request Coupling

**File**: `#hfq#dxp_cl_forecast_event.clas.abap` / `.clas.xml`
**Instantiation**: `CREATE PUBLIC`
**Inherits from**: `/HFQ/DXP_CL_DYN_OBJECT`
**Interfaces implemented**: `BI_OBJECT`, `BI_PERSISTENT`, `IF_WORKFLOW`
**Class event**: `TRIGGER_VALUE_REQUEST` exporting `SALES_DOCUMENT_ID`

SAP Business Workflow object class. The `BI_OBJECT`, `BI_PERSISTENT`, and `IF_WORKFLOW` interface methods are all empty stubs — only `RAISE_EVENT` contains logic.

**Public methods**:

| Method | Description |
|--------|-------------|
| `GET_INSTANCE` (static) | Factory — always returns a new instance (`NEW #()`) |
| `RAISE_EVENT` | Builds a workflow event container, sets `SALES_DOCUMENT_ID`, raises the event via `CL_SWF_EVT_EVENT->RAISE( )` with debugging enabled |

---

### `/HFQ/DXP_CL_FORECAST_REQEST` — Request Forecast Data from Webservice

**File**: `#hfq#dxp_cl_forecast_reqest.clas.abap` / `.clas.xml`
*(Note: class name contains a typo — "REQEST" instead of "REQUEST".)*
**Instantiation**: `CREATE PRIVATE`
**Message class**: `/HFQ/DXP_MC_B2B`
**Release status**: `K` (released for customer)

Encapsulates the HTTP call to the Dimater external system for forecast profile values.

**Public constants**:

| Constant | Value |
|----------|-------|
| `GC_ACCEPT` | `'accept'` |
| `GC_UTF_8` | `'application/json;charset=utf-8'` |

**Public methods**:

| Method | Signature | Description |
|--------|-----------|-------------|
| `GET_INSTANCE` (static) | `IV_DESTINATION` (RFCDEST), `IV_SALES_DOCUMENT_ID` → `RR_FORECAST_REQUEST` | Factory |
| `GET_DATA` | → `EV_DATE_FROM`, `EV_DATE_TO`, `ET_PROFILE_VALUES` | Full HTTP request/response cycle |

**`GET_DATA` behaviour**:
1. Retrieves the external document ID from the B2B status instance.
2. Opens an HTTP connection via `CL_HTTP_CLIENT=>CREATE_BY_DESTINATION` using the RFC destination.
3. Sets the request header `externalDocumentId` to the external document ID.
4. Sends a GET request; receives response.
5. Stores the HTTP response status code on all connected sales documents via `SET_RESPONSE_CODE`.
6. If the response status code matches `[45]\d\d` (4xx or 5xx), raises an error.
7. Parses the JSON body (`CONVERT_JSON`): checks `success == true`, extracts `result.from`, `result.to` (ISO date strings), and `result.valuesInKWh` (array of floats).
8. Returns profile values as a table of `TY_PROFILE_VALUE` (field: `value TYPE E_PROFVAL_INT`).

**JSON structure expected from Dimater**:
```
{
  "success": true,
  "result": {
    "from": "YYYY-MM-DD",
    "to":   "YYYY-MM-DD",
    "valuesInKWh": [<float>, ...]
  }
}
```

---

### `/HFQ/DXP_CL_PROFVAL_GETTER` — Getter Class for Profile Values

**File**: `#hfq#dxp_cl_profval_getter.clas.abap` / `.clas.xml`
**Instantiation**: `CREATE PUBLIC`
**Interface implemented**: `/HFQ/DXP_IF_VALUE_GETTER`
**Master language**: German (D) — *Inferred from XML metadata; German descriptions in XML.*

Pluggable value getter for the DXP data export framework. Retrieves forecast profile values from IS-U EDM for a given sales order and time window.

**Private attributes**: `MV_PROFILE` (EDM profile number), `MV_DATE_FROM`, `MV_DATE_TO`, `MV_TIME_FROM`, `MV_TIME_TO`, `MV_TIME_ZONE` (default `'CET'`), `MV_IS_RELEVANT`, `MV_DIVISION_CATEGORY`.

**Interface methods**:

| Method | Description |
|--------|-------------|
| `SET` | Reads date range (`move_in_date_parent`, `move_out_date_parent`), forecast profile, and division category from the export data structure. For gas: sets time_from = 06:00:00 and adds one day to `date_to`. For electricity: sets time_from = 00:00:00. Time_to is always time_from − 1 second. |
| `GET` | Calls `ISU_EDM_PROFILE_EXPORT_INTF` to retrieve profile values. For electricity: multiplies each value by 4 (quarter-hour to hourly conversion). Formats as a JSON-like bracket-delimited string of 6-decimal values. |
| `IS_RELEVANT` | Returns true only if metering procedure = `E01` (RLM). Evaluated once and cached in `MV_IS_RELEVANT`. |
| `DISPLAY` | Calls `ISU_S_PROFLIST_DISPLAY` to show the profile in a dialog. |

---

### `/HFQ/DXP_CL_SALES_ORDER_B2B` — Sales Order Framework for B2B Customers

**File**: `#hfq#dxp_cl_sales_order_b2b.clas.abap` / `#hfq#dxp_cl_sales_order_b2b.clas.locals_imp.abap` / `.clas.xml`
**Instantiation**: `CREATE PUBLIC` — application-level singleton via `GET_INSTANCE`.
**Message class**: `/HFQ/DXP_MC_B2B`

Main orchestration class. Contains the top-level handlers called from outside this package (SD user exits, workflow event receivers) and the private logic for profile creation and upload.

**Public methods**:

| Method | Signature | Description |
|--------|-----------|-------------|
| `GET_INSTANCE` (static) | → `RR_INSTANCE` | Returns singleton; raises message 006 ("B2B Customer Process is not active") if the B2B process flag is inactive in customizing (`CL_CONFIG_ACCESS->IS_B2B_PROCESS_ACTIVE`) |
| `HANDLE_SALES_DOC_UPLOAD` | `IV_SALES_DOCUMENT_ID` (optional) | Entry point after SD sales document creation. Guards against duplicate processing via `CL_DB_MSO_ACCESS->CHECK_ENTRY_EXISTS`. Determines and inserts connected sales docs, then calls `TRIGGER_SALES_DOC_UPLOAD` on the parent. |
| `HANDLE_PROFILE_REQUEST` | `IV_SALES_DOCUMENT_ID` | Called by the workflow event receiver. Sets all connected docs to status 20, calls `CL_FORECAST_REQEST->GET_DATA` (for parent only — Dimater returns a single dataset), uploads profile values, sets all docs to status 50 or error statuses on failure. |
| `HANDLE_REVERSAL` | `IV_SALES_DOCUMENT_ID` | Sets all connected child docs to status 95, then sets parent to 95 last (triggering reversal event). |
| `TRIGGER_SALES_DOC_UPLOAD` | `IV_SALES_DOCUMENT_ID` | Checks RLM and B2B relevance; searches for an existing forecast profile for the same market location and division (reuse); if not found, creates a new EDM profile via `LCL_GENERATOR`; propagates profile to child docs; saves parent last (triggering the forecast request event). |

**Private methods**:

| Method | Description |
|--------|-------------|
| `DETERMINE_CONNECTED_SALES_DOCS` | Queries `VBAK` for sales orders with `AUART = 'Y002'`, `VTWEG = 'Y2'`, and `EXT_REF_DOC_ID LIKE '<root>%'`; maps item category `Y002` → USC mode `NEW_CNTR`, `Y004` → `CHNG_PRD`. |
| `CREATE_FORECAST_PROFILE` | Uses the local class `LCL_GENERATOR` to create a new EDM profile header via `/UCOM/CL_PROFILE_HDR->CREATE`. Profile text is set to `"Prognose-Profil für RLM Marktlokation <MALO>"`. Profile type is looked up from customizing based on USC mode / source. |
| `CHECK_FORECAST_PROFILE` | Ensures the existing profile header's date range covers the sales order date range; extends it if needed. For gas: also ensures `date_to` is set one day beyond the contract end with `time_to = 05:59:59`. |
| `UPLOAD_PROFILE_VALUES` | Calls `BAPI_ISUPROFILE_IMPORT` with prepared profile values; raises error on any `E`-type BAPI return. |
| `PREPARE_PROFILE_VALUES` | Converts flat list of KWh values into `BAPIISUPROVAL2` rows with timestamps. Interval is 15 min (`001500`) for electricity, 60 min (`010000`) for gas. Timestamps are converted to UTC. Status is set to `'W'` (valid). Unit is `'KWH'`. |

**Local class `LCL_GENERATOR`** (defined in `.clas.locals_imp.abap`):
Private helper for EDM profile header creation. Default values: date_to = `99991231`, time_to = `23:59:59`, time zone `CET`, interval 15 min (electricity), 6 decimal places, unit `KWH`. Gas overrides: start 06:00:00, interval 60 min, date_to extended by 1 day. Creates the profile via `/UCOM/CL_PROFILE_HDR->CREATE`.

---

## Interfaces

### `/HFQ/DXP_IF_B2B_CONSTANTS` — Constants for B2B Package

**File**: `#hfq#dxp_if_b2b_constants.intf.abap` / `.intf.xml`

Pure constants interface. All consuming classes use this interface to avoid hard-coded literals.

| Constant | Type | Value | Description |
|----------|------|-------|-------------|
| `GC_INTERVAL_TYPE_QUARTER_HOUR` | INTSIZEID | `'15'` | 15-minute interval |
| `GC_INTERVAL_TYPE_HOUR` | INTSIZEID | `'60'` | 60-minute interval |
| `GC_START_OF_GAS_DAY` | TIMS | `'060000'` | Gas day start: 06:00:00 |
| `GC_START_OF_ELECTRICITY_DAY` | TIMS | `'000000'` | Electricity day start: 00:00:00 |
| `GC_TIMS_QUARTER_HOUR` | TIMS | `'001500'` | Duration of one quarter hour |
| `GC_TIMS_HOUR` | TIMS | `'010000'` | Duration of one hour |
| `GC_PROFILE_UNIT` | E_UNITINTF | `'KWH'` | Profile value unit |
| `GC_B2B_STATUS_COMPLETED` | /HFQ/DXP_E_B2B_STATUS | `'90'` | Final completed status |
| `GC_B2B_STATUS_FORECAST_SAVED` | /HFQ/DXP_E_B2B_STATUS | `'50'` | Forecast saved status |
| `GC_EVT_PARAM_SALES_DOCUMENT_ID` | SWFDNAME | `'SALES_DOCUMENT_ID'` | Workflow event container parameter name |
| `GC_EVENT_VALUE_REQUEST` | SWO_EVENT | `'TRIGGER_VALUE_REQUEST'` | Event name raised on `CL_FORECAST_EVENT` |
| `GC_SO_ATTRIBUTE_METER_PROC` | CU_CHARC | `'M_RPROD_ZAEHLVERFAHREN'` | SO config attribute: metering procedure |
| `GC_SO_ATTRIBUTE_CONTR_DFROM` | CU_CHARC | `'M_VERTR_VERTRAGSLAUFZEIT_VON'` | SO config attribute: contract start date (float) |
| `GC_SO_ATTRIBUTE_CONTR_DTO` | CU_CHARC | `'M_VERTR_VERTRAGSLAUFZEIT_BIS'` | SO config attribute: contract end date (float) |
| `GC_SO_ATTRIBUTE_MALO` | CU_CHARC | `'M_VERTR_MALO'` | SO config attribute: market location ID |
| `GC_PROFVAL_STATUS_VALID` | EDM_EXTSTATUS | `'W'` | EDM profile value status: valid |
| `GC_REGEX_DIMATER_ID` | FCLM_REGEX_FIND | `'DI-\d{6}_\d_\d{4}'` | PCRE pattern to detect full Dimater IDs |
| `GC_USC_MODE_NEW_CNTR` | CRMS4_IU_PFP_KIND | `'NEW_CNTR'` | USC process type: new contract |
| `GC_USC_MODE_CHNG_PRD` | CRMS4_IU_PFP_KIND | `'CHNG_PRD'` | USC process type: change period |

---

## Reports

### `/HFQ/DXP_B2B_PROCESS_STEP` — B2B RLM Step Reprocessing

**File**: `#hfq#dxp_b2b_process_step.prog.abap` / `.prog.xml`
**Transaction**: `/HFQ/DXP_B2B_RPT` ("B2B RLM nach Fehler fortsetzen" — B2B RLM continue after error)
**Title (EN)**: "B2B RLM Step Reprocessing" | **(DE)**: "B2B RLM Schritt Neuverarbeitung"

Selection screen accepts a single mandatory parameter `P_SD_ID` (type `/HFQ/DXP_E_SALES_DOCUMENT_ID`).

Behaviour:
1. Performs an authority check on `S_TCODE` for `/HFQ/DXP_B2B_RPT`; abends with `MESSAGE A172(00)` on failure.
2. Calls `CL_B2B=>GET_INSTANCE( P_SD_ID )->SAVE( IV_FORCE_PROCESS = ABAP_TRUE )`.
3. Displays success message 013 (`"Processing of B2B item &1 triggered."`) or shows error details from the caught `CX_GENERAL_ERROR` as type `I` displayed like `E`.

The `FORCE_PROCESS = TRUE` flag ensures `PROCESS_NEXT_STEP` is always triggered even if the data has not changed, making this report useful for restarting stuck B2B items without modifying any data.

---

## Tables / Data Definitions

### Transparent Database Tables

#### `/HFQ/DXP_T_B2B` — Process Status for B2B Customers

**Type**: Transparent, client-dependent, language-dependent
**Buffering**: None (`BUFALLOW = N`)
**Delivery class**: A (application data)

| Field | Key | Data element | Description |
|-------|-----|-------------|-------------|
| `CLIENT` | X | `MANDT` | Client |
| `SALES_DOCUMENT_ID` | X | `/HFQ/DXP_E_SALES_DOCUMENT_ID` | Sales document number (CHAR 10, ALPHA conversion) |
| (include) | | `/HFQ/DXP_S_B2B_DATA` | Market location + forecast profile |
| (include) | | `/HFQ/DXP_S_B2B_STATUS_FIELDS` | B2B status, response code, admin fields |

**Secondary index `MAL`**: `CLIENT` + `MARKET_LOCATION` — supports the `SEARCH` method which filters by market location.

#### `/HFQ/DXP_T_MSO` — Mapping of Connected Sales Orders

**Type**: Transparent, client-dependent
**Buffering**: None (`BUFALLOW = N`)
**Delivery class**: A (application data)

| Field | Key | Data element / type | Description |
|-------|-----|--------------------|-------------|
| `MANDT` | X | `MANDT` | Client |
| `EXT_ROOT_ID` | X | `/HFQ/DXP_E_EXT_ROOT_ID` | External root ID (CHAR 40) — the Dimater ID stem |
| `CHILD_SALES_DOC_ID` | X | `/HFQ/DXP_E_SALES_DOCUMENT_ID` | Child sales document number |
| `PARENT_SALES_DOC_ID` | | `/HFQ/DXP_E_SALES_DOCUMENT_ID` | Parent sales document number |
| `USC_MODE` | | `CRMS4_IU_PFP_KIND` | Process type: `NEW_CNTR` or `CHNG_PRD` |

### Structure Types (INTTAB)

#### `/HFQ/DXP_S_B2B_DATA` — Data Fields for B2B Customer Process Status

| Field | Data element | Description |
|-------|-------------|-------------|
| `MARKET_LOCATION` | `/HFQ/DXP_E_MARKET_LOCATION` | Market location identifier (MaLo) |
| `FORECAST_PROFILE` | `/HFQ/DXP_E_FORECAST_PROFILE` | EDM forecast profile number |

#### `/HFQ/DXP_S_B2B_STATUS_FIELDS` — Status Fields for Sales Document B2B Process

| Field | Data element | Description |
|-------|-------------|-------------|
| `B2B_STATUS` | `/HFQ/DXP_E_B2B_STATUS` | B2B process status (NUMC 2, domain-validated) |
| `RESPONSE_STATUS_CODE` | `/HFQ/DXP_E_B2B_RESPONSE_CODE` | HTTP response code from Dimater (NUMC 3) |
| (include) | `ISU_ADMIN_EXT` | IS-U admin fields: created by/date/time, changed by/date/time |

#### `/HFQ/DXP_S_B2B_DATA_ALL` — All Non-Key Fields for B2B Customer Process Status

Composite structure that includes both `/HFQ/DXP_S_B2B_DATA` (group `DATA`) and `/HFQ/DXP_S_B2B_STATUS_FIELDS` (group `STATUS`). Used as the in-memory representation inside `CL_B2B` (`GS_DATA_ALL`, `GS_DATA_ALL_OLD`).

---

## Domains and Data Elements

### Domains

| Domain | Type | Length | Description |
|--------|------|--------|-------------|
| `/HFQ/DXP_D_B2B_STATUS` | NUMC | 2 | B2B Customer Process Status — 9 fixed values (00–97); bilingual (EN/DE) |
| `/HFQ/DXP_D_EXT_ROOT_ID` | CHAR | 40 | Root ID of external document ID (Dimater ID stem) |
| `/HFQ/DXP_D_SALES_DOCUMENT_ID` | CHAR | 10 | Sales and Distribution Document Number; conversion exit `ALPHA` |

### Data Elements

| Data element | Domain / base type | Description |
|-------------|-------------------|-------------|
| `/HFQ/DXP_E_B2B_STATUS` | `/HFQ/DXP_D_B2B_STATUS` | B2B Customer Process Status |
| `/HFQ/DXP_E_B2B_RESPONSE_CODE` | NUMC 3 (no domain) | HTML Response Status Code |
| `/HFQ/DXP_E_EXT_DOCUMENT_ID` | `CHAR40` | External Document ID (full Dimater reference ID) |
| `/HFQ/DXP_E_EXT_ROOT_ID` | `/HFQ/DXP_D_EXT_ROOT_ID` | Root ID of external document ID |
| `/HFQ/DXP_E_FORECAST_PROFILE` | `E_PROFILE` | Number of EDM Profile; search help `E_PROFILE` on field `PROFILE` |
| `/HFQ/DXP_E_SALES_DOCUMENT_ID` | `/HFQ/DXP_D_SALES_DOCUMENT_ID` | Sales Document; memory ID `AUN` |

---

## Other Objects

### Message Class `/HFQ/DXP_MC_B2B` — Messages for B2B Customer Functionalities

**Master language**: English

| Number | Text |
|--------|------|
| 000 | Property &1 does not exist. |
| 001 | Sales Doc. &1 could not be inserted. |
| 002 | Sales Doc. &1 Data could not be updated. |
| 003 | Transaction Event Creation Error for Sales Doc. &1. |
| 004 | &1 is not a valid B2B status. |
| 005 | No Sales Document identified for Search Parameters. |
| 006 | B2B Customer Process is not active. |
| 007 | Establishing Connection to Webservice failed with Error &1. (CONSTRUCTOR) |
| 008 | Closing of Connection failed. |
| 009 | Connection failed. HTTP-ERROR: &1 |
| 010 | Request Failed, Success Field contains: &1 |
| 011 | Error Extracting Data From JSON |
| 012 | B2B Activation status is inconsistent. |
| 013 | Processing of B2B item &1 triggered. |
| 014 | External root ID &1 is not existing. |
| 015 | Sales Document &1 is not existing. |

### Namespace Descriptor `/HFQ/`

**File**: `#hfq#.nspc.xml`
**Owner** (DE): Hochfrequenz
Description available in German only (`/HFQ/ Namespace`).

### Transaction `/HFQ/DXP_B2B_RPT`

Calls report `/HFQ/DXP_B2B_PROCESS_STEP`, dynpro 1000.
**Transaction text** (EN/DE): "B2B RLM nach Fehler fortsetzen" (B2B RLM continue after error).
