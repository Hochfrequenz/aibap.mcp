# Package: HFQ / DXP_BASIS

**Description**: PFM-Framework: Basis Package
**Original language**: English (master language: German — `MASTERLANG=D`)
**Number of objects**: 83 files across 4 sub-packages (`/src/`, `/src/ape/`, `/src/belvis/`, `/src/mdx/`)

---

## Executive Summary

`HFQ/DXP_BASIS` is the foundational package of the PFM (Partner Framework Management) data export framework. It defines the core event lifecycle: an *event* (also called a *transaction* in the codebase) is created for an object (a Point of Delivery or Sales Document), assigned a UUID transaction ID, persisted in `/HFQ/DXP_T_EVE`, then progressed through data-collection, data-conversion, and data-export phases, each tracked by a status field. The framework dispatches to configurable `IDataProvider`, `IDataConverter`, and `IDataExporter` implementations, which are resolved at runtime by class name from customizing. Three adapter sub-packages — `ape` (APE integration), `belvis` (belvis ERP integration), and `mdx` (MDX master-data-change integration) — extend the base framework and contain only stub or partially commented-out implementations, indicating active development. *Inferred from class names, interface signatures, and commented-out code blocks.*

---

## Classes

### `/HFQ/DXP_CL_DB_EVE` — Database Access for Event Table

Static (class-method-only) data access object for table `/HFQ/DXP_T_EVE`. Maintains an internal read buffer keyed by `TRANSACTION_ID` to avoid repeated database reads.

**Public class-methods:**

```abap
CLASS-METHODS get_data
  IMPORTING !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
  RETURNING VALUE(rs_event_data) TYPE ty_s_event_data_add
  RAISING /hfq/dxp_cx_general_error.

CLASS-METHODS insert_event
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !is_event_data     TYPE /hfq/dxp_s_event_data
  RAISING /hfq/dxp_cx_general_error.

CLASS-METHODS update_event
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !is_event_data     TYPE /hfq/dxp_s_event_data
  RAISING /hfq/dxp_cx_general_error.
```

`get_data` first checks the class-level sorted-table buffer; on miss it does `SELECT SINGLE` from `/HFQ/DXP_T_EVE`. After read or insert it calls the private `complete_data`, which converts the stored UTC timestamp to local date/time and resolves an external identifier. `update_event` also stamps `CHANGED_AT` / `CHANGED_BY` from `GET TIME STAMP` / `SY-UNAME`. The call to ISU function `ISU_DB_EUITRANS_INT_SINGLE` for PoD external-ID resolution is commented out; the current implementation returns `IV_OBJECT_KEY` directly.

**Inner type `ty_s_event_data_add`**: extends `/HFQ/DXP_S_EVENT_DATA` with `KEY_DATE` (DATS), `KEY_TIME` (TIMS), and `EXTERNAL_ID` (CHAR50).

---

### `/HFQ/DXP_CL_DYN_OBJECT` — Abstract Base for Dynamic Class Instantiation

Abstract class. Provides a single `FINAL` protected method `get_class_name` that returns the ABAP class name of the current instance by calling `CL_ABAP_CLASSDESCR=>GET_CLASS_NAME( ME )` and stripping the `\CLASS=` prefix. No public interface.

---

### `/HFQ/DXP_CL_EVENT` — Event Lifecycle Object

`CREATE PRIVATE`. Represents a single PFM event/transaction. Instances are obtained via `CREATE` (new event) or `GET` (existing event by ID). The class maintains a class-level sorted table of live instances (`GT_EVENTS`).

**Public class-methods:**

```abap
CLASS-METHODS create
  IMPORTING !is_event_data TYPE /hfq/dxp_s_event_data
  RETURNING VALUE(ro_event) TYPE REF TO /hfq/dxp_cl_event
  RAISING /hfq/dxp_cx_general_error.

CLASS-METHODS get
  IMPORTING !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
  RETURNING VALUE(ro_event) TYPE REF TO /hfq/dxp_cl_event
  RAISING /hfq/dxp_cx_general_error.
```

**Public instance methods:**

```abap
METHODS prepare RAISING /hfq/dxp_cx_general_error.
METHODS commit
  IMPORTING !iv_no_dialog TYPE abap_bool DEFAULT abap_true
  RAISING /hfq/dxp_cx_general_error.
METHODS get_event_data RETURNING VALUE(rs_event_data) TYPE /hfq/dxp_s_event.
```

`create` generates a UUID via `CL_SYSTEM_UUID=>CREATE_UUID_X16_STATIC`, sets status to `New`, writes admin fields, and calls `CL_DB_EVE=>INSERT_EVENT`. `prepare` drives the state machine: status `New`/`DataError` → `collect_data`; status `Data`/`ConvError` → `convert_data`. `commit` delegates to `/HFQ/DXP_CL_EXPORT_HANDLER=>GET_INSTANCE( )->COMMIT_EXPORT`. `collect_data` dynamically instantiates data provider classes read from customizing via `/HFQ/DXP_CL_CONFIG_ACCESS`, calls `determine_data` then `save_data` on each. `convert_data` calls `EXPORT_HANDLER->PREPARE_EXPORT`. `export_data` is implemented as an empty stub.

---

### `/HFQ/DXP_CL_EVENT_HANDLER` — Batch Event Processing Orchestrator

Singleton (`GET_INSTANCE`). Processes a table of events through the full pipeline in one call.

**Public methods:**

```abap
CLASS-METHODS get_instance
  RETURNING VALUE(rr_instance) TYPE REF TO /hfq/dxp_cl_event_handler.

METHODS main
  IMPORTING !iv_no_dialog TYPE abap_bool DEFAULT abap_true
  EXPORTING !et_failed_events TYPE /hfq/dxp_tt_event
  CHANGING  !ct_events TYPE /hfq/dxp_tt_event.
```

`main` runs four sequential phases: `complete_event_data` (fills missing data from DB for events that already have a transaction ID), `create_events` (generates UUIDs and inserts new events for rows without a transaction ID), `trigger_data_provision` (runs all configured data providers per event; skipped for asynchronous execution mode), `trigger_export` (prepares and commits the export via `/HFQ/DXP_CL_EXPORT_HANDLER`). Events that fail any phase are moved from `CT_EVENTS` to `ET_FAILED_EVENTS` with the appropriate error status. Execution mode (`S`=synchronous, `A`=asynchronous) is read from customizing via `/HFQ/DXP_CL_CONFIG_ACCESS=>GET_EXECUTION_MODE`.

For PoD object types `trigger_data_provider` resolves MALO/MELO sub-index relations by calling `/UCOM/BADI_POD_DATA_ACCESS=>GET_INSTANCE()->IMP->GET_POD_MALO_MELO`, assigning a sequential sub-index to each MELO. For Sales Document object types it delegates to `/HFQ/DXP_CL_B2B` (not present in this package).

---

### `/HFQ/DXP_CL_EVENT_RECEIVER` — Business Workflow Event Receiver

Implements `BI_EVENT_HANDLER_STATIC`. Receives SAP Business Workflow events (event names `TRIGGER_DATA_PROVISION` and `TRIGGER_DATA_EXPORT` are defined in the constants interface). Extracts the `TRANSACTION_ID` parameter from the workflow event container, constructs a single-entry event table with synchronous execution mode, and calls `CL_EVENT_HANDLER=>GET_INSTANCE()->MAIN`.

---

### `/HFQ/DXP_CL_EVENT_TRIGGER` — Installation-Based Manual Event Trigger

Singleton. Used by the report `/HFQ/DXP_EVENT_TRIGGER` to manually create events for a set of SAP IS-U installations.

**Public methods:**

```abap
CLASS-METHODS get_instance
  RETURNING VALUE(rr_instance) TYPE REF TO /hfq/dxp_cl_event_trigger.

METHODS main
  IMPORTING
    !it_installation_range TYPE ranges_anlage
    !iv_key_date           TYPE /hfq/dxp_e_key_date
    !iv_event_scenario     TYPE /hfq/dxp_e_event_scenario.

METHODS create CHANGING !cs_event TYPE /hfq/dxp_t_eve.
```

`main` validates mandatory inputs, selects PoD internal UIs from `EUIINSTLN` for the given installation range and key date, builds an event list, and delegates to `CL_EVENT_HANDLER=>MAIN`. Results are displayed via `/HFQ/DXP_CL_MONITOR`. An authority check against `S_TCODE` with tcode `/HFQ/DXP_TCODE` is enforced at selection-screen time. `create` is an empty stub.

---

### `/HFQ/DXP_CL_MARKER` — Placeholder Class

`FINAL`. Contains a single empty static method `TODO`. Used as a placeholder; no functional code.

---

### `/HFQ/DXP_CL_UTIL` — Utility Singleton

`CREATE PRIVATE`. General-purpose utilities for the PFM framework.

**Public methods:**

```abap
CLASS-METHODS get_instance
  RETURNING VALUE(rr_instance) TYPE REF TO /hfq/dxp_cl_util.

METHODS get_components_from_struc
  IMPORTING
    !iv_struct_name TYPE /hfq/dxp_e_data_struct_name OPTIONAL
    !iv_data        TYPE any OPTIONAL
    !ir_structdescr TYPE REF TO cl_abap_structdescr OPTIONAL
      PREFERRED PARAMETER ir_structdescr
  RETURNING VALUE(rt_components) TYPE abap_component_tab
  RAISING /hfq/dxp_cx_general_error.

METHODS conv_obj_key_to_sales_doc
  IMPORTING !iv_object_key TYPE /hfq/dxp_e_object_key
  RETURNING VALUE(rv_sales_document_id) TYPE /hfq/dxp_e_sales_document_id.

METHODS conv_sales_doc_to_obj_key
  IMPORTING !iv_sales_document_id TYPE /hfq/dxp_e_sales_document_id
  RETURNING VALUE(rv_object_key) TYPE /hfq/dxp_e_object_key.

METHODS is_domain_value_valid
  IMPORTING
    !iv_value  TYPE domvalue_l
    !iv_domain TYPE domname
  RETURNING VALUE(rv_is_valid) TYPE abap_bool.

METHODS get_division_category
  IMPORTING !iv_division TYPE sparte
  RETURNING VALUE(rv_division_category) TYPE spartyp.

METHODS convert_timestamp_datetime
  IMPORTING !iv_timestamp TYPE /hfq/dxp_e_timestamp
  EXPORTING !ev_date TYPE dats  !ev_time TYPE tims
  RAISING /hfq/dxp_cx_general_error.

METHODS convert_datetime_timestamp
  IMPORTING !iv_date TYPE dats  !iv_time TYPE tims DEFAULT '000000'
  RETURNING VALUE(rv_timestamp) TYPE /hfq/dxp_e_timestamp
  RAISING /hfq/dxp_cx_general_error.

METHODS compress
  IMPORTING !iv_uncompressed TYPE string
  RETURNING VALUE(rv_compressed) TYPE xstring
  RAISING /hfq/dxp_cx_general_error.

METHODS decompress
  IMPORTING !iv_compressed TYPE xstring
  RETURNING VALUE(rv_uncompressed) TYPE string
  RAISING /hfq/dxp_cx_general_error.
```

Timestamp conversion uses `SY-ZONLO` (or falls back to `CL_ABAP_CONTEXT_INFO=>GET_USER_TIME_ZONE`). `get_components_from_struc` recursively flattens INCLUDE structures using RTTI. `compress`/`decompress` use `CL_ABAP_GZIP` with header. `is_domain_value_valid` calls `DD_DOMVALUES_GET`; returns `ABAP_TRUE` when domain has no fixed values. `conv_obj_key_to_sales_doc` extracts characters 13–22 from the 22-char object key.

---

### `/HFQ/DXP_CX_GENERAL_ERROR` — Framework Exception Class

Inherits `CX_STATIC_CHECK`. Implements `IF_T100_MESSAGE` and `IF_T100_DYN_MSG`.

**Public attributes (read-only):** `OBJ_MAIN` (STRING), `OBJ_SUB` (STRING), `EVENT_STATUS` (/HFQ/DXP_E_STATUS).

**Public methods:**

```abap
METHODS constructor
  IMPORTING
    !textid       LIKE if_t100_message=>t100key OPTIONAL
    !previous     LIKE previous OPTIONAL
    !obj_main     TYPE string OPTIONAL
    !obj_sub      TYPE string OPTIONAL
    !event_status TYPE /hfq/dxp_e_status OPTIONAL.

CLASS-METHODS raise_general
  IMPORTING
    !iv_event_status TYPE /hfq/dxp_e_status OPTIONAL
    !ir_previous     TYPE REF TO cx_root OPTIONAL
    !iv_msgid        TYPE symsgid DEFAULT sy-msgid
    !iv_msgno        TYPE symsgno DEFAULT sy-msgno
    !iv_msgv1..v4    TYPE symsgv DEFAULT sy-msgv1..v4
  RAISING /hfq/dxp_cx_general_error.
```

`RAISE_GENERAL` is the canonical way to raise this exception. It automatically captures the call position from the ABAP call stack (level 3, resolved to the class name via `CL_OO_INCLUDE_NAMING`) and stores it in `OBJ_MAIN`/`OBJ_SUB`. The `EVENT_STATUS` attribute allows callers to embed the current lifecycle status in the exception so that handlers can set the correct DB status without a second lookup.

---

### `/HFQ/DXP_CL_CHECK` (sub-package `ape`) — APE Check Class

Inherits `/APE/CL_CHECK`. Constant `GC_CR_TRANSACTION_CREATION_ERR = 'TRANSACTION_CREATION_ERROR'`.

**Public method:**

```abap
METHODS create_transaction_event
  EXPORTING !et_check_result TYPE /ape/t_check_result
  RAISING /ape/cx_exception.
```

The entire implementation is commented out. The comments show the intended logic: read APE process step data, verify that the process ended with a confirmation event code (`/US4G/IF_CONSTANTS_GPKE=>GC_INFO_CODE_ENDWITHCONFIRM`), resolve the PoD int_ui and event scenario from config, then call `CL_EVENT_HANDLER=>MAIN`. Returns check result codes `ZDSX_TRANSACTION_CREATED`, `ZDSX_TRANSACTION_DECLINED`, or `ZDSX_TRANSACTION_CREATION_ERROR`. *Not active — all code is commented out.*

---

### `/HFQ/DXP_CL_MDX_CHGTRK_GEN` (sub-package `mdx`) — MDX Change Tracking Generator

Implements `/UCOM/IF_MDX_TRACK_CHANGES`. Constructor accepts change event, change date, old/new data references, rule set, object key, and process document number/UUID.

**Interface methods (all stubs except `CHECK_TRACKING`):**
- `/UCOM/IF_MDX_TRACK_CHANGES~CHECK_TRACKING` — returns `ABAP_TRUE` unconditionally (tracking always active).
- `/UCOM/IF_MDX_TRACK_CHANGES~GET_TIME_SLICE_FIELDS` — empty stub.
- `/UCOM/IF_MDX_TRACK_CHANGES~PREPARE_CHANGED_DATA` — empty stub.
- `/UCOM/IF_MDX_TRACK_CHANGES~TRACK_CHANGES` — empty stub.

Constructor copies old/new data references into new instances via `CREATE DATA ... LIKE`. *Inferred from context: intended to integrate MDX master-data-change events into the PFM event pipeline.*

---

### `/HFQ/DXP_CL_MDX_PROCESS` (sub-package `mdx`) — MDX Process Stub

Implements `/UCOM/IF_MDX_PROCESS`. All three interface methods (`START`, `START_WITHOUT_TRIGGER`, `START_WITH_TRIGGER`) are empty stubs. *No functional implementation.*

---

### `/HFQ/DXP_CL_MDX_TRIGGER` (sub-package `mdx`) — MDX Trigger Stub

Implements `/UCOM/IF_MDX_TRIGGER`. All four interface methods (`CREATE_OUTBOUND_ENTRIES`, `FILL_BUFFER`, `GET_TRIGGERS`, `PROCESS_OUTBOUND_ENTRIES`) are empty stubs. *No functional implementation.*

---

## Interfaces

### `/HFQ/DXP_IF_BASIS_CONSTANTS` — PFM Interface with Basis Constants

Constants-only interface. Key values (all verified against the source):

| Constant | Value | Purpose |
|---|---|---|
| `GC_STATUS_NEW` | `'New'` | Initial status after event creation |
| `GC_STATUS_DATA_COLLECTED` | `'Data'` | Data providers ran successfully |
| `GC_STATUS_DATA_ERROR` | `'DataError'` | Data provider failure |
| `GC_STATUS_DATA_TRANSFORMED` | `'Conversion'` | Data converted for export |
| `GC_STATUS_DATA_TRANSFORM_ERROR` | `'ConvError'` | Conversion failure |
| `GC_STATUS_EXPORT_OK` | `'ExportOk'` | Export succeeded |
| `GC_STATUS_EXPORT_ERROR` | `'ExpError'` | Export failure |
| `GC_STATUS_CANCELED` | `'Cancelled'` | Processing cancelled |
| `GC_STATUS_COMPLETED` | `'Completed'` | Fully completed |
| `GC_STATUS_EVENT_CREATION_ERROR` | `'010'` | Event could not be created |
| `GC_OBJECT_TYPE-POD` | `'PoD'` | Point of Delivery |
| `GC_OBJECT_TYPE-SALES_DOC` | `'SDoc'` | Sales Document |
| `GC_SOURCE_MDC` | `'MDC'` | MDX master-data-change scenario |
| `GC_SOURCE_SALES` | `'Sales'` | Sales scenario |
| `GC_SOURCE_BOS` | `'BoS'` | Begin-of-Supply scenario |
| `GC_SOURCE_EOS` | `'EoS'` | End-of-Supply scenario |
| `GC_SOURCE_SOEXT` | `'SOExt'` | Supply-order extension scenario |
| `GC_EVENT_DATA_PROVISION` | `'TRIGGER_DATA_PROVISION'` | Workflow event name |
| `GC_EVENT_DATA_EXPORT` | `'TRIGGER_DATA_EXPORT'` | Workflow event name |
| `GC_TABLE_NAME_EVENT` | `'/HFQ/DXP_T_EVE'` | Event persistence table |
| `GC_CR_TRANSACTION_CREATED` | `'ZDSX_TRANSACTION_CREATED'` | APE check result |
| `GC_CR_TRANSACTION_DECLINED` | `'ZDSX_TRANSACTION_DECLINED'` | APE check result |
| `GC_CR_TRANSACTION_CREATION_ERR` | `'ZDSX_TRANSACTION_CREATION_ERROR'` | APE check result |

Several reason-code constants (for supply scenarios, contract extensions, etc.) are commented out in the source.

---

### `/HFQ/DXP_IF_DATA_CONVERTER` — Data Converter Contract

```abap
METHODS convert_data
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !iv_sub_index      TYPE /hfq/dxp_e_sub_index OPTIONAL
    !is_table_field    TYPE /hfq/dxp_s_table_field
    !iv_internal_value TYPE data
  RETURNING VALUE(rt_external_values) TYPE /hfq/dxp_tt_ext_value_data
  RAISING /hfq/dxp_cx_general_error.

METHODS set_conversion_rules
  IMPORTING !iv_set_name TYPE /hfq/dxp_e_conv_set_name
  RAISING /hfq/dxp_cx_general_error.
```

Transforms one internal field value into a table of external key-value pairs. Implementations are resolved dynamically at runtime from customizing.

---

### `/HFQ/DXP_IF_DATA_EXPORTER` — Data Exporter Contract

```abap
TYPES ty_t_transaction_id TYPE STANDARD TABLE OF /hfq/dxp_e_transaction_id.

METHODS prepare_export
  IMPORTING !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
  RAISING /hfq/dxp_cx_general_error.

METHODS commit_export
  IMPORTING
    !iv_event_source TYPE /hfq/dxp_e_event_scenario
    !iv_fullpath     TYPE string OPTIONAL
    !iv_no_dialog    TYPE abap_bool
  RAISING /hfq/dxp_cx_general_error.
```

Two-phase export: `prepare_export` stages one transaction's data; `commit_export` sends all staged data to the external system. `IV_NO_DIALOG` suppresses file-picker dialogs.

---

### `/HFQ/DXP_IF_DATA_PROVIDER` — Data Provider Contract

```abap
METHODS get_data
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !iv_sub_index      TYPE /hfq/dxp_e_sub_index OPTIONAL
  EXPORTING !er_data TYPE REF TO data
  RAISING /hfq/dxp_cx_general_error.

METHODS save_data
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !iv_sub_index      TYPE /hfq/dxp_e_sub_index OPTIONAL
    !iv_force_overwrite TYPE abap_bool DEFAULT abap_true
    !ir_data           TYPE REF TO data
  RETURNING VALUE(rv_update_done) TYPE abap_bool
  RAISING /hfq/dxp_cx_general_error.

METHODS determine_data
  IMPORTING
    !iv_transaction_id TYPE /hfq/dxp_e_transaction_id
    !iv_sub_index      TYPE /hfq/dxp_e_sub_index OPTIONAL
    !iv_int_ui_malo    TYPE /us4g/de_int_ui_malo OPTIONAL
    !iv_int_ui_melo    TYPE /us4g/de_int_ui_melo OPTIONAL
  EXPORTING !er_data TYPE REF TO data
  RAISING /hfq/dxp_cx_general_error.
```

`determine_data` fetches source data (e.g. from IS-U or SD); `save_data` persists it to the framework's intermediate store; `get_data` retrieves it. The `IV_INT_UI_MALO`/`IV_INT_UI_MELO` parameters carry the metering point relations resolved by `CL_EVENT_HANDLER`.

---

## Reports

### `/HFQ/DXP_CREATE` — Create Single Event (Development/Test)

German title: "Exporttrigger anlegen". Selection screen with object type (listbox, default `PoD`), object key, key date (default today), and scenario (listbox). Creates one event via `CL_EVENT=>CREATE`, calls `PREPARE` and `COMMIT`, then displays the result in `/HFQ/DXP_CL_MONITOR`. No authority check. *Inferred from context: intended as a developer/test tool.*

---

### `/HFQ/DXP_EVENT_CREATE` — Manual Creation of Transaction Events

Title: "Manual Creation of Transaction Events". Selection screen with installation range (`SO_INSTL`), key date (`P_KEYDAT`, default today), and scenario (`P_SCEN`, listbox). Delegates entirely to `CL_EVENT_TRIGGER=>GET_INSTANCE()->MAIN`. An authority check against `S_TCODE`/`/HFQ/DXP_TCODE` is enforced before execution. This is the user-facing manual trigger program, accessible via transaction `/HFQ/DXP_EVE_CREATE`.

**Includes:** `#HFQ#DXP_EVENT_TRIGGER_TOP` (global data, report header), `#HFQ#DXP_EVENT_TRIGGER_S1` (selection screen), `#HFQ#DXP_EVENT_TRIGGER_F1` (main logic).

---

## Tables / Data Definitions

### `/HFQ/DXP_T_EVE` — Event Persistence Table (Transparent Table)

Client-dependent, language-dependent. Application class `APPL0` (no buffering). German label: "Eventlog".

| Field group | Structure |
|---|---|
| Key | `/HFQ/DXP_S_EVENT_KEY` (CLIENT MANDT + TRANSACTION_ID RAW16) |
| Data | `/HFQ/DXP_S_EVENT_DATA` (see below) |

### `/HFQ/DXP_S_EVENT` — Internal Structure for PFM Events (Type `INTTAB`)

Fields: `TRANSACTION_ID` (/HFQ/DXP_E_TRANSACTION_ID) + all of `/HFQ/DXP_S_EVENT_DATA`.

### `/HFQ/DXP_S_EVENT_DATA` — Event Data Structure (Type `INTTAB`)

| Field | Data element | Description |
|---|---|---|
| `OBJECT_KEY` | `/HFQ/DXP_E_OBJECT_KEY` | Key of the PoD or sales doc (CHAR22, ALPHA conv. exit) |
| `OBJECT_TYPE` | `/HFQ/DXP_E_OBJECT_TYPE` | `PoD` or `SDoc` |
| `SCENARIO` | `/HFQ/DXP_E_EVENT_SCENARIO` | Source scenario (CHAR10) |
| `TIMESTAMP` | `/HFQ/DXP_E_TIMESTAMP` | Event timestamp (TZNTSTMPL) |
| `STATUS` | `/HFQ/DXP_E_STATUS` | Lifecycle status (CHAR10) |
| `RETURN_CODE` | `/HFQ/DXP_E_RETURN_CODE` | External system return code (CHAR30) |
| `RETURN_MESSAGE` | `/HFQ/DXP_E_RETURN_MESSAGE` | External system message (CHAR200) |
| `CHANGED_AT` | `ABP_LASTCHANGE_TSTMPL` | Admin: last change timestamp |
| `CHANGED_BY` | `ABP_LASTCHANGE_USER` | Admin: last changed by |
| `CREATED_AT` | `ABP_CREATION_TSTMPL` | Admin: creation timestamp |
| `CREATED_BY` | `ABP_CREATION_USER` | Admin: created by |

### `/HFQ/DXP_S_EVENT_KEY` — Event Key Structure (Type `INTTAB`)

Fields: `CLIENT` (MANDT) + `TRANSACTION_ID` (/HFQ/DXP_E_TRANSACTION_ID).

### `/HFQ/DXP_S_EXT_VALUE_DATA` — External Value Data (Type `INTTAB`)

| Field | Roll name | Description |
|---|---|---|
| `FIELD_NAME` | `NAME_FELD` | Field name (table key) |
| `EXTERNAL_VALUE` | `/HFQ/DXP_E_OUTPUT_VALUE` | Converted external value |

### Table Types

| Type | Row type | Description |
|---|---|---|
| `/HFQ/DXP_TT_EVENT` | `/HFQ/DXP_S_EVENT` | Standard table of events |
| `/HFQ/DXP_TT_EXT_VALUE_DATA` | `/HFQ/DXP_S_EXT_VALUE_DATA` | Sorted table keyed by `FIELD_NAME` |
| `/HFQ/DXP_TT_TRANSACTION_ID` | `/HFQ/DXP_E_TRANSACTION_ID` | Standard table of transaction IDs (RAW16) |

---

## Domains and Data Elements

### Domains

| Domain | Type | Length | Description | Fixed values |
|---|---|---|---|---|
| `/HFQ/DXP_D_EVENT_SOURCE` | CHAR | 10 | Source of Event | Empty (default/wildcard), `CustAddr` |
| `/HFQ/DXP_D_EXECUTION_MODE` | CHAR | 1 | Execution Mode | `A` (Asynchronous), `S` (Synchronous) |
| `/HFQ/DXP_D_MARKET_LOCATION` | NUMC | 11 | Market Location | — |
| `/HFQ/DXP_D_MESSAGE_TEXT` | CHAR | 200 | Message text (lowercase) | — |
| `/HFQ/DXP_D_OBJECT_KEY` | CHAR | 22 | Object Key (ALPHA conv. exit, lowercase) | — |
| `/HFQ/DXP_D_OBJECT_TYPE` | CHAR | 5 | Object Type | `PoD` (Point of Delivery), `SDoc` (Sales Document) |
| `/HFQ/DXP_D_RETURN_CODE` | CHAR | 30 | Return code for export | — |
| `/HFQ/DXP_D_STATUS` | CHAR | 10 | Event Status | `New`, `Data`, `DataError`, `Conversion`, `ConvError`, `ExportErr`, `Complete`, `Cancelled`, `ExportOk` |

**Belvis sub-package domain appends** (extend `/HFQ/DXP_D_EVENT_SOURCE`):

| Append domain | Values |
|---|---|
| `/HFQ/DXP_AD_CUSTOMER` | `CustCreate`, `CustChange` |
| `/HFQ/DXP_AD_CUSTOMER_ADDRESS` | `CuAdChange` |

### Data Elements

| Data element | Domain / base type | Description |
|---|---|---|
| `/HFQ/DXP_E_DATE_FROM` | DATS | Valid From Date |
| `/HFQ/DXP_E_DATE_TO` | DATS | Valid To Date |
| `/HFQ/DXP_E_EVENT_SCENARIO` | `/HFQ/DXP_D_EVENT_SOURCE` | Scenario of Event |
| `/HFQ/DXP_E_EXECUTION_MODE` | `/HFQ/DXP_D_EXECUTION_MODE` | Execution mode (A/S) |
| `/HFQ/DXP_E_EXT_VALUE_INDEX` | NUMC(3) | Running index for external value conversion |
| `/HFQ/DXP_E_KEY_DATE` | DATS | Key Date for Data Export |
| `/HFQ/DXP_E_MARKET_LOCATION` | `/HFQ/DXP_D_MARKET_LOCATION` | Market Location (MaLo) |
| `/HFQ/DXP_E_OBJECT_KEY` | `/HFQ/DXP_D_OBJECT_KEY` | Object Key |
| `/HFQ/DXP_E_OBJECT_TYPE` | `/HFQ/DXP_D_OBJECT_TYPE` | Object Type (PoD or SDoc) |
| `/HFQ/DXP_E_RETURN_CODE` | `/HFQ/DXP_D_RETURN_CODE` | External return code |
| `/HFQ/DXP_E_RETURN_MESSAGE` | `/HFQ/DXP_D_MESSAGE_TEXT` | External return message |
| `/HFQ/DXP_E_STATUS` | `/HFQ/DXP_D_STATUS` | Event lifecycle status |
| `/HFQ/DXP_E_TIMESTAMP` | `TZNTSTMPL` | Framework timestamp |
| `/HFQ/DXP_E_TRANSACTION_ID` | `GUID16` (RAW16) | UUID-based transaction ID |

---

## Other Objects

### Transaction `/HFQ/DXP_EVE_CREATE`

Calls program `/HFQ/DXP_EVENT_CREATE`, screen 1000. Description in both German and English: "Transaktion erstellen".

### Message Class `/HFQ/DXP_MC_BASIS` — "Basis Nachrichtenklasse"

15 messages (000–015), all defined in English with German translations. Representative messages:

| Nr | Text |
|---|---|
| 000 | Event data for transaction ID &1 not found. |
| 001 | Event for transaction ID &1 could not be updated. |
| 002 | UUID could not be generated. |
| 003 | New event with transaction ID &1 could not be inserted. |
| 005 | Event for transaction ID &1 could not be read. |
| 006 | No PoD found for given selection. |
| 008 | Mandatory field for installation must not be empty. |
| 009 | Mandatory field for key date must not be empty. |
| 010 | Mandatory field for source must not be empty. |
| 013 | Transaction event with transaction ID &1 successfully created. |
| 015 | Process was declined. |

### Namespace `/HFQ/`

Owner: Hochfrequenz (from namespace metadata).
