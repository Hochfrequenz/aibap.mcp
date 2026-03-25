# Package: HFQ / DXP_DATA

**Description**: PFM-Framework: Data Provision
**Original language**: English (E)
**Number of objects**: 90 files across main package and sub-package `ubd`

Sub-package `HFQ/DXP_DATA/ubd`:
**Description**: UBD Logiken
**Original language**: German (D)

---

## Executive Summary

`HFQ/DXP_DATA` is the data provision layer of the PFM (Process Flow Management) Framework for energy utility processes. It implements a class hierarchy of typed data providers — one per data domain (general PoD data, billing, settlement, technical, sales order) — all rooted in the abstract base class `\HFQ\DXP_CL_DP_ABSTRACT`, which handles the generic read/write lifecycle against dedicated transparent database tables keyed by `TRANSACTION_ID` and `SUB_INDEX`. A parallel branch (`\HFQ\DXP_CL_DP_SERIALIZED`) stores arbitrary deep structures in a path/field/value serialization table (`/HFQ/DXP_T_DSER`), enabling flexible extension without schema changes. The sub-package `ubd` provides a concrete serialized data provider for customer name/address data fetched from the /US4G/ UBD service. The package also owns all DDIC artefacts — transparent tables, flat structures, domains, and data elements — that define the data contracts of the PFM framework.

---

## Classes

### `/HFQ/DXP_CL_DP_ABSTRACT`

**Description**: Abstract Data Provider for PFM
**Superclass**: `/HFQ/DXP_CL_DYN_OBJECT`
**Type**: `ABSTRACT`, `PUBLIC`
**Implements**: `/HFQ/DXP_IF_DATA_PROVIDER` (with `FINAL METHODS determine_data`)
**Message class**: `/HFQ/DXP_MC_DATA`

Base class for all domain-specific data providers. Handles the full CRUD lifecycle against customizing-configured database tables. The database table name and structure name are retrieved at runtime from `/HFQ/DXP_CL_CONFIG_ACCESS`. Line types are built dynamically via RTTI.

**Public interface methods (from `/HFQ/DXP_IF_DATA_PROVIDER`):**

| Method | Signature | Behavior (verified from implementation) |
|---|---|---|
| `determine_data` | `IMPORTING iv_transaction_id iv_sub_index iv_int_ui_malo iv_int_ui_melo EXPORTING er_data` | FINAL. Calls `initialize`, loads event buffer from `/HFQ/DXP_CL_DB_EVE`, creates RTTI-based data structure, delegates to abstract `process_data_provision`. |
| `get_data` | `IMPORTING iv_transaction_id [iv_sub_index] EXPORTING er_data` | Reads from the configured transparent table via dynamic `SELECT ... INTO CORRESPONDING FIELDS`. Raises `/HFQ/DXP_CX_GENERAL_ERROR` if no data found. |
| `save_data` | `IMPORTING iv_transaction_id iv_sub_index iv_force_overwrite ir_data RETURNING rv_update_done` | Attempts `INSERT`; if duplicate exists and `iv_force_overwrite = true`, falls back to `UPDATE`. Returns `abap_false` when input data is initial. |

**Protected methods:**

| Method | Signature | Description |
|---|---|---|
| `get_general_data` | `IMPORTING iv_transaction_id iv_sub_index RETURNING rs_general_data TYPE /HFQ/DXP_S_DATA_GENERAL RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Instantiates `\HFQ\DXP_CL_DP_GENERAL` and reads general data for the given transaction/sub-index. |
| `get_pod_data_access` | `RETURNING rr_pod_data_access TYPE REF TO /UCOM/IF_POD_DATA_ACCESS RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Singleton: returns cached instance from `/UCOM/CL_AP_POD_MGMT_FACTORY`. |
| `get_process_document_data` | `RETURNING rs_proc_data TYPE /APE/S_PROC_DATA` | Stub — implementation is fully commented out. Returns empty structure. |
| `get_technical_data` | `IMPORTING iv_transaction_id iv_sub_index RETURNING rs_technical_data TYPE /HFQ/DXP_S_DATA_TECHNICAL RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Instantiates `\HFQ\DXP_CL_DP_TECHNICAL` and reads technical data. |
| `initialize` | (none) | Clears `gs_event_data` and `gs_pod_rel`. Subclasses override to additionally clear their own state. |
| `process_data_provision` | `ABSTRACT IMPORTING iv_transaction_id iv_sub_index ir_data TYPE REF TO data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Template method — must be implemented by each concrete subclass. |

**Protected instance data:**

- `gs_event_data TYPE /HFQ/DXP_CL_DB_EVE=>ty_s_event_data_add` — buffered event data
- `gs_pod_rel TYPE ty_s_pod_rel` — sub-index / int_ui_malo / int_ui_melo triple
- `gs_proc_data TYPE /APE/S_PROC_DATA` — process document data (currently unused)

---

### `/HFQ/DXP_CL_DP_GENERAL`

**Description**: Data Provider for General Data for PFM
**Superclass**: `/HFQ/DXP_CL_DP_ABSTRACT`
**DB table**: `/HFQ/DXP_T_DGN`

Determines general PoD attributes for a transaction. All work is done in `process_data_provision`.

**Protected methods (all private):**

| Method | Signature | Behavior |
|---|---|---|
| `process_data_provision` | redefined | Fills `\HFQ\DXP_S_DATA_GENERAL`: `int_ui` via `get_int_ui`, `pod_type` via `get_pod_type`, `installation` via `get_installation`, `division_category` via `get_division_category`, `energy_direction` via `get_energy_direction`. Clears `holiday_profile`. |
| `get_int_ui` | `RETURNING rv_int_ui TYPE INT_UI RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Returns `gs_pod_rel-int_ui_malo`; falls back to `gs_event_data-object_key` when object type is `POD`. |
| `get_pod_type` | `IMPORTING is_data RETURNING rv_pod_type TYPE /HFQ/DXP_E_POD_TYPE RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/IF_POD_DATA_ACCESS~get_pod_category`; maps result to `'MA'` or `'SU'` (checks `EEDMSETTLUNITPOD`). |
| `get_installation` | `IMPORTING is_data RETURNING rv_installation TYPE ANLAGE RAISING /HFQ/DXP_CX_GENERAL_ERROR` | For `MA`: calls `/UCOM/IF_POD_DATA_ACCESS~get_installation`. For `SU`: returns empty. |
| `get_division_category` | `IMPORTING is_data RETURNING rv_division_category TYPE SPART RAISING /HFQ/DXP_CX_GENERAL_ERROR` | For `MA`: calls `/UCOM/IF_POD_DATA_ACCESS~get_division_category`. For `SU`: queries `EEDMSETTLUNIT JOIN EEDMSETTLUNITPOD`. |
| `get_energy_direction` | `IMPORTING is_data RETURNING rv_energy_direction TYPE /HFQ/DXP_E_ENERGY_DIRECTION RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Reads `EANL-BEZUG` via `CL_ISU_DB_EANL`. |
| `is_settlunit_pod` | `IMPORTING iv_int_ui RETURNING rv_result TYPE ABAP_BOOL` | SELECT EXISTS on `EEDMSETTLUNITPOD JOIN EEDMSETTLUNIT` for the key date. |

---

### `/HFQ/DXP_CL_DP_BILLING`

**Description**: Data Provider for Settlement Data for PFM *(note: XML description says "Settlement", but the class provides billing/contract data)*
**Superclass**: `/HFQ/DXP_CL_DP_ABSTRACT`
**DB table**: `/HFQ/DXP_T_DBI`

Determines billing-relevant attributes for market locations (skips settlement units). Only active for `pod_type = 'MA'`.

**Private methods:**

| Method | Signature | Behavior |
|---|---|---|
| `process_data_provision` | redefined | Reads general data, then calls `get_billing_data_for_malo` only for `pod_type = 'MA'`. |
| `get_billing_data_for_malo` | `CHANGING cs_data TYPE /HFQ/DXP_S_DATA_BILLING RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Fills `contract`, `contract_account`, `move_in_date`, `move_out_date` via `get_contract`; `business_partner` via `get_business_partner`; CRM fields via `get_crm_data`. |
| `get_contract` | `IMPORTING is_data EXPORTING ev_contract ev_contract_account ev_move_in_date ev_move_out_date RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/IF_POD_DATA_ACCESS~get_contract` — returns single contract if exactly one found. |
| `get_business_partner` | `IMPORTING is_data RETURNING rv_business_partner TYPE BU_PARTNER RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/IF_POD_DATA_ACCESS~get_bp` — returns single BP if exactly one found. |
| `get_crm_data` | `CHANGING cs_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | For scenario `update_procurement_segment`: hardcodes `procurement_segment = 'Lieferende'`. Otherwise resolves CRM item GUID and reads order data. |
| `get_crm_item_guid` | `IMPORTING is_data RETURNING rv_item_guid TYPE CRMT_OBJECT_GUID RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Queries `CRMS4D_IUCP_I` filtered by contract number and key-date timestamp. Returns empty if not exactly one result. |
| `get_order_data` | `IMPORTING iv_item_guid RETURNING rs_order_data TYPE TY_S_ORDER_DATA` | Queries `CRMS4V_IU_1OCVAL`; reads `ordered_prod` as `procurement_segment`; uses `CTCV_CONVERT_FLOAT_TO_DATE` to read move-out date from characteristic `ATINN = '0000000887'`. Sets `is_b2b = true` when `procurement_segment CP '*_B2B_*'`. |

---

### `/HFQ/DXP_CL_DP_SETTLEMENT`

**Description**: Data Provider for Settlement Data for PFM
**Superclass**: `/HFQ/DXP_CL_DP_ABSTRACT`
**DB table**: `/HFQ/DXP_T_DST`

Determines settlement/balancing attributes. Dispatches between market location (`MA`) and settlement unit (`SU`) paths.

**Private methods (key ones):**

| Method | Signature | Behavior |
|---|---|---|
| `process_data_provision` | redefined | Reads general data and technical data; for `MA` calls `get_settlement_data_for_malo`, for `SU` calls `get_settlement_data_for_su`. |
| `get_settlement_data_for_malo` | `IMPORTING iv_transaction_id iv_sub_index CHANGING cs_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Orchestrates: settlement unit (via `CL_ISU_EDM_UI_SETTLUNIT`), metering procedure, settlement services (coord/sup/DSO), grid data, settlement territory, TSO ID, aggregation responsibility, and SLP/TLP usage profiles. |
| `get_settlement_data_for_su` | `CHANGING cs_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Reads settlement unit from `EEDMSETTLUNITPOD JOIN EEDMSETTLUNIT`, settlement services, and grid ID via `CL_ISU_EDM_SETTLUNIT`. |
| `get_settlement_unit_for_malo` | `IMPORTING is_data EXPORTING ev_settlunit ev_settl_start_date ev_settl_end_date RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Uses `CL_ISU_EDM_UI_SETTLUNIT~select_pod` + `/US4G/CL_UBD_DATA_POD~det_async_settlement_view`; applies date search logic with optional override from process document. |
| `get_usage_data` | `IMPORTING is_data EXPORTING es_slp_data es_tlp_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Queries `ELPASS JOIN EUFASS JOIN EPROFHEAD` for balancing-role profiles; determines SLP/TLP via `/US4G/CL_EDM_BA_FACTORY`. Clears TLP for E02, clears SLP for E14. |
| `get_settlement_territory` | `IMPORTING is_data RETURNING rv_settlterritory RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Queries `/US4G/S_ST_GH` by grid ID and key date. |
| `get_tso_id` | `IMPORTING is_data RETURNING rv_tso_id RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/BADI_DETERMINE_SP~determine_tso_from_grid`. |
| `get_aggresp` | `IMPORTING is_data RETURNING rv_aggresp RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/US4G/CL_STL_MALO_FACTORY~get_stl_malo_query~sel_stl_malo_by_keys`. |
| `get_metering_procedure` | `IMPORTING iv_transaction_id iv_sub_index is_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/BADI_INSTL_ACCESS~get_installation_type`; if result is non-initial, saves it via `\HFQ\DXP_CL_DP_TECHNICAL~save_data`. |
| `get_balancing_dates_from_pdoc` | `EXPORTING ev_settl_start_date ev_settl_end_date` | Reads from process document step data (via `get_process_document_data`). Returns empty values because `get_process_document_data` is stubbed out. *Inferred from code: effectively a no-op at runtime.* |

---

### `/HFQ/DXP_CL_DP_TECHNICAL`

**Description**: Data Provider for Technical Data for PFM
**Superclass**: `/HFQ/DXP_CL_DP_ABSTRACT`
**DB table**: `/HFQ/DXP_T_DTE`

Determines technical attributes of the metering location. Only active for `pod_type = 'MA'`.

**Private methods:**

| Method | Signature | Behavior |
|---|---|---|
| `process_data_provision` | redefined | Reads general data; for `MA` calls `get_technical_data_for_malo`. |
| `get_technical_data_for_malo` | `CHANGING cs_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Fills `int_ui_melo`, `metering_procedure`, `market_classification`, `external_reference_id`, `mos_id`, `mos_start_date`, `mos_end_date`, `address_number`, `pressure_level`, `voltage_level`. |
| `get_int_ui_melo` | `IMPORTING is_data RETURNING rv_int_ui_melo RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Returns `gs_pod_rel-int_ui_melo`. |
| `get_metering_type` | `IMPORTING is_data EXPORTING ev_metering_procedure ev_market_classification RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/BADI_INSTL_ACCESS~get_installation_type`. |
| `get_metering_operator_service` | `IMPORTING is_data EXPORTING ev_meter_operator ev_mos_start_date ev_mos_end_date RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Reads MOS service provider (service type `'M1'`) via `/UCOM/CL_AP_POD_MGMT_FACTORY~get_pod_data_access~get_service_provider`. |
| `get_external_reference_id` | `IMPORTING is_data RETURNING rv_external_reference_id RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Reads `EUITRANS-EXT_UI` for the PoD at key date. |
| `get_pressure_level` | `IMPORTING is_data RETURNING rv_pressure_level RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Gas only: queries grid via `ISU_DB_EUIGRID_SELECT`, then reads `/US4G/BKSUP-GRID_LEVEL`. |
| `get_voltage_level` | `IMPORTING is_data RETURNING rv_voltage_level RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Electricity only: reads `EANL-SPEBENE` for the installation. |
| `get_address_number` | `IMPORTING is_data RETURNING rv_address_number RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `/UCOM/CL_AP_POD_MGMT_FACTORY~get_pod_data_access~get_name_address`. |
| `get_badi_instl_access` | `RETURNING rr_badi_instl_access RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Singleton: cached instance from `/UCOM/BADI_INSTL_ACCESS`. |

---

### `/HFQ/DXP_CL_DP_SALES_EC`

**Description**: Data Provider for Sales Order Data for PFM
**Superclass**: `/HFQ/DXP_CL_DP_ABSTRACT`
**DB table**: `/HFQ/DXP_T_DSO`

Handles sales-order-driven transactions. Overrides both `get_data` and `save_data` to also read/write sub-data structures (general, billing, settlement, technical) via a registered list of sub-data-interfaces.

**Public methods:**

| Method | Signature | Behavior |
|---|---|---|
| `constructor` | (none) | Registers sub-data-interface entries for general, billing, settlement, and technical data providers. *Inferred from instance data `gt_sub_data_interfaces` — exact registration code not shown in the definition section read.* |
| `/HFQ/DXP_IF_DATA_PROVIDER~get_data` | redefined | Reads sales order rows from parent, then loop-fills each sub-data structure by calling the registered sub-provider's `get_data`. |
| `/HFQ/DXP_IF_DATA_PROVIDER~save_data` | redefined | Calls parent `save_data`, then iterates sub-data-interfaces and calls each sub-provider's `save_data`. |

**Protected methods (key ones):**

| Method | Signature |
|---|---|
| `get_sales_order_data` | `RETURNING rs_data_sales_order TYPE /HFQ/DXP_S_DATA_SALES_ORDER RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `determine_sales_order_data` | `RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `determine_malo_and_melo` | `RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `determine_contract_dates` | `RETURNING rs_timeslice TYPE TY_S_TIMESLICE RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `determine_continuous_timeslice` | `IMPORTING it_timeslices RETURNING rs_timeslice TYPE TY_S_TIMESLICE RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `determine_settlement_territory` | `IMPORTING is_data_sales_order RETURNING rv_settlterritory RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `complete_config_values` | `IMPORTING it_sales_order_config_values TYPE COD_T_BAPICUVALM RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `get_config_value` | `IMPORTING is_config_value EXPORTING ev_result TYPE ANY` |
| `get_general_data_sales` | `IMPORTING iv_transaction_id iv_sub_index RETURNING rs_data_general RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `get_billing_data` | `IMPORTING iv_transaction_id iv_sub_index RETURNING rs_data_billing RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `get_settlement_data` | `IMPORTING iv_transaction_id iv_sub_index is_data_sales_order RETURNING rs_data_settlement RAISING /HFQ/DXP_CX_GENERAL_ERROR` |
| `get_technical_data_sales` | `IMPORTING iv_transaction_id iv_sub_index is_data_sales_order RETURNING rs_data_technical RAISING /HFQ/DXP_CX_GENERAL_ERROR` |

---

### `/HFQ/DXP_CL_DP_SERIALIZED`

**Description**: Abstract Data Provider for PFM (serialized variant)
**Superclass**: `/HFQ/DXP_CL_DYN_OBJECT`
**Implements**: `/HFQ/DXP_IF_DATA_PROVIDER` (FINAL: `determine_data`), `/HFQ/DXP_IF_DP_SERIALIZED`
**DB table**: `/HFQ/DXP_T_DSER` (path/field/value serialization)

Base class for data providers that store their data in the generic path-serialized table rather than a domain-specific transparent table. Subclasses override `determine_data_child` to populate the output structure from live systems; the base-class `get_data` and `save_data` read/write via `\HFQ\DXP_CL_DP_SERIALIZER`.

**Public interface methods:**

| Method | Behavior |
|---|---|
| `determine_data` | FINAL. Creates RTTI structure from `mv_struct_name`, calls `determine_data_child`. |
| `get_data` | Reads from `/HFQ/DXP_T_DSER`; deserializes via `\HFQ\DXP_CL_DP_SERIALIZER~build_ext_structure_all`. |
| `save_data` | Serializes input structure via `\HFQ\DXP_CL_DP_SERIALIZER~serialize`, then saves via `~save_serialize`. Returns `abap_false` for initial data. |
| `get_serialized_data` | `IMPORTING iv_transaction_id [iv_include_external_values] RETURNING rt_data TYPE /HFQ/DXP_TT_SERIAL_DATA` — SELECTs all rows from `/HFQ/DXP_T_DSER`; optionally excludes paths matching `*@*`. |

**Protected methods:**

| Method | Signature | Description |
|---|---|---|
| `determine_data_child` | `IMPORTING iv_transaction_id iv_sub_index ir_data RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Default implementation raises `/HFQ/DXP_CX_GENERAL_ERROR`. Subclasses must override. |
| `create_line_type` | `IMPORTING iv_name RETURNING rr_structdescr RAISING /HFQ/DXP_CX_GENERAL_ERROR` | RTTI helper: describes structure by name, wraps in `gc_group_name_data` include. |
| `get_configuration` | `RETURNING rs_configuration RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Reads customizing config from `/HFQ/DXP_CL_CONFIG_ACCESS`. |

**Protected instance data:**

- `mv_struct_name TYPE STRING` — ABAP structure name used as the data schema; set by the subclass constructor.

---

### `/HFQ/DXP_CL_DP_SERIALIZER`

**Description**: Path Serializer
**Create**: `PROTECTED` (factory method `get_instance`)
**Has unit tests**: yes (`WITH_UNIT_TESTS = X`)

Generic utility that converts between typed ABAP structures and a flat path/field/value table (`/HFQ/DXP_TT_SERIAL_DATA`). Used by `\HFQ\DXP_CL_DP_SERIALIZED`.

**Public class method:**

| Method | Signature |
|---|---|
| `get_instance` | `IMPORTING iv_path_prefix TYPE /APE/DE_PATH_PREFIX iv_ignore_empty_value TYPE /APE/DE_BOOLEAN_FLAG [iv_path_end_slash iv_only_extract_path_fields it_keep_initial_field] RETURNING ro_serializer TYPE REF TO /HFQ/DXP_CL_DP_SERIALIZER` |

**Public instance methods:**

| Method | Signature | Description |
|---|---|---|
| `serialize` | `IMPORTING is_data TYPE ANY [iv_init_path] EXPORTING et_path TYPE /HFQ/DXP_TT_SERIAL_DATA RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Recursively flattens a structure into path/field/value rows. Tables are indexed as `COMPONENT[n]`. |
| `build_ext_structure` | `IMPORTING is_path_line TYPE /HFQ/DXP_S_SERIAL_DATA CHANGING cs_data TYPE ANY RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Applies one path/field/value row to a target structure. |
| `build_ext_structure_all` | `IMPORTING it_path_tab TYPE /HFQ/DXP_TT_SERIAL_DATA CHANGING cs_data TYPE ANY RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Iterates and applies all rows. |
| `build_data_from_same_path` | `IMPORTING it_path_tab TYPE /HFQ/DXP_TT_SERIAL_DATA CHANGING cs_data TYPE ANY RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Applies rows sharing the same path prefix. |
| `save_serialize` | `IMPORTING iv_transaction_id it_path TYPE /HFQ/DXP_TT_SERIAL_DATA [iv_force_overwrite] RETURNING rv_update_done RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Converts path rows to `/HFQ/DXP_T_DSER` rows (each with a UUID `DATA_ID`); uses `MODIFY` (force) or `INSERT ACCEPTING DUPLICATE KEYS` (no force). |

**Unit tests** (`ltcl_test_path`): cover round-trip serialization/deserialization of nested structures and tables, path slash variants, `build_data_from_same_path` with indexed paths.
*Note: test class uses `/APE/CL_PATH_SERIALIZER` and `/APE/CL_BACK_SERVICE_FACTORY` — the serializer appears to be an adapter or subclass of an APE framework base. Inferred from test class context.*

---

### `/HFQ/DXP_CL_DP_UBD_CUSTOMER` (sub-package `ubd`)

**Description**: Data Provider for Name Address
**Superclass**: `/HFQ/DXP_CL_DP_SERIALIZED`
**Data structure**: `/HFQ/DXP_CL_DP_UBD_CUSTOMER=>TY_S_DATA` (includes `/HFQ/DXP_S_DP_CUSTOMER`)

Concrete serialized data provider that fetches customer name/address data from the `/US4G/` UBD (Utility Business Data) service and serializes it into `/HFQ/DXP_T_DSER`.

**Public methods:**

| Method | Signature | Behavior |
|---|---|---|
| `constructor` | `RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Sets `mv_struct_name` to `'\CLASS=/HFQ/DXP_CL_DP_UBD_CUSTOMER=>TY_S_DATA'`. |

**Protected methods:**

| Method | Signature | Behavior |
|---|---|---|
| `determine_data_child` | redefined | Reads event data; creates UBD instance via `/APE/CL_BASIS_FACTORY`; queries `/US4G/IF_UBD_NM_ADDR~query` for `NaAdrPoD` category at `event_data-external_id` and `key_date`; maps result via `mapping`. |
| `mapping` | `IMPORTING is_ubd_data TYPE /US4G/S_UBD_DATA RETURNING rs_dxp_data TYPE TY_S_DATA` | Maps UBD name/address response to `/HFQ/DXP_S_DP_CUSTOMER`: `customer_id`, `customer_name` (first + last concatenated), `academic_title`, `first_name`, `last_name`, `customer_address` (street, house number, postal code, city, district, country code), and the original request. |

---

## Interfaces

### `/HFQ/DXP_IF_DATA_CONSTANTS`

**Description**: Data Constants for PFM Framework

Constant repository for the PFM data layer. Key constants (verified from `.intf.abap`):

| Constant | Value | Description |
|---|---|---|
| `GC_POD_TYPE_MARKET_LOCATION` | `'MA'` | PoD type: Market Location |
| `GC_POD_TYPE_SETTLEMENT_UNIT` | `'SU'` | PoD type: Settlement Unit |
| `GC_METER_PROC_E01` | `'E01'` | Interval Reading (RLM) |
| `GC_METER_PROC_E02` | `'E02'` | Non-Interval Reading (SLP/SEP) |
| `GC_METER_PROC_E14` | `'E14'` | TLP/TEP with Separate Measurement |
| `GC_METER_PROC_E24` | `'E24'` | TLP with Combined Measurement |
| `GC_DIVISION_CAT_ELEC` | `'01'` | Division category: Electricity |
| `GC_DIVISION_CAT_GAS` | `'02'` | Division category: Gas |
| `GC_PROFILE_TYPE_SLP` | `'SLP'` | Standard Load Profile |
| `GC_PROFILE_TYPE_TLP` | `'TLP'` | Telemetric Load Profile |
| `GC_SERVICE_TYPE_MOS` | `'M1'` | Metering Operator Service |
| `GC_STAT_LIFECYCLE_RELEASED` | `'D'` | CRM lifecycle status: Released |
| `GC_PROCUREMENT_SEGMENT_EOD` | `'Lieferende'` | End-of-delivery procurement segment |
| `GC_ATINN_MOVE_OUT_DATE` | `'0000000887'` | SAP characteristic ID for move-out date in CRM order |
| `GC_MARKETL_CLASSIF_Z52/Z53/Z68` | — | MaLo market classification codes |
| `GC_FIELD_NAME_TRANSACTION_ID/SUB_INDEX` | `'TRANSACTION_ID'` / `'SUB_INDEX'` | Field name constants for dynamic ABAP |

---

### `/HFQ/DXP_IF_DP_SERIALIZED`

**Description**: Interface for serialized data providers

| Method | Signature |
|---|---|
| `get_serialized_data` | `IMPORTING iv_transaction_id [iv_include_external_values TYPE ABAP_BOOL DEFAULT ABAP_FALSE] RETURNING rt_data TYPE /HFQ/DXP_TT_SERIAL_DATA RAISING /HFQ/DXP_CX_GENERAL_ERROR` |

---

## Tables / Data Definitions

### Transparent Database Tables

All transparent tables use `TABKAT` 3 (application) and `BUFALLOW = N`. Keys are always the include `/HFQ/DXP_S_DATA_KEY` (CLIENT + TRANSACTION_ID + SUB_INDEX).

| Table | Description | Data include |
|---|---|---|
| `/HFQ/DXP_T_DGN` | General Transaction Data for PFM Framework | `/HFQ/DXP_S_DATA_GENERAL` |
| `/HFQ/DXP_T_DBI` | Billing Transaction Data for PFM Framework | `/HFQ/DXP_S_DATA_BILLING` |
| `/HFQ/DXP_T_DST` | Settlement Transaction Data for PFM Framework | `/HFQ/DXP_S_DATA_SETTLEMENT` |
| `/HFQ/DXP_T_DTE` | Technical Transaction Data for PFM Framework | `/HFQ/DXP_S_DATA_TECHNICAL` |
| `/HFQ/DXP_T_DSO` | Sales Order Transaction Data for PFM Framework | `/HFQ/DXP_S_DATA_SO_DB` |
| `/HFQ/DXP_T_DSER` | Serialized Step Data (`TABKAT 4`, `TABART APPL1`) | Key: `/HFQ/DXP_S_SERIAL_DATA_KEY` (CLIENT + TRANSACTION_ID + DATA_ID); Data: `/HFQ/DXP_S_SERIAL_DATA` |

### Flat Structures (INTTAB)

#### Key Structures

| Structure | Description | Fields |
|---|---|---|
| `/HFQ/DXP_S_DATA_KEY` | Key Components for PFM Data | CLIENT (MANDT), TRANSACTION_ID, SUB_INDEX |
| `/HFQ/DXP_S_SERIAL_DATA_KEY` | Serialized Data Key | CLIENT, TRANSACTION_ID, DATA_ID (UUID) |

#### Domain Data Structures

| Structure | Description | Key Fields |
|---|---|---|
| `/HFQ/DXP_S_DATA_GENERAL` | Data Fields for PFM General Data | INT_UI, POD_TYPE, DIVISION_CATEGORY, INSTALLATION, ENERGY_DIRECTION, HOLIDAY_PROFILE |
| `/HFQ/DXP_S_DATA_BILLING` | Data Fields for PFM Billing Data | CONTRACT, BUSINESS_PARTNER, CONTRACT_ACCOUNT, MOVE_IN_DATE, MOVE_OUT_DATE, PROCUREMENT_SEGMENT |
| `/HFQ/DXP_S_DATA_SETTLEMENT` | Data Fields for PFM Settlement Data | SETTLUNIT, SETTL_START_DATE, SETTL_END_DATE, SETTLTERRITORY, COORD_ID, SUP_ID, DSO_ID, TSO_ID, AGGRESP, GRID_ID, GRID_START_DATE, GRID_END_DATE, SLP (include), TLP (include), FORECAST_PROFILE |
| `/HFQ/DXP_S_DATA_TECHNICAL` | Data Fields for PFM Technical Data | INT_UI_MELO, METERING_PROCEDURE, MARKET_CLASSIFICATION, MOS_ID, MOS_START_DATE, MOS_END_DATE, ADDRESS_NUMBER, PRESSURE_LEVEL, VOLTAGE_LEVEL, EXTERNAL_REFERENCE_ID |
| `/HFQ/DXP_S_DATA_SO_DB` | PFM Sales Order Data (DB variant) | SALES_DOC, EXTERNAL_DOC_ID, INTERNAL_DOC_ID, STREET, HOUSE_NUMBER, HOUSE_NUMBER_SUP, POSTAL_CODE, CITY, FORECAST_MASS, FORECAST_PERIOD_START_DATE, FORECAST_PERIOD_END_DATE, MOVE_IN_DATE_PARENT, MOVE_OUT_DATE_PARENT |
| `/HFQ/DXP_S_DATA_SALES_ORDER` | PFM Sales Order Data (full) | Includes: DATA_SO_DB + DATA_GENERAL + DATA_BILLING + DATA_SETTLEMENT + DATA_TECHNICAL |
| `/HFQ/DXP_S_LOAD_PROFILE` | Substructure for Load Profile in PFM | TYPE\_, LOAD_PROFILE_ID\_, PROFILE\_, USAGE_FACTOR\_, START_DATE\_, END_DATE\_, MASS\_ |
| `/HFQ/DXP_S_SERIAL_DATA` | Serialized Data Row | PATH, FIELD, VALUE, PERSON_DATA_IND |

#### UBD Sub-package Structures

| Structure | Description | Fields |
|---|---|---|
| `/HFQ/DXP_S_DP_CUSTOMER` | Customer data for provision | EXTERNAL_CUSTOMER_ID, CUSTOMER_ID (BU_PARTNER), CUSTOMER_NAME, ACADEMIC_TITLE, FIRST_NAME, LAST_NAME, CUSTOMER_ADDRESS (structure), REQUEST |
| `/HFQ/DXP_S_DP_CUSTOMER_ADDRESS` | Customer address sub-structure | STREET, HOUSE_NUMBER, POSTAL_CODE, CITY, DISTRICT, COUNTRY_CODE |

### Table Types

| Type | Description | Row type |
|---|---|---|
| `/HFQ/DXP_TT_SERIAL_DATA` | Table of serialized data rows | `/HFQ/DXP_S_SERIAL_DATA` (standard table, non-unique default key) |

---

## Domains and Data Elements

### Domains

| Domain | Type/Length | Description | Values |
|---|---|---|---|
| `/HFQ/DXP_D_POD_TYPE` | CHAR(2) | PoD Type in PFM Framework | `MA` = Market Location, `SU` = Settlement Unit |
| `/HFQ/DXP_D_METER_PROC` | CHAR(3) | Metering Procedure | `E01` = RLM, `E02` = SLP/SEP, `E14` = TLP separate, `E24` = TLP combined, `Z29` = Flat-rate, `Z36` = TEP reference |
| `/HFQ/DXP_D_AGGRESP` | CHAR(1) | Aggregation Responsibility DSO/TSO | (blank) = DSO, `1` = TSO, `2` = Charging point operator |
| `/HFQ/DXP_D_VALUE` | SSTR(200) | Value (for serialization) | — |
| `/HFQ/DXP_D_ENERGY_DIRECTION` | *Inferred from context.* Not verified against implementation. | Energy direction (feeding/consumption) | — |
| `/HFQ/DXP_D_FEEDING` | *Inferred from context.* Not verified against implementation. | Feeding indicator | — |
| `/HFQ/DXP_D_HOLIDAY_PROFILE` | *Inferred from context.* Not verified against implementation. | Holiday profile | — |
| `/HFQ/DXP_D_LP_TYPE` | *Inferred from context.* Not verified against implementation. | Load profile type | — |
| `/HFQ/DXP_D_MARKETL_CLASSIF` | *Inferred from context.* Not verified against implementation. | Market location classification | — |
| `/HFQ/DXP_D_PATH` | *Inferred from context.* Not verified against implementation. | Serialization path string | — |
| `/HFQ/DXP_D_PERS_DATA_INDICATOR` | *Inferred from context.* Not verified against implementation. | Personal data indicator | — |
| `/HFQ/DXP_D_PROF_TYPE` | *Inferred from context.* Not verified against implementation. | Profile type (SLP/TLP) | — |

### Data Elements (selection)

| Data Element | Domain | Description |
|---|---|---|
| `/HFQ/DXP_E_POD_TYPE` | `/HFQ/DXP_D_POD_TYPE` | Type of PoD in PFM Framework |
| `/HFQ/DXP_E_METER_PROC` | `/HFQ/DXP_D_METER_PROC` | Metering Procedure |
| `/HFQ/DXP_E_AGGRESP` | `/HFQ/DXP_D_AGGRESP` | Aggregation Responsibility |
| `/HFQ/DXP_E_VALUE` | `/HFQ/DXP_D_VALUE` | Serialized field value |
| `/HFQ/DXP_E_PATH` | `/HFQ/DXP_D_PATH` | Serialization path |
| `/HFQ/DXP_E_ENERGY_DIRECTION` | `/HFQ/DXP_D_ENERGY_DIRECTION` | Energy direction |
| `/HFQ/DXP_E_FEEDING` | `/HFQ/DXP_D_FEEDING` | Feeding indicator |
| `/HFQ/DXP_E_HOLIDAY_PROFILE` | `/HFQ/DXP_D_HOLIDAY_PROFILE` | Holiday profile |
| `/HFQ/DXP_E_LOAD_PROF_ID` | *Inferred.* | Load profile external ID |
| `/HFQ/DXP_E_LP_TYPE` | `/HFQ/DXP_D_LP_TYPE` | Load profile type |
| `/HFQ/DXP_E_LP_USAGE_FACTOR` | *Inferred.* | Usage factor for load profile |
| `/HFQ/DXP_E_LP_UF_START_DATE` / `..._END_DATE` | *Inferred.* | Usage factor validity dates |
| `/HFQ/DXP_E_MARKETL_CLASSIF` | `/HFQ/DXP_D_MARKETL_CLASSIF` | Market location classification |
| `/HFQ/DXP_E_METER_OPERATOR_ID` | *Inferred.* | Metering operator ID |
| `/HFQ/DXP_E_MOS_START_DATE` / `..._END_DATE` | *Inferred.* | MOS service validity dates |
| `/HFQ/DXP_E_GRID_START_DATE` / `..._END_DATE` | *Inferred.* | Grid allocation validity dates |
| `/HFQ/DXP_E_SETTL_START_DATE` / `..._END_DATE` | *Inferred.* | Settlement period dates |
| `/HFQ/DXP_E_PERS_DATA_INDICATOR` | `/HFQ/DXP_D_PERS_DATA_INDICATOR` | Personal data indicator for serialized rows |
| `/HFQ/DXP_E_INT_UI_MELO` | *Inferred.* | Internal key for metering location |
| `/HFQ/DXP_E_VIRTUAL_POD` | *Inferred.* | External reference ID / virtual PoD |
| `/HFQ/DXP_E_TRANS_OPERATOR_ID` | *Inferred.* | Transmission system operator ID |
| `/HFQ/DXP_E_PROCUREMENT_SEGM` | *Inferred.* | Procurement segment / product ID |
| `/HFQ/DXP_E_SUB_INDEX` | *Inferred.* | Sub-index for n:m transaction/PoD relationships |
| `/HFQ/DXP_E_DATA_STEP_ID` | *Inferred.* | UUID key for serialized data rows |
| `/HFQ/DXP_E_PROF_TYPE` | `/HFQ/DXP_D_PROF_TYPE` | Profile type |
| `/HFQ/DXP_E_CUSTOMER_NAME` | *Inferred (UBD sub-package).* | Combined customer name string |
| `/HFQ/DXP_E_EXT_CUSTOMER_ID` | *Inferred (UBD sub-package).* | External customer ID |

---

## Other Objects

### Message Class `/HFQ/DXP_MC_DATA`

**Description**: Messages for Data Provision (PFM)
**Master language**: English

| Nr | Text (EN) |
|---|---|
| 000 | Field-Symbol could not be assigned. |
| 001 | Data for Transaction &1 already exists. |
| 002 | Data for Transaction &1 could not be saved. |
| 003 | Data for Transaction &1 not found. |
| 004 | Contract data could not be fully determined. |
| 005 | Transaction was not caused by sales order. |
| 006 | Point of Delivery ID of internal PoD &1 not found. |
| 007 | Installation for internal Point of Delivery &1 not found. |
| 008 | No devices found for installation &1. |
| 009 | An error occured while completing the config value data. |
| 010 | Import parameter for timeslices is empty. |
| 011 | Incorrectly defined timeslice. DATE_FROM is bigger than DATE_TO. |
| 012 | Timeslices are intersecting and cannot be glued together. |
| 013 | Mandatory field MaLo could not be determined for contract extension. |
| 999 | Data for mandatory Field &1 could not be filled. |
