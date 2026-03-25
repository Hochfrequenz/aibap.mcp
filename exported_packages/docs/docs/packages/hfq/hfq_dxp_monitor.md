# Package: HFQ / DXP_MONITOR

**Description**: PFM-Framework: Monitoring
**Original language**: English (E)
**Number of objects**: 14 (3 classes, 1 interface, 5 data elements, 1 message class, 4 program includes + 1 main program, 1 transaction)

---

## Executive Summary

`HFQ/DXP_MONITOR` is the monitoring sub-package of the PFM (Process/Data Export Framework) built by Hochfrequenz. It delivers a standalone SAP GUI transaction (`/HFQ/DXP_TRANSMON`) that lets operators inspect, navigate, and re-trigger PFM export events from a three-panel ALV view.

The package is self-contained from a UI perspective: it owns its selection screen, all ALV wiring, and the event-handler class that reacts to toolbar button clicks. Business logic (actual export execution) is delegated to classes from the broader `HFQ/DXP_*` namespace (`/HFQ/DXP_CL_EVENT_HANDLER`, `/HFQ/DXP_CL_EXPORT_HANDLER`).

Key runtime flow:

1. The report `/HFQ/DXP_TRANS_MONITOR` (reachable as transaction `/HFQ/DXP_TRANSMON`) checks authority for T-code `S_TCODE` and calls `lcl_pfm_transmon=>start_of_selection`.
2. `start_of_selection` instantiates `/HFQ/DXP_CL_MONITOR` via its factory method `GET_INSTANCE`, calls `SELECT_TRANSACTIONS` with the selection-screen ranges, and calls `DISPLAY`.
3. `/HFQ/DXP_CL_MONITOR` splits the screen into three `CL_SALV_TABLE` panels: **Main** (event list), **Sub** (market/metering-location sub-indices per transaction), and **Data** (dynamically generated columns showing exported field values).
4. User interactions (double-click, toolbar buttons) are handled by `/HFQ/DXP_CL_MONITOR_HANDLER`, which fires back into the monitor's public API.
5. `/HFQ/DXP_CL_DE_ALV` is a lightweight specialisation of `/HFQ/DXP_CL_DE_ABSTRACT` that runs a data-export preparation in memory without persisting results, supplying raw transformed data to the Data panel.

---

## Classes

### `/HFQ/DXP_CL_MONITOR`

**Description**: Transaction Monitor for PFM
**Instantiation**: `CREATE PRIVATE` — use the factory method `GET_INSTANCE`
**Message class**: `/HFQ/DXP_MC_MON`

The central model/controller class for the monitoring view. Manages three `CL_SALV_TABLE` instances embedded in a `CL_GUI_SPLITTER_CONTAINER` hierarchy and coordinates data selection, display, and refresh.

#### Screen layout

The screen is split into two rows (top 40 %, bottom 60 %) and the bottom row is split into two columns (left 25 %, right 75 %):

| Panel | Container | ALV variable | Purpose |
|-------|-----------|--------------|---------|
| Main | top row | `gr_alv_main` | One row per PFM event, read from `/HFQ/DXP_T_EVE` joined with `EUITRANS` |
| Sub | bottom-left | `gr_alv_sub` | Sub-indices (MALO / MELO) for the selected event(s) |
| Data | bottom-right | `gr_alv_data` | Dynamically generated columns with individual exported field values |

#### Public methods

| Method | Parameters | Description |
|--------|-----------|-------------|
| `GET_INSTANCE` (static) | `IR_CONTAINER`, `IV_MAX_SEL_ROWS` (default 100), `IT_RANGE_SET_NAME` | Factory: resets the export handler cache and returns a new instance |
| `CONSTRUCTOR` | same as `GET_INSTANCE` | Creates splitter, three ALVs, registers event handlers; raises `/HFQ/DXP_CX_GENERAL_ERROR` |
| `DISPLAY` | — | Calls `display()` on all three ALV instances |
| `SELECT_TRANSACTIONS` | `IT_RANGE_POD`, `IT_RANGE_TRANS_ID`, `IT_RANGE_KEY_DATE`, `IT_RANGE_STATUS`, `IT_RANGE_SOURCE` | Reads `/HFQ/DXP_T_EVE` LEFT OUTER JOIN `EUITRANS`, resolves status/source fixed values to human-readable text, refreshes the Main ALV, and auto-fills the Sub panel for the first result row |
| `FILL_DATA_SUB` | `IT_DATA_MAIN` | Drives `/HFQ/DXP_CL_DE_ALV` to prepare export data in memory, populates the Sub ALV (MALO/MELO) and the dynamic Data ALV |
| `UPDATE_DATA_MAIN` | `IT_EVENTS`, `IT_FAILED_EVENTS` | Updates the `DATA` field of existing Main rows after a re-export attempt |
| `GET_DATA_MAIN` | — | Returns the internal `ty_t_alv_main` table |
| `GET_DATA_SUB` | — | Returns the internal `ty_t_alv_sub` table |
| `GET_ALV_MAIN` | — | Returns the `CL_SALV_TABLE` reference for the Main panel |
| `HANDLE_HOTSPOT_CLICK` | `IV_ROW`, `IV_COLUMN` | Looks up the registered `ty_s_hotspot_handler` by row/column and fires the stored `/HFQ/DXP_IF_VALUE_GETTER` |

#### Main ALV toolbar buttons

| Function code | Tooltip (EN) | Description |
|---------------|--------------|-------------|
| `DISPLAY` | Display Selection | Fill Sub/Data panels with the highlighted Main rows |
| `EXPORT_SELECTION` | Export Selection | Re-export only the selected Main events |
| `REFRESH_PFM_MONITOR` | Refresh Monitor | Re-query selected transaction IDs from the database |
| `EXPORT` (Sub ALV) | Export All Events | Re-export all currently shown Sub events |

#### Column order in Main ALV (positions 1–12)

`TRANSACTION_ID` (1) → `EXT_UI` (2) → `OBJECT_KEY` (3) → `OBJECT_TYPE` (4) → `PROC_DOC_NO` (5) → `KEY_DATE` (6) → `TIMESTAMP` (7) → `SOURCE` (8) → `REASON` (10) → `STATUS` (12)

#### Row coloring in Main ALV

The `STATUS` and `STATUS_TEXT` cells are colored by `refresh_alv_main`:

| Condition | Color |
|-----------|-------|
| `gc_status_export_ok` | Green (col 5, intensity 1) |
| `gc_status_export_error`, `gc_status_data_error`, `gc_status_data_transform_error`, `gc_status_canceled`, `gc_status_event_creation_error` | Red (col 6, intensity 1) |
| All other statuses | Blue (col 2) |

#### Internal (private) types

| Type | Description |
|------|-------------|
| `ty_s_hotspot_handler` | Row + column + `IF_VALUE_GETTER` ref, keyed in a sorted table so click events can look up the correct getter |
| `ty_t_range_pod`, `ty_t_range_trans_id`, `ty_t_range_key_date`, `ty_t_range_status`, `ty_t_range_source`, `ty_t_range_set_name` | RANGE types for selection-screen parameters |

#### SQL query in `SELECT_TRANSACTIONS`

```abap
SELECT FROM /hfq/dxp_t_eve AS event
  LEFT OUTER JOIN euitrans ON euitrans~int_ui = event~object_key
  FIELDS event~*, euitrans~ext_ui
  WHERE euitrans~ext_ui IN @it_range_pod AND ...
  ORDER BY event~created_at DESCENDING
  INTO CORRESPONDING FIELDS OF TABLE @gt_data_main
  UP TO @gv_data_width ROWS.  " omitted when gv_data_width = 0 (= unlimited)
```

*Note: commented-out predicates on `datefrom`/`dateto` for the EUITRANS join suggest a date-range filter was considered but not yet activated.*

---

### `/HFQ/DXP_CL_MONITOR_HANDLER`

**Description**: ALV Handler class for PFM Monitor
**Instantiation**: `CREATE PUBLIC`

Wires SAP ALV events to the `/HFQ/DXP_CL_MONITOR` public API. Holds a single private reference `gr_monitor` set in the constructor.

#### Public constants (ALV function codes)

| Constant | Value | Purpose |
|----------|-------|---------|
| `GC_DISPLAY_SEL` | `'DISPLAY'` | Show selected events in Sub/Data panels |
| `GC_MOVE_LEFT` | `'MOVE_LEFT'` | *Defined but not wired to a handler method* |
| `GC_MOVE_RIGHT` | `'MOVE_RIGHT'` | *Defined but not wired to a handler method* |
| `GC_EXPORT_EVENTS` | `'EXPORT'` | Re-export all Sub events |
| `GC_EXPORT_EVENT_SEL` | `'EXPORT_SELECTION'` | Re-export selected Main events |
| `GC_REFRESH_PFM_MONITOR` | `'REFRESH_PFM_MONITOR'` | Refresh Main ALV data |

#### Event handler methods

| Method | ALV Event | Bound to |
|--------|-----------|----------|
| `DOUBLE_CLICK_MAIN` | `DOUBLE_CLICK` of `CL_SALV_EVENTS_TABLE` | Main ALV |
| `DISPLAY_SEL_MAIN` | `ADDED_FUNCTION` | Main ALV — reacts to `GC_DISPLAY_SEL` |
| `EXPORT_SEL_MAIN` | `ADDED_FUNCTION` | Main ALV — reacts to `GC_EXPORT_EVENT_SEL` |
| `REFRESH_PFM_MONITOR` | `ADDED_FUNCTION` | Main ALV — reacts to `GC_REFRESH_PFM_MONITOR` |
| `EXPORT_EVENTS_SUB` | `ADDED_FUNCTION` | Sub ALV — reacts to `GC_EXPORT_EVENTS` |
| `HOTSPOT_CLICK_DATA` | `LINK_CLICK` | Data ALV — delegates to `monitor->handle_hotspot_click` |

**Export logic** (both `EXPORT_EVENTS_SUB` and `EXPORT_SEL_MAIN`): collects the relevant `/HFQ/DXP_TT_EVENT` rows, calls `/HFQ/DXP_CL_EVENT_HANDLER=>get_instance()->main( iv_no_dialog = abap_false )`, then passes returned events and failed events back to `monitor->update_data_main`.

---

### `/HFQ/DXP_CL_DE_ALV`

**Description**: Data Export Class for ALV
**Superclass**: `/HFQ/DXP_CL_DE_ABSTRACT`
**Instantiation**: `CREATE PUBLIC`
**Implements**: `/HFQ/DXP_IF_DATA_EXPORTER`

A thin specialisation of the abstract data exporter whose sole purpose is to run the export preparation in memory and expose the result for the Data ALV panel without saving to the database.

| Method | Description |
|--------|-------------|
| `CONSTRUCTOR` | Calls `super->constructor()` and sets `gv_save_data = abap_false` |
| `GET_DATA` | Returns `gt_transformed_data` as `TY_T_TRANSFORMED_DATA` |
| `/HFQ/DXP_IF_DATA_EXPORTER~COMMIT_EXPORT` | Intentionally empty — no persistence |
| `PREPARE_EXPORT_CHILD` | Redefined from abstract class — intentionally empty |

---

## Interfaces

### `/HFQ/DXP_IF_MON_CONSTANTS`

**Description**: Constants for PFM Monitor
**Visibility**: Public (`EXPOSURE = 2`)

A pure constants interface used by `/HFQ/DXP_CL_MONITOR` for column names, type names, and structural names needed to set up the dynamic Data ALV.

| Constant | Value | Description |
|----------|-------|-------------|
| `GC_STRUCT_NAME_COLOR` | `'LVC_T_SCOL'` | Structure name for the ALV color column |
| `GC_STRUCT_NAME_CELL_TYPE` | `'SALV_T_INT4_COLUMN'` | Structure name for the ALV cell-type column |
| `GC_FIELD_NAME_COLOR` | `'COLOR'` | Field name of the color column |
| `GC_FIELD_NAME_CELL_TYPE` | `'CELL_TYPE'` | Field name of the cell-type column |
| `GC_FIELD_NAME_DESCRIPTION` | `'FIELD_DESCRIPTION'` | Field name for the field description column in Data ALV |
| `GC_TABLE_NAME_DESCRIPTION` | `'TABLE_DESCRIPTION'` | Field name for the table description column in Data ALV |
| `GC_ELEM_TYPE_TABLE_DESCRIPTION` | `'/HFQ/DXP_E_TABLE_DESCRIPTION'` | DDIC type for table description |
| `GC_ELEM_TYPE_FIELD_DESCRIPTION` | `'/HFQ/DXP_E_FIELD_DESCRIPTION'` | DDIC type for field description |
| `GC_ELEM_TYPE_REASON` | `'/HFQ/DXP_E_REASON'` | DDIC type for reason fixed values |
| `GC_ELEM_TYPE_STATUS` | `'/HFQ/DXP_E_STATUS'` | DDIC type for status fixed values |
| `GC_ELEM_TYPE_SOURCE` | `'/HFQ/DXP_E_EVENT_SCENARIO'` | DDIC type for source/scenario fixed values |
| `GC_TRANSFORMATION_TABLE_NAME` | `'/HFQ/DXP_T_IEC'` | Name of the IEC transformation customising table |
| `GC_SOURCE_TEXT_ERROR` | `'<Fixed source value not found.>'` | Fallback text when source fixed value is missing |
| `GC_STATUS_TEXT_ERROR` | `'<Fixed status value not found.>'` | Fallback text when status fixed value is missing |
| `GC_REASON_TEXT_ERROR` | `'<Fixed reason value not found.>'` | Fallback text when reason fixed value is missing |
| `GC_COLUMN_NAME_TRANSACTION_ID` | `'TRANSACTION_ID'` | LVC field name |
| `GC_COLUMN_NAME_EXT_UI` | `'EXT_UI'` | LVC field name |
| `GC_COLUMN_NAME_OBJECT_KEY` | `'OBJECT_KEY'` | LVC field name |
| `GC_COLUMN_NAME_OBJECT_TYPE` | `'OBJECT_TYPE'` | LVC field name |
| `GC_COLUMN_NAME_PROC_DOC_NO` | `'PROC_DOC_NO'` | LVC field name |
| `GC_COLUMN_NAME_KEY_DATE` | `'KEY_DATE'` | LVC field name |
| `GC_COLUMN_NAME_TIMESTAMP` | `'TIMESTAMP'` | LVC field name |
| `GC_COLUMN_NAME_SOURCE` | `'SOURCE'` | LVC field name |
| `GC_COLUMN_NAME_REASON` | `'REASON'` | LVC field name |
| `GC_COLUMN_NAME_STATUS` | `'STATUS'` | LVC field name |

---

## Reports

### `/HFQ/DXP_TRANS_MONITOR`

**Description**: PFM Transaction Monitor (executable report, `SUBC = 1`)
**Transaction**: `/HFQ/DXP_TRANSMON`

The main program is kept intentionally minimal and delegates everything to include programs and `lcl_pfm_transmon`.

#### Program structure (includes)

| Include | Type | Role |
|---------|------|------|
| `_TOP` (`SUBC = I`) | Data declarations | Defines `REPORT`, constant `gc_tcode = '/HFQ/DXP_TCODE'`, and global variables `gs_event`, `gv_pod`, `gv_conv_set_name`; defers `lcl_pfm_transmon` |
| `_S01` (`SUBC = I`) | Selection screen | Declares selection-screen blocks *top* and *bottom* |
| `_D01` (`SUBC = I`) | Class definition | Defines local class `lcl_pfm_transmon` with static method `start_of_selection` |
| `_M01` (`SUBC = I`) | Class implementation | Implements `start_of_selection` |

#### Selection screen (include `_S01`)

**Block top** (with frame):

| Screen element | Field | Type | Description |
|----------------|-------|------|-------------|
| `S_TID` | `gs_event-transaction_id` | SELECT-OPTIONS | Transaction ID range |
| `S_DATE` | `gs_event-timestamp` | SELECT-OPTIONS | Timestamp range |
| `S_POD` | `gv_pod` (type `EXT_UI`) | SELECT-OPTIONS | POD (external utility installation ID) range |
| `S_STAT` | `gs_event-status` | SELECT-OPTIONS | Status range |
| `S_SRC` | `gs_event-scenario` | SELECT-OPTIONS | Source/scenario range |
| `S_SETNAM` | `gv_conv_set_name` | SELECT-OPTIONS NO INTERVALS | Conversion set name filter |

**Block bottom** (with frame):

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `P_MAXREC` | `DDSHMAXREC` | 100 | Maximum number of rows to retrieve (0 = unlimited) |

#### `START-OF-SELECTION` logic

1. Performs authority check `S_TCODE` / `TCD` for `/HFQ/DXP_TCODE`; issues message `A172(00)` on failure.
2. Calls `lcl_pfm_transmon=>start_of_selection`.

#### `lcl_pfm_transmon=>start_of_selection`

```abap
DATA(lr_monitor) = /hfq/dxp_cl_monitor=>get_instance(
    ir_container      = cl_gui_container=>default_screen
    iv_max_sel_rows   = p_maxrec
    it_range_set_name = s_setnam[] ).

lr_monitor->select_transactions( it_range_pod      = s_pod[]
                                 it_range_trans_id = s_tid[]
                                 it_range_key_date = s_date[]
                                 it_range_status   = s_stat[]
                                 it_range_source   = s_src[] ).
lr_monitor->display( ).
WRITE space.  " forces the ALV into the list container
```

---

## Other Objects

### Transaction `/HFQ/DXP_TRANSMON`

| Attribute | Value |
|-----------|-------|
| Transaction code | `/HFQ/DXP_TRANSMON` |
| Program | `/HFQ/DXP_TRANS_MONITOR` |
| Dynpro | 1000 |
| Description (EN) | Transaction Monitor for PFM |
| Description (DE) | Transaktions-Monitor |

### Message Class `/HFQ/DXP_MC_MON`

**Description**: Nachrichten für Monitoring (PFM) / Messages for Monitoring (PFM)
**Master language**: English

| Nr | EN text | DE text |
|----|---------|---------|
| 000 | Field-Symbol not assigned. | Feld-Symbol nicht zugewiesen. |

Used by `/HFQ/DXP_CL_MONITOR` (`MSG_ID` in the class definition). Message 000 is raised as type `E` when the dynamic field-symbol assignment fails in `prepare_alv`.

---

## Domains and Data Elements

All five data elements are defined with master language `D` (German) and have both English and German label texts.

| Data element | Domain | Description (EN) | Label (EN) |
|--------------|--------|-----------------|------------|
| `/HFQ/DXP_E_FIELD_DESCRIPTION` | `TEXT50` | Description of (Database) Table | Field Description |
| `/HFQ/DXP_E_REASON_TEXT` | `DDTEXT` | Description of fixed reason values | Reason text |
| `/HFQ/DXP_E_SOURCE_TEXT` | `DDTEXT` | Description of fixed source values | Source text |
| `/HFQ/DXP_E_STATUS_TEXT` | `DDTEXT` | Description of fixed status values | Status text |
| `/HFQ/DXP_E_TABLE_DESCRIPTION` | `AS4TEXT` | Description of Database Table | Table Description |

These data elements type the human-readable text columns (`source_text`, `reason_text`, `status_text`) in `ty_s_alv_main` and the table/field description columns of the dynamic Data ALV.

*Note: `/HFQ/DXP_E_REASON_TEXT` is defined and used as a structural type in `ty_s_alv_main`, but the code that resolves reason fixed values is commented out in `SELECT_TRANSACTIONS` and `refresh_alv_main` — the `reason_text` column will always be empty at runtime.*

---

## Design Notes

- **Dynamic column count**: the Data ALV column count equals `IV_MAX_SEL_ROWS` (default 100). Each column is named `SUB_INDEX_<n>` and is initially hidden; columns are made visible on demand as sub-indices are loaded. Column headers are set to `<EXT_UI>(<n>)` to show which transaction each column belongs to.
- **Hotspot / button cells**: when a data field holds a reference to `/HFQ/DXP_IF_VALUE_GETTER` (rather than a scalar), the Data ALV cell is rendered as a button (`ICON_SELECT_DETAIL → [...]`). A sorted internal table `gt_hotspot_handler` maps row + column to the getter so `HANDLE_HOTSPOT_CLICK` can invoke it.
- **`GC_MOVE_LEFT` / `GC_MOVE_RIGHT`**: these two constants are defined in `/HFQ/DXP_CL_MONITOR_HANDLER` but no handler method is implemented for them. *Inferred: reserved for a future feature to scroll the visible sub-index columns left/right.*
- **Authority check gap**: the authority check in `START-OF-SELECTION` checks T-code `'/HFQ/DXP_TCODE'` (the value of constant `gc_tcode`), not `'/HFQ/DXP_TRANSMON'` (the actual transaction code). *Inferred: `/HFQ/DXP_TCODE` may be a separate calling transaction, or the constant name is misleading.*
