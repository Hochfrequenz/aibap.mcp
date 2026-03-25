# Package: HFQ / DXP_EXPORT

**Description**: PFM-Framework: Export Functionalities
**Original language**: English (E)
**Number of objects**: 22 (2 interfaces, 9 classes + 1 behavior implementation class, 1 behavior definition, 2 CDS views, 2 transparent tables, 1 structure, 1 table type, 1 data element, 1 message class, 1 event binding)

---

## Executive Summary

`HFQ/DXP_EXPORT` is the export sub-package of the PFM (presumably "Prozessframework"/"Process Framework for Market") data export framework. It provides a layered architecture for transforming SAP-internal field values into external representations and writing them to CSV files or server-side directories.

The package is structured around three responsibilities:

1. **Data conversion** (`DC` prefix): Converting internal SAP field values (business partners, addresses, service providers, dates, settlement units, territories) into externally meaningful string representations. The abstract base (`CL_DC_ABSTRACT`) applies configurable conversion rules; `CL_DC_BELVIS` is the concrete Belvis-specific converter. `CL_DC_CONTROLLER` is the singleton orchestrator that persists converted values in `T_IEC`.

2. **Data export** (`DE` prefix): A class hierarchy builds CSV content from transformed data. `CL_DE_ABSTRACT` handles data gathering, conversion invocation, and value-getter injection. `CL_DE_CSV_BASIC` writes one or more CSV files interactively to the user's local machine. `CL_DE_CSV_TO_DIR` writes to server-side directories. `CL_DE_DIR_1DATASET` extends that with one file per transaction/dataset. `CL_DE_SERIALIZED` is a parallel track that operates on serialized data rather than structured tables and saves converted output back into the serialized store.

3. **BTP export path** (RAP/event-driven): A Root View Entity (`R_EXPORT_COMPRESSED`) with a Behavior Definition exposes compressed export data and fires an `exportData` event carrying decompressed content and API endpoint parameters toward SAP BTP. The event binding `(hfq)dxp_evb_export_data` declares the producer/operation contract.

The central orchestration entry point is `CL_EXPORT_HANDLER` (singleton, private constructor), which resolves the correct exporter class(es) from configuration for a given event source and key date, calls `prepare_export` for each transaction, and then `commit_export` while tracking success/failure status back to the event table.

---

## Classes

### `/HFQ/DXP_CL_EXPORT_HANDLER`
**Description**: Export Handler for PFM Framework
**Visibility**: Public, `CREATE PRIVATE` (singleton via `GET_INSTANCE`)
**Superclass**: none
**Interfaces**: none

The top-level orchestrator. Maintains an internal sorted table of exporter instances keyed by `(source, class_name)`. On `PREPARE_EXPORT` it reads event data, resolves exporter class names from config access, instantiates each unique exporter once, calls `prepare_export` on it, and appends the transaction ID. On `COMMIT_EXPORT` it iterates all instances, calls `commit_export`, and writes back `gc_status_export_ok` or `gc_status_export_error` to the event DB table. `REFRESH` clears the singleton.

| Method | Visibility | Description |
|---|---|---|
| `GET_INSTANCE` | Public (class) | Returns singleton instance |
| `PREPARE_EXPORT` | Public | Resolves and calls exporter for a transaction |
| `COMMIT_EXPORT` | Public | Triggers all exporters; updates event status |
| `REFRESH` | Public (class) | Clears singleton |

---

### `/HFQ/DXP_CL_DC_ABSTRACT`
**Description**: Abstract Data Converter for PFM
**Visibility**: Public, Abstract
**Superclass**: `/HFQ/DXP_CL_DYN_OBJECT` *(from a different package)*
**Interfaces**: `/HFQ/DXP_IF_DATA_CONVERTER` (methods `CONVERT_DATA` and `SET_CONVERSION_RULES` are final)
**Message class**: `/HFQ/DXP_MC_EXP`

Implements the `CONVERT_DATA` protocol: first attempts a direct static mapping (`internal_value` → `external_value`) from the loaded conversion rules; if no direct match, falls back to a dynamically dispatched conversion method name stored in the rule. `SET_CONVERSION_RULES` loads rules from `CL_CONFIG_ACCESS` by set name.

Protected helper `ADD_SUFFIX` appends a `_<suffix>` to a field name, raising an error if the result exceeds 30 characters.

| Member | Type | Description |
|---|---|---|
| `GT_CONVERSION_RULES` | Private data | Loaded conversion rules (`/HFQ/DXP_TT_CONVERSION_RULE`) |
| `ADD_SUFFIX` | Protected method | Append suffix to field name (max 30 chars) |

---

### `/HFQ/DXP_CL_DC_BELVIS`
**Description**: Data Converter Class for Belvis
**Visibility**: Public
**Superclass**: `/HFQ/DXP_CL_DC_ABSTRACT`
**Message class**: `/HFQ/DXP_MC_EXP`

Provides the concrete conversion methods that the parent's dynamic dispatch mechanism invokes by name from conversion rules. Each method has the same signature: imports `IV_TRANSACTION_ID`, `IV_SUB_INDEX`, `IV_FIELD_NAME`, `IV_INTERNAL_VALUE`; exports `ET_EXTERNAL_VALUES` of type `TY_T_EXT_VALUE`; raises `/HFQ/DXP_CX_GENERAL_ERROR`.

| Method | Behaviour |
|---|---|
| `CONVERT_BUSINESS_PARTNER` | Calls `BUP_PARTNER_GET`; maps person (first+last name) or org name to `CUSTOMER_NAME` |
| `CONVERT_ADDRESS` | Calls `ADDR_SELECT_ADRC_SINGLE`; maps post code, city, district, street, house number, country |
| `CONVERT_SERVICE_PROV` | Selects from `ESERVPROV JOIN ESPEXTIDTYPE`; emits external ID + code list ID (with `TYPE` suffix) |
| `CONVERT_DATE` | Formats `DATS` as ISO date; maps `99991231` to literal `'infinite'` |
| `CONVERT_CONTR_DATE` | Delegates to `CONVERT_DATE`, then conditionally appends `PERIOD_START_DATE` / `PERIOD_END_DATE` based on scenario (BOS/EOS) and procurement segment/metering procedure flags |
| `CONVERT_SETTL_DATE` | Like `CONVERT_CONTR_DATE` but with EOS-specific +1-day logic for period start |
| `CONVERT_SETTLUNIT` | Selects external balancing unit from `EEDMSETTLUNIT`; for gas adds `gas_quality` (L/H from balancing unit code character 7) |
| `CONVERT_SETTLTERRITORY` | For gas: maps to fixed gas market area constant. For electricity: selects from `/US4G/S_ST`, emits settlement territory external ID and control area |
| `CONVERT_INT_UI` | Translates internal usage point to external UI via `EUITRANS` (skipped for sales scenario) |
| `CONVERT_SALES_DOC` | Only for sales scenario; emits external UI from event object key, address fields from `/HFQ/DXP_T_DSO`, and dummy gas quality |
| `CONVERT_USAGE_FACTOR` | Casts `USEFACTOR` and emits left-aligned string |
| `GET_GENERAL_DATA` (private) | Instantiates `CL_DP_GENERAL` and retrieves first row |
| `GET_BILLING_DATA` (private) | Instantiates `CL_DP_BILLING` and retrieves first row |
| `GET_TECHNICAL_DATA` (private) | Instantiates `CL_DP_TECHNICAL` and retrieves first row |
| `GET_SETTLEMENT_DATA` (private) | Instantiates `CL_DP_SETTLEMENT` and retrieves first row |
| `DETERMINE_ADDRESS_SD` (private) | Reads address columns from `/HFQ/DXP_T_DSO` for a given sales document |

---

### `/HFQ/DXP_CL_DC_CONTROLLER`
**Description**: DB-Class for Converted Data in PFM
**Visibility**: Public, `CREATE PROTECTED` (singleton via `GET_INSTANCE`)
**Message class**: `/HFQ/DXP_MC_EXP`

Singleton controller that bridges data conversion and database persistence of converted values. `CONVERT_DATA` iterates the `CT_DATA` table, invokes the configured data converter, and both appends the converted records to `CT_DATA` and persists them to `T_IEC` via `SAVE_DATA`. `GET_DATA` selects persisted external values from `T_IEC`. `SAVE_DATA` uses `MODIFY` (force overwrite) or `INSERT` with duplicate-key handling.

| Method | Visibility | Description |
|---|---|---|
| `GET_INSTANCE` | Public (class) | Returns singleton, sets `GV_SAVE_DATA` flag |
| `GET_DATA` | Public | SELECT from `/HFQ/DXP_T_IEC` by transaction/sub-index |
| `SAVE_DATA` | Public | Writes external values to `/HFQ/DXP_T_IEC` |
| `CONVERT_DATA` | Public | Orchestrates conversion and saving for a data table |

---

### `/HFQ/DXP_CL_DE_ABSTRACT`
**Description**: Basic Data Export Class for CSV
**Visibility**: Public, Abstract
**Superclass**: `/HFQ/DXP_CL_DYN_OBJECT`
**Interfaces**: `/HFQ/DXP_IF_DATA_EXPORTER` (`COMMIT_EXPORT` abstract)
**Message class**: `/HFQ/DXP_MC_EXP`

The core engine of the export pipeline. `PREPARE_EXPORT` (interface implementation) orchestrates three steps: `FILL_TRANSFORMED_DATA` → `INJECT_DATA` → `PREPARE_EXPORT_CHILD` (abstract, implemented by concrete subclasses).

`FILL_TRANSFORMED_DATA` reads config access to determine data providers and export formatting for the event's scenario/key-date, then for each provider instantiates it, retrieves data, and calls `CL_DC_CONTROLLER` to convert each row. Results are collected in `GT_TRANSFORMED_DATA` (sorted by `transaction_id`, `sub_index`, keyed also by `conv_set`).

`INJECT_DATA` walks `GT_TRANSFORMED_DATA` grouped by `conv_set`, finds CSV field customizing entries that carry an injector class name, and calls `INJECT_GETTER` for each. `INJECT_GETTER` instantiates the injector, checks relevance, calls `SET`, and stores a reference to the injector as a value-getter in the export data table.

`GET_FIELD_VALUE_AS_STRING` handles both plain ABAP elementary values (string cast) and references to `/HFQ/DXP_IF_VALUE_GETTER` (calls `GET`).

Protected types: `TY_S_TRANSFORMED_DATA` (transaction_id, sub_index, conv_set, export_data table), `TY_T_TRANSFORMED_DATA` (sorted table).

---

### `/HFQ/DXP_CL_DE_CSV_BASIC`
**Description**: Basic Data Export Class for CSV
**Visibility**: Public
**Superclass**: `/HFQ/DXP_CL_DE_ABSTRACT`
**Message class**: `/HFQ/DXP_MC_EXP`

Interactive CSV export to the SAP GUI front end. `PREPARE_EXPORT_CHILD` (redefinition) iterates `GT_TRANSFORMED_DATA` grouped by `conv_set`, builds a header row from `PREPARE_CSV_HEADER`, then builds data rows by matching `GT_TRANSFORMED_DATA` against CSV field customizing ordered by serial number. Empty column gaps are filled. Results are accumulated in `GT_CSV` (one entry per `conv_set`).

`COMMIT_EXPORT` (interface redefinition) shows a file-save dialog (`CL_GUI_FRONTEND_SERVICES`), warns if more than one CSV will be created, and downloads each non-empty CSV with a counter suffix using `GUI_DOWNLOAD`. Raises `gc_status_data_transformed` (not an error) when `IV_NO_DIALOG = TRUE`.

`PREPARE_CSV_HEADER` builds the semicolon-delimited header string from the CSV field customizing, inserting empty columns for gaps in serial number sequence.

---

### `/HFQ/DXP_CL_DE_CSV_TO_DIR`
**Description**: Automized CSV Export to directory
**Visibility**: Public
**Superclass**: `/HFQ/DXP_CL_DE_CSV_BASIC`
**Message class**: `/HFQ/DXP_MC_EXP`

Server-side (background-compatible) CSV export. `PREPARE_EXPORT_CHILD` (redefinition) builds CSV lines per conv_set/directory combination. Each transaction's target directories are resolved from config access via `GET_DIRECTORIES`. Results accumulate in `GT_CSV_FILES` (sorted by `directory`, `conv_set`).

`COMMIT_EXPORT` opens each file path via `OPEN DATASET … FOR OUTPUT IN TEXT MODE`, writes all lines with `TRANSFER`, and closes. The file path is `<directory>/<timestamp><uuid>.csv` from `GENERATE_FILENAME`.

`BUILD_CSV_LINE` produces a single semicolon-delimited CSV line for one export data set.

---

### `/HFQ/DXP_CL_DE_DIR_1DATASET`
**Description**: Automized CSV Export to Directory with CSV for every dataset
**Visibility**: Public
**Superclass**: `/HFQ/DXP_CL_DE_CSV_TO_DIR`

Variant of `CL_DE_CSV_TO_DIR` that writes one file per transaction/dataset instead of one file per conversion set. `PREPARE_EXPORT_CHILD` first calls `FILTER_TRANSFORMED_DATA` to keep at most one sub-index per transaction ID (preferring the row matching the event's object key for PoD-type objects). `COMMIT_EXPORT` calls `CONVERT_CSVS_TO_LINEWISE_CSVS` which explodes each multi-row CSV in `GT_CSV_FILES` into individual one-data-row CSVs, then writes them.

---

### `/HFQ/DXP_CL_DE_SERIALIZED`
**Description**: Export class for serialized data (German: "Export Klasse für serialisierte Daten")
**Visibility**: Public
**Superclass**: none
**Interfaces**: `/HFQ/DXP_IF_DATA_EXPORTER`
**Message class**: `/HFQ/DXP_MC_EXP`

A parallel export path that operates on the serialized data layer (`CL_DP_SERIALIZED`) rather than structured provider tables. `PREPARE_EXPORT`: reads serialized internal data, determines applicable conversion/export formatting from config, optionally filters via overridable `FILTER_EXPORT_FORMATTING`, converts each serialized field for each conversion set (prefixing the path with `<path>@<conv_set>`), saves converted data back via `CL_DP_SERIALIZER`, then recursively injects value-getters into the work area. Child classes override `PREPARE_EXPORT_CHILD` to consume the resulting `cs_work_area`. `COMMIT_EXPORT` is empty (no-op base implementation).

Protected work area type `TY_S_WORKAREA` bundles: `transaction_id`, `conversion_sets`, `int_data`, `ext_data` (keyed by conversion set), `injectors`.

---

### `/HFQ/DXP_BP_R_EXPORT_COMPRESSE`
**Description**: Behavior Implementation for `/HFQ/DXP_R_EXPORT_COMPRESSED`
**Category**: Behavior implementation class (CATEGORY 06), Abstract, Final
**Linked behavior definition**: `/HFQ/DXP_R_EXPORT_COMPRESSED`

The behavior implementation shell. The public class body is empty. The local saver class `lsc_DXP_R_EXPORT_COMPRESSED` (inheriting `CL_ABAP_BEHAVIOR_SAVER`) implements `save_modified`: for each created `COMPRESSED` entity, it calls `CL_UTIL=>decompress` on `compressedData`, raises the `exportData` entity event with decompressed content and a placeholder API endpoint parameter (TODO marker present), and sets status to `gc_status_export_ok` or `gc_status_export_error`. The update to the event DB table is commented out.

The local handler `lhc_COMPRESSED` provides `GET_INSTANCE_AUTHORIZATIONS` with an empty implementation.

---

## Interfaces

### `/HFQ/DXP_IF_EXP_CONSTANTS`
**Description**: PFM Interface with Basis Constants
**Exposure**: Public (2)

A constant container used throughout the export package. Key constants:

| Constant | Value | Purpose |
|---|---|---|
| `GC_TABLE_NAME_EXT_VALUE` | `/HFQ/DXP_T_IEC` | Target table for converted external values |
| `GC_ELEMENT_EXTERNAL_VALUE` | `/HFQ/DXP_E_OUTPUT_VALUE` | Data element for external value fields |
| `GC_CSV_FIELD_SEPARATOR` | `;` | CSV column delimiter |
| `GC_FIELD_NAME_MAX_LENGTH` | 30 | Maximum field name length |
| `GC_TABLE_NAME_DATA` | general/billing/sales_order/settlement/technical → table names | Data table name constants |
| `GC_INJECTION` | `*INJECTION*` | Wildcard pattern for injection |
| `GC_STRING_PATTERN_START` / `GC_STRING_PATTERN_END` | `*START*` / `*END*` | Field name pattern matching for period dates |
| `GC_GAS_QUALITY_LOW` | `L-Gas` | Gas quality identifier |
| `GC_FIELD_NAMES` | structure of 30 field-name constants | Canonical output column names (address, UI, dates, etc.) |
| `GC_FIELD_NAME_SUFFIXES` | `TYPE` | Standard type suffix |

---

### `/HFQ/DXP_IF_VALUE_GETTER`
**Description**: Getter Interface for Output Values ("Getter Interface für Ausgabewerte")
**Exposure**: Public (2)

Defines a contract for objects that can lazily compute or hold a string value for CSV output. Used in the injector mechanism so that computed values are evaluated at CSV rendering time rather than at preparation time.

| Method | Description |
|---|---|
| `GET` | Returns the string value |
| `DISPLAY` | Display action (no return value) |
| `SET( IT_EXPORT_DATA )` | Provide the export data table to the getter |
| `IS_RELEVANT( IT_EXPORT_DATA )` | Returns `ABAP_BOOL` — whether this getter applies |

---

## CDS Views

### `/HFQ/DXP_R_EXPORT_COMPRESSED`
**Type**: Root View Entity (RAP)
**Description**: Root-Entity zu Komprimierten Exportdaten
**Base table**: `/HFQ/DXP_T_E_CMP`
**Authorization check**: `#NOT_REQUIRED`

```abap
key transaction_id as transactionId,
key set_name       as setName,
compressed_data    as compressedData
```

Exposes the compressed export data table as a RAP root entity. Backing behavior: create/update/delete, and the `exportData` event (parameterized by `/HFQ/DXP_A_BTP_EXPORT`).

---

### `/HFQ/DXP_A_BTP_EXPORT`
**Type**: Abstract Entity (event parameter entity)
**Description**: Abstrakte Parameter Entität ("Abstract Parameter Entity")

```abap
parametes : abap.string;   -- note: typo in source ("parametes")
content   : abap.string;
```

Defines the payload of the `exportData` RAP event: an API endpoint parameter string and the decompressed export content string. *Note: the field `parametes` appears to be a typo for `parameters` in the source code.*

---

## Tables / Data Definitions

### `/HFQ/DXP_T_IEC` — Converted Values (Transparent Table)
**Description**: Converted Values in PFM Framework (DE: Konvertierte Werte (PFM))
**Table class**: Transparent, client-dependent
**Buffering**: Not allowed
**Data class**: APPL0 (master data)

| Field | Key | Type | Description |
|---|---|---|---|
| `.INCLUDE /HFQ/DXP_S_DATA_KEY` | X | Structure | Key components: transaction_id + sub_index |
| `FIELD_NAME` | X | `NAME_FELD` | Output field name |
| `EXTERNAL_VALUE` | | `/HFQ/DXP_E_OUTPUT_VALUE` | Converted external value string |

Persisted by `CL_DC_CONTROLLER` and read back in `GET_DATA`.

---

### `/HFQ/DXP_T_E_CMP` — Compressed Data for Export (Transparent Table)
**Description**: Komprimierte Daten zum Export
**Table class**: Transparent, client-dependent
**Buffering**: Not allowed
**Data class**: APPL1 (transaction data)

| Field | Key | Type | Description |
|---|---|---|---|
| `.INCLUDE /HFQ/DXP_S_EVENT_KEY` | X | Structure | Event key (transaction_id) |
| `SET_NAME` | X | `/HFQ/DXP_E_CONV_SET_NAME` | Conversion set name |
| `COMPRESSED_DATA` | | `/HFQ/DXP_E_COMPRESSED_DATA` | Raw string (RAWSTRING domain) compressed payload |
| `_DATAAGING` | | `DATA_TEMPERATURE` | Data aging temperature field |

Backing table for the RAP Root View Entity `R_EXPORT_COMPRESSED`.

---

### `/HFQ/DXP_S_EXPORT_DATA` — Export Data Structure (Structure / INTTAB)
**Description**: Export Data Structure for PFM Framework (DE: Struktur für Datenexport (PFM))

| Field | Type | Description |
|---|---|---|
| `.INCLUDE /HFQ/DXP_S_TABLE_FIELD` | | Table name + field name compound |
| `VALUE_REFERENCE` | `REF TO DATA` | Generic reference to the field value |

Row type of `TT_EXPORT_DATA`. The generic reference allows holding any ABAP type, including references to value-getter interface implementations.

---

### `/HFQ/DXP_TT_EXPORT_DATA` — Sorted Export Data Table (Table Type)
**Description**: Sorted Export Data Table for PFM Framework
**Row type**: `/HFQ/DXP_S_EXPORT_DATA`
**Table kind**: SORTED, unique key on `TABLE_NAME` + `FIELD_NAME`

The primary in-memory container passed through the export pipeline.

---

## Domains and Data Elements

### `/HFQ/DXP_E_COMPRESSED_DATA` — Data Element
**Description**: Komprimierte Daten ("Compressed Data")
**Domain**: `RAWSTRING`
**Master language**: German

Holds binary-compressed export payloads in `T_E_CMP`. Screen texts: "Kompr. Dat" (short), "Komprimierte Daten" (medium/long).

---

## Other Objects

### `/HFQ/DXP_MC_EXP` — Message Class
**Description**: Nachrichten für Export (PFM) / Messages for Export (PFM)
**Master language**: English; German translation included

| Nr | Text (EN) |
|---|---|
| 000 | Method &1 is not valid for conversion. |
| 001 | Data for Transaction Id &1 not inserted into DB Table &2. |
| 002 | &1 could not be converted. |
| 003 | Ext. ID of Service Prov. &1 could not be determined. |
| 004 | Ext. ID of Settl. Unit &1 could not be determined. |
| 005 | Ext. ID of PoD &1 could not be determined. |
| 006 | &1 exceeds maximal Field Name Length. |
| 007 | File dialog was cancelled and path is empty. |
| 008 | Ext. ID of Settl. Terr. &1 could not be determined. |
| 009 | Konvertierungssatz zu &1 konnte nicht eindeutig ermittelt werden. *(DE only)* |
| 010 | Objecttype &1 of the transaction event is not as expected. |
| 014 | Klasse &1 implementiert nicht &2. *(DE only)* |

---

### `(HFQ)DXP_EVB_EXPORT_DATA` — Event Binding
**Type**: EVTB (Event Binding, JSON)
**Description**: Data Export Binding
**Original language**: en

Registers the RAP entity event as a SAP Business Technology Platform event:

| Property | Value |
|---|---|
| `producerNamespace` | `hfq` |
| `producer` | `PEC8FD80B8B64E55DF52C40CFB90C8073` |
| `boName` | `dxp` |
| `boOperation` | `export` |
| `producerType` | `hfq.dxp.export.v*` |
| Entity / Event | `/HFQ/DXP_R_EXPORT_COMPRESSED` / `EXPORTDATA` |

Declares the BTP event contract for downstream consumers of the compressed export data event.

---

### `/HFQ/` — Namespace
**Owner**: Hochfrequenz
