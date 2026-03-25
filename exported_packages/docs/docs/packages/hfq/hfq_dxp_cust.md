# Package: HFQ / DXP_CUST

**Description**: PFM-Framework: Customizing Package
**Original language**: English (E)
**Number of objects**: 96 files across 1 class, 1 interface, 1 function group, 1 transaction, 5 transparent tables, 3 maintenance views, 1 view cluster, 6 structures, 2 table types, 1 search help, 1 message class, 10 domains, and 24 data elements

---

## Executive Summary

HFQ/DXP_CUST is the customizing sub-package of the PFM (Process/Forecast Management) data export framework. It owns all the SAP table-maintenance infrastructure — transparent customizing tables, maintenance views, a view cluster, and a dedicated transaction (`/HFQ/DXP_CUST`) — that an administrator uses to configure the framework at runtime. The singleton class `/HFQ/DXP_CL_CONFIG_ACCESS` reads all configuration from these tables at construction time and exposes it to the rest of the framework through a typed, date-sensitive, event-source-aware API. The constants interface `/HFQ/DXP_IF_CUST_CONSTANTS` provides the canonical string literals (parameter names, interface names, execution mode flags) used as lookup keys throughout the framework, eliminating scattered magic strings. *The overall relationship between this package and the wider PFM framework (e.g., which packages own the interfaces `/HFQ/DXP_IF_DATA_PROVIDER`, `/HFQ/DXP_IF_DATA_EXPORTER`, `/HFQ/DXP_IF_DATA_CONVERTER`) is inferred from context. Not verified against implementation.*

---

## Classes

### `/HFQ/DXP_CL_CONFIG_ACCESS`

**Description**: Central, singleton configuration accessor for the PFM framework. Reads all customizing tables into internal buffers at construction time; all public methods operate on those buffers (no further database reads, except `GET_CSV_FIELDS` which lazy-loads on first access per set).

**Instantiation**: `CREATE PRIVATE` — callers must use `GET_INSTANCE`.

**Public types** (declared in the class):

| Type | Description |
|---|---|
| `TY_S_FILTERED_PARAMETER_VALUE` | Structure: `PARAMETER_INDEX` + `PARAMETER_VALUE` |
| `TY_T_FILTERED_PARAMETER_VALUE` | Standard table of the above, non-unique key on `PARAMETER_INDEX` |
| `TY_S_PARAMETER_CLASS` | Structure: `PARAMETER_INDEX` + `PARAMETER_VALUE` (typed as `SEOCLSNAME`) |
| `TY_T_PARAMETER_CLASS` | Standard table of the above |
| `TY_S_DP_CONFIG_DATA` | Structure: `TABLE_NAME` + `STRUCT_NAME` (data provider config payload) |
| `TY_S_DATA_PROVIDER_CONFIG` | Structure: `DP_CLSNAME` + includes `TY_S_DP_CONFIG_DATA` |
| `TY_T_DATA_PROVIDER_CONFIG` | Standard table of the above, keyed by `DP_CLSNAME` |
| `TY_S_EXP_FORMATTING_CONFIG` | Structure: `SET_NAME` + `DC_CLSNAME` |
| `TY_T_EXP_FORMATTING_CONFIG` | Standard table of the above, keyed by `SET_NAME` |
| `TY_S_DIRECTORY` | Structure: `PARAMETER_INDEX` + `DIRECTORY` |
| `TY_T_DIRECTORY` | Standard table of the above, empty key |

**Public instance attributes** (read-only):

| Attribute | Type | Source table |
|---|---|---|
| `GT_DATA_PROVIDER_CONFIG` | `TY_T_DATA_PROVIDER_CONFIG` | `/HFQ/DXP_C_DPC` |
| `GT_DATA_EXPORT_FORMATTING` | `TY_T_EXP_FORMATTING_CONFIG` | `/HFQ/DXP_C_DCR` |

**Public methods**:

```abap
CLASS-METHODS GET_INSTANCE
  RETURNING value(RR_INSTANCE) TYPE REF TO /HFQ/DXP_CL_CONFIG_ACCESS.
```
Returns the singleton instance; creates it on first call. Thread-safety is not addressed in the implementation.

```abap
METHODS GET_CONVERSION_RULES
  IMPORTING !IV_SET_NAME TYPE /HFQ/DXP_E_CONV_SET_NAME
  RETURNING value(RT_CONVERSION_RULES) TYPE /HFQ/DXP_TT_CONVERSION_RULE
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns all conversion rule entries for the named set from the internal buffer (keyed by `SET_NAME` in `GT_CONVERSION_RULE_SETS`). Raises if the set is not found (table-line-not-found is propagated as a general error). Source: `/HFQ/DXP_C_DCR` joined with `/HFQ/DXP_C_DCO`.

```abap
METHODS GET_CSV_FIELDS
  IMPORTING !IV_SET_NAME TYPE /HFQ/DXP_E_CONV_SET_NAME
  RETURNING value(RT_CSV_FIELDS) TYPE /HFQ/DXP_TT_CSV_FIELD
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns the ordered CSV field layout for a conversion set. Lazy-loaded per set from `/HFQ/DXP_C_DCR` left-joined with `/HFQ/DXP_C_CSV`. Recursively resolves `SUBSET_NAME` references: if a CSV entry has a non-initial `SUBSET_NAME`, the method calls itself for that subset and stores the result as a data reference in `SUBSET_FORMAT`. Raises if a duplicate serial number (column index) is detected within a set.

```abap
METHODS GET_DATA_CONVERTER
  IMPORTING !IV_EXPORT_FORMATTING TYPE /HFQ/DXP_E_CONV_SET_NAME OPTIONAL
  RETURNING value(RR_DATA_CONVERTER) TYPE REF TO /HFQ/DXP_IF_DATA_CONVERTER
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Looks up the converter class for the given set in `GT_DATA_EXPORT_FORMATTING`, validates that the class implements `/HFQ/DXP_IF_DATA_CONVERTER` (via `CL_ABAP_CLASSDESCR`), instantiates it dynamically, and calls `SET_CONVERSION_RULES` on the new instance. Raises with message 000 if the interface is not implemented; message 004 if the set is not defined.

```abap
METHODS GET_DATA_EXPORTER
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO OPTIONAL
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE OPTIONAL
  RETURNING value(RT_DATA_EXPORTER) TYPE TY_T_PARAMETER_CLASS
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Delegates to `GET_PARAMETER_VALUES` with parameter name `'DataExporter'`. The returned table maps parameter index to a class name that implements `/HFQ/DXP_IF_DATA_EXPORTER` (validated at construction time; invalid entries are deleted from the buffer).

```abap
METHODS GET_DATA_PROVIDER
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO OPTIONAL
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE OPTIONAL
  RETURNING value(RT_DATA_PROVIDER) TYPE TY_T_PARAMETER_CLASS
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Same as `GET_DATA_EXPORTER` but for parameter name `'DataProvider'` and interface `/HFQ/DXP_IF_DATA_PROVIDER`.

```abap
METHODS GET_DATA_PROVIDER_CONFIG
  IMPORTING !IV_DP_CLSNAME TYPE /HFQ/DXP_E_DATA_PROVIDER_CLS
  RETURNING value(RS_DP_CONFIG_DATA) TYPE TY_S_DP_CONFIG_DATA
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns the `TABLE_NAME` and `STRUCT_NAME` registered for a data provider class in `/HFQ/DXP_C_DPC`. Raises with message 002 if the class is not found.

```abap
METHODS GET_EXECUTION_MODE
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO OPTIONAL
            !IV_TIMESTAMP    TYPE /HFQ/DXP_E_TIMESTAMP OPTIONAL
  RETURNING value(RV_EXECUTION_MODE) TYPE /HFQ/DXP_E_EXECUTION_MODE
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Reads parameter `'ExecutionMode'` for the event source and converts the optional timestamp to a key date via `/HFQ/DXP_CL_UTIL`. The effective values (after deduplication) must be exactly one; raises with message 001 otherwise.

```abap
METHODS GET_EXPORT_FORMATTING
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO OPTIONAL
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE OPTIONAL
  RETURNING value(RV_EXPORT_FORMATTING) TYPE /HFQ/DXP_E_CONV_SET_NAME
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns the single `ExportFormatting` parameter value for the event source / date. Raises with message 001 if the value is ambiguous or absent.

```abap
METHODS GET_SOURCE_FROM_PROC_ID
  IMPORTING !IV_PROC_ID  TYPE /APE/DE_PROC_ID
            !IV_KEY_DATE TYPE /HFQ/DXP_E_KEY_DATE
  RETURNING value(RV_EVENT_SOURCE) TYPE /HFQ/DXP_E_EVENT_SCENARIO
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Resolves an APE process ID to a DXP event scenario using the date-validity mapping in `/HFQ/DXP_C_MSO`. Raises with message 007 if no valid mapping exists for the given process ID and key date.

```abap
METHODS IS_B2B_PROCESS_ACTIVE
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO OPTIONAL
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE DEFAULT SY-DATUM
  RETURNING value(RV_IS_ACTIVE) TYPE ABAP_BOOL.
```
Returns `ABAP_TRUE` if exactly one non-initial `B2BCustomerProcess` parameter value is configured for the event source. Does not raise; returns `ABAP_FALSE` on any error or ambiguity.

```abap
METHODS GET_B2B_FORECAST_PROFTYPE
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE DEFAULT SY-DATUM
  RETURNING value(RV_PROFTYPE) TYPE PROFTYPE
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns the single `B2BForecastProfType` parameter value cast to `PROFTYPE`. Raises with message 001 if not exactly one unique value.

```abap
METHODS GET_DIRECTORIES
  IMPORTING !IV_EVENT_SOURCE TYPE /HFQ/DXP_E_EVENT_SCENARIO
            !IV_KEY_DATE     TYPE /HFQ/DXP_E_KEY_DATE DEFAULT SY-DATUM
  RETURNING value(RT_DIRECTORIES) TYPE TY_T_DIRECTORY
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns one or more directory paths configured under parameter `'Directory'` for the event source. Each entry is an index-to-path pair. Raises with message 001 if no directories are defined.

```abap
METHODS GET_RFC_DESTINATION
  IMPORTING !IV_EVENT_SOURCE    TYPE /HFQ/DXP_E_EVENT_SCENARIO
            !IV_PARAMETER_NAME  TYPE /HFQ/DXP_E_PARAMETER
            !IV_KEY_DATE        TYPE /HFQ/DXP_E_KEY_DATE DEFAULT SY-DATUM
  RETURNING value(RV_RFC_DESTINATION) TYPE RFCDEST
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Returns a single RFC destination value stored under an arbitrary parameter name (the caller passes the parameter name). Raises with message 001 if not exactly one unique value.

```abap
METHODS GET_DATA_INJECTOR
  IMPORTING !IV_INJECTION_CLS TYPE /HFQ/DXP_E_DATA_INJECT_CLS OPTIONAL
  RETURNING value(RR_DATA_INJECTOR) TYPE REF TO /HFQ/DXP_IF_VALUE_GETTER
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Validates that `IV_INJECTION_CLS` implements `/HFQ/DXP_IF_VALUE_GETTER` (the `GC_INTERFACE_DATA_INJECTOR` constant) and creates an instance dynamically. Raises with message 000 if the interface check fails.

```abap
METHODS GET_PARAMETER_VALUES
  IMPORTING !IV_PARAMETER_NAME TYPE /HFQ/DXP_E_PARAMETER
            !IV_EVENT_SOURCE   TYPE /HFQ/DXP_E_EVENT_SCENARIO
            !IV_KEY_DATE       TYPE /HFQ/DXP_E_KEY_DATE
  RETURNING value(RT_PARAMETER_VALUES) TYPE TY_T_FILTERED_PARAMETER_VALUE
  RAISING /HFQ/DXP_CX_GENERAL_ERROR.
```
Core lookup: filters the internal `GT_PARAMETER_VALUES` buffer by event source, parameter name, and date validity (`DATE_FROM <= IV_KEY_DATE` AND `DATE_TO >= IV_KEY_DATE`). If the event source is non-initial but yields no results, falls back to the "empty event source" default rows automatically.

**Constructor behaviour** (private): Calls `SELECT_PARAMETER_CONFIG`, `SELECT_DATA_PROVIDER_CONFIG`, `SELECT_EXP_FORMATTING_CONFIG`, `SELECT_CONVERSION_RULE_SET`, and `SELECT_PROC_SOURCE_CONFIG` in sequence. During parameter config loading, each class value that appears as `DataProvider` or `DataExporter` parameter is validated against its respective interface via `CL_ABAP_CLASSDESCR`; rows with invalid class names are deleted from the buffer. Execution mode values are validated against domain `/HFQ/DXP_D_EXECUTION_MODE`.

---

## Interfaces

### `/HFQ/DXP_IF_CUST_CONSTANTS`

**Description**: Constants interface — "PFM Interface with Customizing Constants". Provides all string literals used as lookup keys across the framework, eliminating hard-coded strings in callers.

**Constants**:

| Constant | Value | Type | Purpose |
|---|---|---|---|
| `GC_DOMAIN_EXECUTION_MODE` | `'/HFQ/DXP_D_EXECUTION_MODE'` | `DOMNAME` | Domain name for execution mode validation |
| `GC_EVENT_SOURCE_EMPTY` | `' '` (SPACE) | `/HFQ/DXP_E_EVENT_SCENARIO` | Sentinel for "no event source" (default rows) |
| `GC_EXECUTION_MODE_ASYNCHRONOUS` | `'A'` | `/HFQ/DXP_E_EXECUTION_MODE` | Asynchronous execution mode flag |
| `GC_EXECUTION_MODE_SYNCHRONOUS` | `'S'` | `/HFQ/DXP_E_EXECUTION_MODE` | Synchronous execution mode flag |
| `GC_INTERFACE_DATA_CONVERTER` | `'/HFQ/DXP_IF_DATA_CONVERTER'` | `SEOCLSNAME` | Interface that data converter classes must implement |
| `GC_INTERFACE_DATA_EXPORTER` | `'/HFQ/DXP_IF_DATA_EXPORTER'` | `SEOCLSNAME` | Interface that data exporter classes must implement |
| `GC_INTERFACE_DATA_PROVIDER` | `'/HFQ/DXP_IF_DATA_PROVIDER'` | `SEOCLSNAME` | Interface that data provider classes must implement |
| `GC_INTERFACE_DATA_INJECTOR` | `'/HFQ/DXP_IF_VALUE_GETTER'` | `SEOCLSNAME` | Interface that data injector classes must implement |
| `GC_PARAMETER_DATA_CONVERTER` | `'DataConverter'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for data converter class |
| `GC_PARAMETER_DATA_EXPORTER` | `'DataExporter'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for data exporter class |
| `GC_PARAMETER_DATA_PROVIDER` | `'DataProvider'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for data provider class |
| `GC_PARAMETER_EXECUTION_MODE` | `'ExecutionMode'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for execution mode |
| `GC_PARAMETER_EXPORT_METHOD` | `'ExportMethod'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for export method |
| `GC_PARAMETER_EXPORT_FORMATTING` | `'ExportFormatting'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for export formatting set |
| `GC_PARAMETER_B2B_CUSTOM_PROC` | `'B2BCustomerProcess'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for B2B customer process flag |
| `GC_PARAMETER_B2B_PROFILE_TYPE` | `'B2BForecastProfType'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for B2B forecast profile type |
| `GC_PARAMETER_DIRECTOY` | `'Directory'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for file system directory (*note: typo "DIRECTOY" is in the source code*) |
| `GC_PARAMETER_B2B_FORECAST_RFC` | `'FCastRFC'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for B2B forecast RFC destination |
| `GC_PARAMETER_DP_SERIALIZED_STR` | `'DPSerialStruct'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for data provider serialization structure |
| `GC_PARAMETER_DP_SERIALIZED_CL` | `'DPSerialClass'` | `/HFQ/DXP_E_PARAMETER` | Parameter name for data provider serialization class |

---

## Function Groups

### `/HFQ/DXP_CUST`

**Description**: Standard SAP View Maintenance (SM30/SM34) function group, generated to support the three maintenance views and the table maintenance for the main customizing tables. All logic is delegated to the standard SAP View Maintenance framework (SVIMT includes); no custom business logic is present in this function group beyond the generated data access routines.

**Includes**:
- `/HFQ/LDXP_CUSTTOP` — Global declarations / function-pool header (`FUNCTION-POOL /HFQ/DXP_CUST MESSAGE-ID SV`)
- `/HFQ/LDXP_CUSTF00` — Generated view-related FORM routines for `/HFQ/DXP_V_CSV`, `/HFQ/DXP_V_DCO`, and `/HFQ/DXP_V_PVA` (GET_DATA, DB_UPD, READ_SINGLE, CORR_MAINT)
- `/HFQ/LDXP_CUSTI00` — PAI modules
- `/HFQ/LDXP_CUSTT00` — View-related data declarations
- `/HFQ/SAPLDXP_CUST` — Main include referencing standard SVIMT includes (`LSVIMFXX`, `LSVIMOXX`, `LSVIMIXX`)

**Function modules** (generated, standard SM30 framework):
- `TABLEFRAME_/HFQ/DXP_CUST` — Imports: `VIEW_ACTION`, `VIEW_NAME`, `CORR_NUMBER`; Tables: `DBA_SELLIST`, `DPL_SELLIST`, `EXCL_CUA_FUNCT`, `X_HEADER`, `X_NAMTAB`; Exception: `MISSING_CORR_NUMBER`
- `TABLEPROC_/HFQ/DXP_CUST` — Global flag set; standard view processing entry point
- `VIEWFRAME_/HFQ/DXP_V_CSV`, `VIEWFRAME_/HFQ/DXP_V_DCO`, `VIEWFRAME_/HFQ/DXP_V_PVA` — View frame function modules
- `VIEWPROC_/HFQ/DXP_V_CSV`, `VIEWPROC_/HFQ/DXP_V_DCO`, `VIEWPROC_/HFQ/DXP_V_PVA` — View processing function modules

**Screen programs**: Screens 0001–0007 (standard generated SM30/SM34 dialog screens).

---

## Tables / Data Definitions

### Transparent Customizing Tables

#### `/HFQ/DXP_C_PAR` — Parameter Customizing for PFM Framework

Client-dependent, language-dependent, transport class `C` (customizing). Key structure includes `/HFQ/DXP_S_PARAMETER_KEY` (event source + parameter name + date from). Additional non-key field: `DATE_TO`.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| EVENT_SOURCE (via include) | `/HFQ/DXP_E_EVENT_SCENARIO` | Event scenario / trigger source |
| PARAMETER_NAME (via include) | `/HFQ/DXP_E_PARAMETER` | Parameter name (CHAR 20) |
| DATE_FROM (via include) | `/HFQ/DXP_E_DATE_FROM` | Validity start date |
| DATE_TO | `/HFQ/DXP_E_DATE_TO` | Validity end date |

This is the header table. Parameter values are stored in `/HFQ/DXP_C_PVA`.

#### `/HFQ/DXP_C_PVA` — Parameter Value Customizing for PFM Framework

Client-dependent, language-dependent, transport class `C`. Key: `EVENT_SOURCE` + `PARAMETER_NAME` + `DATE_FROM` + `PARAMETER_INDEX`.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| EVENT_SOURCE (via include) | `/HFQ/DXP_E_EVENT_SCENARIO` | Event scenario |
| PARAMETER_NAME (via include) | `/HFQ/DXP_E_PARAMETER` | Parameter name |
| DATE_FROM (via include) | `/HFQ/DXP_E_DATE_FROM` | Validity start date |
| PARAMETER_INDEX | `/HFQ/DXP_E_PARAMETER_INDEX` | Running index (NUMC 3), allows multiple values per parameter |
| PARAMETER_VALUE | `/HFQ/DXP_E_PARAMETER_VALUE` | Value string (CHAR 50, lowercase) |

#### `/HFQ/DXP_C_DPC` — Customizing for PFM Data Providers

Client-dependent, transport class `C`. Key: `DP_CLSNAME`.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| DP_CLSNAME | `/HFQ/DXP_E_DATA_PROVIDER_CLS` | Data provider class name (CHAR 30); entity table search help via `/HFQ/DXP_C_DPC` |
| TABLE_NAME | `TABNAME16` | Associated database table name |
| STRUCT_NAME | `/HFQ/DXP_E_DATA_STRUCT_NAME` | Associated ABAP structure name |

#### `/HFQ/DXP_C_DCR` — Set for Conversion Rules for PFM Framework

Client-dependent, transport class `C`. Key: `SET_NAME`. This is the header table for conversion rule sets; detail rows are in `/HFQ/DXP_C_DCO`.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| SET_NAME | `/HFQ/DXP_E_CONV_SET_NAME` | Conversion set name (TEXT20, search help `/HFQ/DXP_SH_CONV_SET`) |
| CONV_CLSNAME | `/HFQ/DXP_E_DATA_CONVERT_CLS` | Data converter class name (based on `SEOCLSNAME`) |
| SET_DESCRIPTION | `/HFQ/DXP_E_SET_DESCRIPTION` | Free-text description (TEXT30) |

#### `/HFQ/DXP_C_DCO` — Customizing for PFM Data Field Value Conversion

Client-dependent, language-dependent, transport class `C`. Key: `SET_NAME` + `SERIAL_NUMBER`. Contains the actual conversion rule rows (internal-to-external value mappings) for a set.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| SET_NAME | `/HFQ/DXP_E_CONV_SET_NAME` | Conversion set reference |
| SERIAL_NUMBER | `/HFQ/DXP_E_SERIAL_NUMBER` | Ordering index (NUMC 3) |
| TABLE_NAME (via include) | `/HFQ/DXP_E_DATA_TABLE_NAME` | Source table or path (CHAR 255) |
| FIELD_NAME (via include) | `NAME_FELD` | Source field name |
| INTERNAL_VALUE | `/HFQ/DXP_E_INPUT_VALUE` | Internal (SAP) value (CHAR 50) |
| EXTERNAL_VALUE | `/HFQ/DXP_E_OUTPUT_VALUE` | External (output) value (CHAR 50) |
| CONVERSION_METHOD | `/HFQ/DXP_E_DATA_CONV_METHOD` | Conversion method name (CHAR 61) |

#### `/HFQ/DXP_C_CSV` — Customizing CSV Field Order

Client-dependent, language-dependent, transport class `C`. Key: `SET_NAME` + `SERIAL_NUMBER`. Defines which source table fields map to which target CSV columns, and in which order. Change logging enabled (`PROTOKOLL`).

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| SET_NAME | `/HFQ/DXP_E_CONV_SET_NAME` | Conversion set reference |
| SERIAL_NUMBER | `/HFQ/DXP_E_SERIAL_NUMBER` | Column ordering index (NUMC 3) |
| TABLE_NAME (via include) | `/HFQ/DXP_E_DATA_TABLE_NAME` | Source table or path |
| FIELD_NAME (via include) | `NAME_FELD` | Source field name |
| TARGET_FIELD | `/HFQ/DXP_E_TARGET_FIELD` | CSV column header or JSON field name (CHAR 60, lowercase) |
| SUBSET_NAME | `/HFQ/DXP_E_CONV_SUBSET_NAME` | Reference to a sub-set of rules (TEXT20) for nested structures |
| OUTPUT_TYPE | `/HFQ/DXP_E_OUTPUT_TYPE` | Output type fixed values: empty / `N` / `T` / `S` |
| INJECTOR | `/HFQ/DXP_E_DATA_INJECT_CLS` | Class name implementing `/HFQ/DXP_IF_VALUE_GETTER` for injected values |

#### `/HFQ/DXP_C_MSO` — Customizing for Mapping of Process ID to Source

Client-dependent, transport class `C`. Key: `PROC_ID` + `DATE_FROM`. Maps APE process IDs to DXP event scenarios with date validity.

| Field | Type / Data element | Description |
|---|---|---|
| CLIENT | MANDT | Client |
| PROC_ID | `/APE/DE_PROC_ID` | APE process ID (search help from domain) |
| DATE_FROM | `/HFQ/DXP_E_DATE_FROM` | Validity start date |
| DATE_TO | `/HFQ/DXP_E_DATE_TO` | Validity end date |
| SCENARIO | `/HFQ/DXP_E_EVENT_SCENARIO` | DXP event scenario (fixed-value validated) |

### Structures (INTTAB)

| Structure | Description | Fields |
|---|---|---|
| `/HFQ/DXP_S_PARAMETER_KEY` | Key fields for parameter customizing | `EVENT_SOURCE`, `PARAMETER_NAME`, `DATE_FROM` — included in `/HFQ/DXP_C_PAR` and `/HFQ/DXP_C_PVA` |
| `/HFQ/DXP_S_TABLE_FIELD` | Compound of table name and field name | `TABLE_NAME` (`/HFQ/DXP_E_DATA_TABLE_NAME`), `FIELD_NAME` (`NAME_FELD`) — included in `/HFQ/DXP_C_DCO` and `/HFQ/DXP_C_CSV` |
| `/HFQ/DXP_S_CONVERSION_RULE` | Single conversion rule (internal to external) | Includes `S_TABLE_FIELD` + `INTERNAL_VALUE` + `EXTERNAL_VALUE` + `CONVERSION_METHOD` |
| `/HFQ/DXP_S_EXPORT_FORMAT` | Export format field descriptor | `SET_NAME`, `SERIAL_NUMBER`, includes `S_TABLE_FIELD`, `TARGET_FIELD`, `IS_TABLE` (XFELD checkbox), `INJECTOR`, `SUBSET_NAME`, `SUBSET_FORMAT` (REF TO DATA) |
| `/HFQ/DXP_S_WID_KEY` | Key structure for weather station table | `CLIENT` (MANDT), `POST_CODE` (`/HFQ/DXP_E_POSTCODE_WST_ID`, NUMC 2) |
| `/HFQ/DXP_S_WID_DATA` | Data structure for weather station table | `WEATHER_STATION` (`/HFQ/DXP_E_WEATHER_STATION`, CHAR 20) |

### Table Types

| Type | Row type | Key | Description |
|---|---|---|---|
| `/HFQ/DXP_TT_CONVERSION_RULE` | `/HFQ/DXP_S_CONVERSION_RULE` | Primary: `TABLE_NAME`, `FIELD_NAME`, `INTERNAL_VALUE`; secondary sorted key on `FIELD_NAME` | Standard sorted table of conversion rules |
| `/HFQ/DXP_TT_CSV_FIELD` | `/HFQ/DXP_S_EXPORT_FORMAT` | Unique primary key: `SET_NAME`, `SERIAL_NUMBER` | Standard sorted table of CSV field definitions |

### Maintenance Views

| View | Root table | Description |
|---|---|---|
| `/HFQ/DXP_V_PVA` | `/HFQ/DXP_C_PVA` | Maintenance View for Parameter Value Configuration in PFM. Key fields `EVENT_SOURCE`, `PARAMETER_NAME`, `DATE_FROM` are read-only in the view (set from the parent table in the view cluster). |
| `/HFQ/DXP_V_DCO` | `/HFQ/DXP_C_DCO` | Maintenance View for Parameter Value Configuration in PFM (data field value conversion). `SET_NAME` is read-only, search help `/HFQ/DXP_SH_CONV_SET`. |
| `/HFQ/DXP_V_CSV` | `/HFQ/DXP_C_CSV` | Maintenance View for CSV Customizing. `SET_NAME` is read-only, search help `/HFQ/DXP_SH_CONV_SET`. Includes `OUTPUT_TYPE` with fixed-value check. |

### View Cluster

**`/HFQ/DXP_VC_CUST`** — Bundles all customizing tables into a single SM34 view cluster exposed via transaction `/HFQ/DXP_CUST`:

| Level | Object | Predecessor | Dependency type |
|---|---|---|---|
| 1 | `/HFQ/DXP_C_PAR` | self | Root (R) |
| 2 | `/HFQ/DXP_V_PVA` | `/HFQ/DXP_C_PAR` | Sub-object (S), cardinality X — linked via `EVENT_SOURCE`, `PARAMETER_NAME`, `DATE_FROM` |
| 1 | `/HFQ/DXP_C_DPC` | self | Root (R) |
| 1 | `/HFQ/DXP_C_DCR` | self | Root (R) |
| 2 | `/HFQ/DXP_V_DCO` | `/HFQ/DXP_C_DCR` | Sub-object (S), cardinality X — linked via `SET_NAME` |
| 2 | `/HFQ/DXP_V_CSV` | `/HFQ/DXP_C_DCR` | Sub-object (S), cardinality X — linked via `SET_NAME` |

### Search Help

**`/HFQ/DXP_SH_CONV_SET`** — Simple search help for conversion set names. Selection method: `/HFQ/DXP_C_DCR` (table). Displays: `SET_NAME`, `CONV_CLSNAME`, `SET_DESCRIPTION`. Referenced by data element `/HFQ/DXP_E_CONV_SET_NAME`.

---

## Other Objects

### Transaction

**`/HFQ/DXP_CUST`** — "Customizing for PFM Interface" (German: "Framework Einstellungen (PFM)"). Calls SM34 with view cluster `/HFQ/DXP_VC_CUST` in update mode (`UPDATE=X`). Supported on Win32 and SAP GUI Platinum; WebGUI not enabled.

### Message Class

**`/HFQ/DXP_MC_CUST`** — "Customizing Messages (PFM)". Master language: English. Contains 9 messages (000–008) used by `/HFQ/DXP_CL_CONFIG_ACCESS`:

| Nr | Text |
|---|---|
| 000 | Class &1 does not implement Interface &2. |
| 001 | Value for Parameter &1 not unique. |
| 002 | Configuration for Class &1 not found. |
| 003 | &1 is not a valid Value for Domain &2. |
| 004 | Set &1 is not defined. |
| 005 | No Sets defined. |
| 006 | Column index &1 in CSV customizing for conv. rule set &2 is not unique. |
| 007 | Source for Proc.Id. &1 and date &2 not found. |
| 008 | Reason for Proc.Id. &1 and Parameter &2 not found. |

### Namespace Object

**`/HFQ/.nspc.xml`** — Namespace registration file for the `/HFQ/` namespace prefix.

### AVAS Entries

Four `.avas.xml` files (object attribute value assignments) are present in the source directory. Their contents relate to object tagging/classification within the SAP system. *Their specific attribute assignments are not documented here as they do not affect runtime behaviour.*

---

## Domains and Data Elements

### Domains

| Domain | Type | Length | Description / Notes |
|---|---|---|---|
| `/HFQ/DXP_D_COLUMN_INDEX` | NUMC | 3 | Column index for CSV field ordering |
| `/HFQ/DXP_D_CONV_SET_NAME` | CHAR | 20 | Conversion set name (lowercase) |
| `/HFQ/DXP_D_DATA_PROVIDER_CLS` | CHAR | 30 | Data provider class name; entity table `/HFQ/DXP_C_DPC` |
| `/HFQ/DXP_D_DATA_TABLE_NAME` | CHAR | 255 | Table name or path |
| `/HFQ/DXP_D_OUTPUT_TYPE` | CHAR | 1 | Fixed values: ` ` (empty), `N`, `T`, `S` |
| `/HFQ/DXP_D_PARAMETER` | CHAR | 20 | Parameter name (lowercase) |
| `/HFQ/DXP_D_PARAMETER_VALUE` | CHAR | 50 | Parameter value (lowercase) |
| `/HFQ/DXP_D_SERIAL_NUMBER` | NUMC | 3 | Serial number / ordering index |
| `/HFQ/DXP_D_TARGET_FIELD` | CHAR | 60 | CSV column header or JSON target field name (lowercase) |
| `/HFQ/DXP_D_WEATHER_STATION` | CHAR | 20 | Weather station identifier (lowercase) |

### Data Elements

| Data Element | Domain | Description |
|---|---|---|
| `/HFQ/DXP_E_CB_TABLE` | `XFELD` | Checkbox: conversion subset is table (for JSON customizing) |
| `/HFQ/DXP_E_COLUMN_INDEX` | `/HFQ/DXP_D_COLUMN_INDEX` | Column index of a field inside a CSV |
| `/HFQ/DXP_E_CONV_SET_NAME` | `TEXT20` | Set of conversion rules; search help `/HFQ/DXP_SH_CONV_SET` attached |
| `/HFQ/DXP_E_CONV_SUBSET_NAME` | `TEXT20` | Subset of conversion rules for nested structures |
| `/HFQ/DXP_E_DATA_CONV_METHOD` | `CHAR61` | Data conversion method name |
| `/HFQ/DXP_E_DATA_CONVERT_CLS` | `SEOCLSNAME` | Class name of the data converter |
| `/HFQ/DXP_E_DATA_EXPORTER_CLS` | `SEOCLSNAME` | Class name of the data exporter |
| `/HFQ/DXP_E_DATA_INJECT_CLS` | `SEOCLSNAME` | Class name of the data injector (implements `/HFQ/DXP_IF_VALUE_GETTER`) |
| `/HFQ/DXP_E_DATA_PROVIDER_CLS` | `/HFQ/DXP_D_DATA_PROVIDER_CLS` | Class name of the data provider |
| `/HFQ/DXP_E_DATA_STRUCT_NAME` | `AS4TAB` | ABAP structure name for the data provider output |
| `/HFQ/DXP_E_DATA_TABLE_NAME` | `/HFQ/DXP_D_DATA_TABLE_NAME` | Database table name or path for data provider |
| `/HFQ/DXP_E_INPUT_VALUE` | `/HFQ/DXP_D_PARAMETER_VALUE` | Internal (SAP-side) value for conversion |
| `/HFQ/DXP_E_OUTPUT_TYPE` | `/HFQ/DXP_D_OUTPUT_TYPE` | Output type for CSV/JSON field |
| `/HFQ/DXP_E_OUTPUT_VALUE` | `/HFQ/DXP_D_PARAMETER_VALUE` | External (output-side) value for conversion |
| `/HFQ/DXP_E_PARAMETER` | `/HFQ/DXP_D_PARAMETER` | Parameter name for the framework |
| `/HFQ/DXP_E_PARAMETER_INDEX` | — (inline NUMC 3) | Running index for multi-valued parameters |
| `/HFQ/DXP_E_PARAMETER_VALUE` | `/HFQ/DXP_D_PARAMETER_VALUE` | Parameter value string |
| `/HFQ/DXP_E_POSTCODE_WST_ID` | — (inline NUMC 2) | Postal code prefix for weather station mapping |
| `/HFQ/DXP_E_SERIAL_NUMBER` | `/HFQ/DXP_D_SERIAL_NUMBER` | Serial/ordering number for CSV fields |
| `/HFQ/DXP_E_SET_DESCRIPTION` | `TEXT30` | Description for a conversion rule set |
| `/HFQ/DXP_E_TARGET_FIELD` | `/HFQ/DXP_D_TARGET_FIELD` | CSV header name or JSON target field name |
| `/HFQ/DXP_E_WEATHER_STATION` | `/HFQ/DXP_D_WEATHER_STATION` | Weather station name |
| `/HFQ/DXP_E_CONV_SET_NAME` | `TEXT20` | (listed separately from domain-based counterpart; uses standard `TEXT20` domain and attaches search help `/HFQ/DXP_SH_CONV_SET` directly on the data element) |
| `/HFQ/DXP_E_CONV_SUBSET_NAME` | `TEXT20` | Subset name reference within a conversion set |

*Note: Data elements `/HFQ/DXP_E_DATE_FROM`, `/HFQ/DXP_E_DATE_TO`, `/HFQ/DXP_E_EVENT_SCENARIO`, `/HFQ/DXP_E_EXECUTION_MODE`, `/HFQ/DXP_E_KEY_DATE`, `/HFQ/DXP_E_TIMESTAMP` are referenced in the class and table definitions but are not defined in this package. Inferred to belong to a base/shared DXP package.*
