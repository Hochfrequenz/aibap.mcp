# Package: HFQ / KOMDATA_EDIT

**Description**: Report zum Ändern der eigenen PARTIN-Kontaktdaten
**Original language**: German (D)
**Number of objects**: 11 (1 transaction, 1 executable report, 4 report includes, 5 classes)

---

## Executive Summary

`HFQ/KOMDATA_EDIT` implements the user-facing transaction `/HFQ/KOMDATA` for displaying, editing, and sending PARTIN contact data sheets (Kommunikationsdaten) in the energy market context (edi@energy PARTIN format). The package was created by Hochfrequenz Unternehmensberatung GmbH in December 2021.

The architecture is a classic ABAP module-pool / dynpro application built on top of the domain object `/HFQ/CL_PARTIN` (defined in a separate package). Screen 0100 hosts four embedded `CL_GUI_ALV_GRID` controls placed side by side and stacked vertically:

| ALV control | Container name | Data category |
|---|---|---|
| Availability times | `CONT_ALV_AVAILABILITY` | Erreichbarkeitszeiten |
| Balancing groups | `CONT_ALV_BILANZKREIS` | Bilanzkreise |
| Contact data | `CONT_ALV_CONTACTS` | Kontaktdaten |
| Bank data | `CONT_ALV_BANK_DATA` | Bankdaten |

Each ALV is managed by a concrete subclass of the abstract base class `/HFQ/CL_KD_ALVHANDLER`. All user-command dispatching (toolbar buttons, edit/display toggle, save, send, validation) is centralised in the static-method-only class `/HFQ/CL_KD_UCOMM_HANDLER`.

Key behavioural traits:
- Edit mode is locked via enqueue object `ENQUEUE_/HFQ/KOMDATA_SP` / `DEQUEUE_/HFQ/KOMDATA_SP` per serviceid + version.
- Changed cells are highlighted in colour (yellow for changed vs. previous version, grey for read-only columns) using per-cell colour tables (`lvc_t_scol`).
- "Mass send" (`SNDM`) triggers version generation, validity-date popup, call to `/HFQ/TRIG_PARTIN_MAS`, marks the version as sent, and cleans up backlog versions.
- "Single send" (`SEND`) is only allowed for already-sent versions; it calls `/HFQ/TRIG_PARTIN_SND` after a receiver-selection popup.
- Own service provider check and Sparte (commodity) applicability check gate the edit mode.
- Balancing-group editability depends on whether the sender's service type is a Lieferant (determined dynamically via `/HFQ/MAND_BIKR` and `/HFQ/INTCODES`).

---

## Classes

### `/HFQ/CL_KD_ALVHANDLER` — Abstract ALV Handler Base Class

**File**: `src/#hfq#cl_kd_alvhandler.clas.abap` / `.xml`
**Modifiers**: `PUBLIC ABSTRACT`

The base class that owns the `CL_GUI_CUSTOM_CONTAINER` and `CL_GUI_ALV_GRID` instances and wires their events. Concrete subclasses implement the `UPDATE_VIEW` abstract method to populate and refresh their specific data.

#### Key attributes

| Name | Type | Description |
|---|---|---|
| `GR_ALV_GRID` | `REF TO CL_GUI_ALV_GRID` | The ALV grid control |
| `GR_CONTAINER` | `REF TO CL_GUI_CUSTOM_CONTAINER` | Dynpro custom container |
| `GR_PARTIN` | `REF TO /HFQ/CL_PARTIN` | Current PARTIN data object |
| `GR_PARTIN_PREV` | `REF TO /HFQ/CL_PARTIN` | Previous-version PARTIN (for diff highlighting) |
| `GT_FIELDCAT` | `LVC_T_FCAT` | ALV field catalogue |
| `GV_EDITABLE` | `ABAP_BOOL` | Read-only public flag |
| `GV_INITIALIZED` | `ABAP_BOOL` | Set by `PREPARE_ALV` on first display |
| `GV_LAYOUT` | `LVC_S_LAYO` | ALV layout structure |

#### Local types (protected)

Four styled display types are defined for each data category, each extending the corresponding flat structure with `color_field TYPE lvc_t_scol` and `style_field TYPE lvc_t_styl`:

- `/HFQ/S_KD_CONT_STYLED` / `/HFQ/T_KD_CONT_STYLED` — contact rows with an additional `button_icon TYPE icon_d`
- `/HFQ/S_KD_AVAIL_STYLED` / `/HFQ/T_KD_AVAIL_STYLED` — availability rows
- `/HFQ/S_KD_BIKREIS_STYLED` / `/HFQ/T_KD_BIKREIS_STYLED` — balancing-group rows
- `/HFQ/S_KD_BANK_STYLED` / `/HFQ/T_KD_BANK_STYLED` — bank rows

#### Methods

| Method | Visibility | Signature summary | Description |
|---|---|---|---|
| `CONSTRUCTOR` | Public | `IV_CONTAINER_NAME TYPE C`, `IR_PARTIN REF TO /HFQ/CL_PARTIN`, `IV_EDITABLE TYPE BOOLEAN` | Creates container + grid, registers event handlers |
| `UPDATE_DATA` | Public | — | Calls `gr_alv_grid->check_changed_data( )` to flush pending ALV edits |
| `SET_PARTIN` | Public | `IR_PARTIN REF TO /HFQ/CL_PARTIN` | Replaces the bound PARTIN object and refreshes the view |
| `SET_EDITABLE` | Public | `IV_EDITABLE TYPE BOOLEAN` | Toggles editability and triggers `UPDATE_VIEW` |
| `UPDATE_VIEW` | Public abstract | — | Implemented by subclasses |
| `LOAD_PREVIOUS` | Protected | — | Lazily loads `GR_PARTIN_PREV` based on `prev_version` from current header |
| `HANDLE_DATA_CHANGED` | Protected event | `ER_DATA_CHANGED`, `E_ONCOMM`, `E_ONF4*` | Base empty implementation; overridden by subclasses |
| `HANDLE_BUTTON_CLICK` | Protected event | `ES_COL_ID`, `ES_ROW_NO` | Base empty implementation; overridden by `_CONT` subclass |
| `HANDLE_ONF4` | Protected event | `E_FIELDNAME`, `E_FIELDVALUE`, `ES_ROW_NO`, `ER_EVENT_DATA`, `ET_BAD_CELLS`, `E_DISPLAY` | Base empty implementation; overridden by `_BANK` subclass |
| `PREPARE_ALV` | Protected | — | Base empty implementation; overridden by subclasses to set layout + field catalogue |
| `CREATE_ALV` | Private | `IV_CONTAINER_NAME TYPE C` | Creates `CL_GUI_CUSTOM_CONTAINER` and `CL_GUI_ALV_GRID`, registers `ENTER` edit event |
| `GET_COLOR_CHG` | Protected static | Returns `LVC_S_COLO` | Returns colour code 3 (yellow, intensity 1) for changed cells |
| `GET_COLOR_FIX` | Protected static | Returns `LVC_S_COLO` | Returns colour code 2 (grey, intensity 0) for read-only cells |

---

### `/HFQ/CL_KD_ALVHANDLER_AVAIL` — Availability Times ALV Handler

**File**: `src/#hfq#cl_kd_alvhandler_avail.clas.abap` / `.xml`
**Modifiers**: `PUBLIC FINAL INHERITING FROM /HFQ/CL_KD_ALVHANDLER`

Handles the "Erreichbarkeit" (availability / reachability times) section of screen 0100.

#### Constants

| Name | Value | Description |
|---|---|---|
| `GC_DEFAULT_CONTAINER_NAME` | `'CONT_ALV_AVAILABILITY'` | Dynpro container name |
| `GC_STRUC_TYPE` | `'/HFQ/S_KD_AVAIL_ALL'` | Structure used to build the field catalogue |
| `GC_TITLE` | `'Erreichbarkeit'` | ALV grid title |

#### Protected data

| Name | Type | Description |
|---|---|---|
| `GT_DATA` | `/HFQ/T_KD_AVAIL_STYLED` | Display table bound to the grid |
| `GT_DATA_PREV` | `/HFQ/T_KD_AVAIL_ALL` | Snapshot of previous-version data for diff colouring |

#### Columns displayed (field catalogue)

| Column | Position | Editable in edit mode |
|---|---|---|
| `TIME_TYPE_TEXT` | 1 (key) | No (always grey) |
| `VON` | 2 | Yes |
| `BIS` | 3 | Yes |

All other fields hidden (`no_out`).

#### Behaviour

- `PREPARE_ALV`: Builds layout with `cwidth_opt='A'`, no toolbar, colour table `COLOR_FIELD`, no row insert/delete. Calls `LVC_FIELDCATALOG_MERGE` against `/HFQ/S_KD_AVAIL_ALL`.
- `UPDATE_VIEW`: Fetches rows from `gr_partin->get_avail_all( )`, deletes rows with empty `avail_data` when in display mode, sorts by `time_type ASC, von DESC`. Loads previous version for diff, then colour-marks changed fields using reflection (`CL_ABAP_STRUCTDESCR`). Refreshes grid with stable row/column scroll position.
- `HANDLE_DATA_CHANGED`: Iterates modified rows from `er_data_changed->mp_mod_rows` and calls `gr_partin->set_avail( )` for each.

---

### `/HFQ/CL_KD_ALVHANDLER_BANK` — Bank Data ALV Handler

**File**: `src/#hfq#cl_kd_alvhandler_bank.clas.abap` / `.xml`
**Modifiers**: `PUBLIC FINAL INHERITING FROM /HFQ/CL_KD_ALVHANDLER`

Handles the "Bankdaten" section.

#### Constants

| Name | Value | Description |
|---|---|---|
| `GC_DEFAULT_CONTAINER_NAME` | `'CONT_ALV_BANK_DATA'` | Dynpro container name |
| `GC_STRUC_TYPE` | `'/HFQ/S_KD_BANK_ALL'` | Structure for field catalogue |
| `GC_TITLE` | `'Bankdaten'` | ALV grid title |

#### Protected data

| Name | Type |
|---|---|
| `GT_DATA` | `/HFQ/T_KD_BANK_STYLED` |
| `GT_DATA_PREV` | `/HFQ/T_KD_BANK_ALL` |

#### Columns displayed (field catalogue)

| Column | Position | Editable | Notes |
|---|---|---|---|
| `BANK_TYPE_TEXT` | 1 (key) | No | Always grey |
| `BKVID` | 2 | Yes (in edit mode) | Custom F4 help |
| `ACC_HOLDER` | 3 | Conditional | Only editable if customizing flag `manual_bank_edit` is set |
| `IBAN` | 4 | Conditional | Same |
| `BIC` | 5 | Conditional | Same |
| `BANK_NAME` | 6 | Conditional | Same |

The flag is read from `/HFQ/CL_PARTIN_DB=>query_general_cust( /HFQ/CL_PARTIN_DB=>GC_C_MANUAL_BANK_EDIT )`.

#### Behaviour

- `PREPARE_ALV`: Builds layout, excludes many toolbar buttons (max, min, check, refresh, cut, copy, paste, undo, sum, subtot, graph, info, average). Registers F4 for `BKVID` field (`register_f4_for_fields`).
- `HANDLE_ONF4` for `BKVID`: Suppresses standard F4, fetches bank accounts from `/HFQ/CL_PARTIN_HELPER=>get_but_bank_accs_for_sp( )`, joins with BNKA data, and shows a custom value popup via `F4IF_INT_TABLE_VALUE_REQUEST`. Returns the selected `BKVID` as a modification record.
- `HANDLE_DATA_CHANGED`: If `BKVID` is cleared for a row that had a value, also clears `bank_data`; then calls `gr_partin->set_bank( )` and calls `update_view( )` to re-render.
- `UPDATE_VIEW`: Applies grey colour to `BANK_TYPE_TEXT`; if a row has a non-initial `BKVID`, the derived fields (`ACC_HOLDER`, `IBAN`, `BIC`, `BANK_NAME`) are also coloured grey. For balancing-group type `Z32` ("sonstige"), it matches by both `bank_type + iban` against the previous version; otherwise by `bank_type` alone.

---

### `/HFQ/CL_KD_ALVHANDLER_BIKR` — Balancing Group ALV Handler

**File**: `src/#hfq#cl_kd_alvhandler_bikr.clas.abap` / `.xml`
**Modifiers**: `PUBLIC FINAL INHERITING FROM /HFQ/CL_KD_ALVHANDLER`

Handles the "Bilanzkreis" (balancing group) section.

#### Constants

| Name | Value |
|---|---|
| `GC_DEFAULT_CONTAINER_NAME` | `'CONT_ALV_BILANZKREIS'` |
| `GC_STRUC_TYPE` | `'/HFQ/S_KD_BIKREIS_ALL'` |
| `GC_TITLE` | `'Bilanzkreis'` |

#### Protected data

| Name | Type |
|---|---|
| `GT_DATA` | `/HFQ/T_KD_BIKREIS_STYLED` |
| `GT_DATA_PREV` | `/HFQ/T_KD_BIKREIS_ALL` |

#### Columns displayed

| Column | Position | Editable |
|---|---|---|
| `BIKREIS_TYPE_TXT` | 1 (key) | No |
| `SETTLEUNIT` | 2 | Yes |

#### Behaviour

- `PREPARE_ALV`: No toolbar, no row insert/delete. Layout with colour column.
- `HANDLE_DATA_CHANGED`: Calls `gr_partin->set_bikreis( )` for each modified row.
- `UPDATE_VIEW`: Fetches via `get_bikreis_all( )`, deletes rows where `bikreis_data IS INITIAL` in display mode, sorts by `bikreis_type ASC, counter ASC`. Previous-version diff applied generically. Key `BIKREIS_TYPE_TXT` always grey.

*Note*: Editability of this ALV is conditionally restricted to Lieferant-type service providers at the screen level (module `FILL_DATA`), independently of this class's own `set_editable` method.

---

### `/HFQ/CL_KD_ALVHANDLER_CONT` — Contact Data ALV Handler

**File**: `src/#hfq#cl_kd_alvhandler_cont.clas.abap` / `.xml`
**Modifiers**: `PUBLIC FINAL INHERITING FROM /HFQ/CL_KD_ALVHANDLER`

Handles the "Kontaktdaten" section.

#### Constants

| Name | Value |
|---|---|
| `GC_DEFAULT_CONTAINER_NAME` | `'CONT_ALV_CONTACTS'` |
| `GC_STRUC_TYPE` | `'/HFQ/S_KD_CONTACT_ALL'` |
| `GC_TITLE` | `'Kontaktdaten'` |

#### Protected data

| Name | Type |
|---|---|
| `GT_DATA` | `/HFQ/T_KD_CONT_STYLED` |
| `GT_DATA_PREV` | `/HFQ/T_KD_CONTACT_ALL` |

#### Columns displayed

| Column | Position | Editable | Notes |
|---|---|---|---|
| `CONTACT_TYPE_TEXT` | 1 (key) | No | Always grey |
| `INFO_CONTACT` | 2 | Yes | |
| `EMAIL` | 3 | Yes | |
| `TEL` | 4 | Yes | |
| `FAX` | 5 | Yes | |
| `CONTACT` | 6 | Conditional | Disabled (style + grey) when using default contact |
| `ADDRESS` | 7 | Conditional | Same |
| `HOUSE_NUM` | 8 | Conditional | Same |
| `POST_CODE` | 9 | Conditional | Same |
| `CITY` | 10 | Conditional | Same |
| `COUNTRY_CODE` | 11 | Conditional | Same |
| `BUTTON_ICON` | 12 | — | Hidden in display mode; icon button to toggle external address |

The icon button column uses `MC_STYLE_BUTTON` and `MC_STYLE_DISABLED` cell styles.

#### Behaviour

- `PREPARE_ALV`: Builds layout with `stylefname='STYLE_FIELD'` (per-cell style control) and `ctab_fname='COLOR_FIELD'`. Dynamically appends `BUTTON_ICON` field to field catalogue with tooltip "Externe Addresse bearbeiten/löschen". Excludes standard toolbar buttons.
- `HANDLE_BUTTON_CLICK`: If `BUTTON_ICON` column clicked, toggles `contact_id` between the default constant (`/HFQ/IF_PARTIN_CONSTANTS=>GC_DEFAULT_CONTACT_ID`) and empty, then calls `gr_partin->set_cont( )` and refreshes view.
- `HANDLE_DATA_CHANGED`: Calls `gr_partin->set_cont( )` for each modified row.
- `UPDATE_VIEW`: Deletes rows where both `contact_addr` and `contact_comm` are initial (display mode), sorts by `contact_type ASC, info_contact DESC`. Rows whose `contact_id` equals the default ID show icon `@0Z@` (ICON_CHANGE) and have the address fields disabled/greyed; others show `@2W@` (ICON_SYSTEM_UNDO) with normal cells.

---

### `/HFQ/CL_KD_UCOMM_HANDLER` — User Command Handler

**File**: `src/#hfq#cl_kd_ucomm_handler.clas.abap` / `.xml`
**Modifiers**: `PUBLIC FINAL` (all methods are class methods; no instantiation needed)

Central dispatcher for all toolbar function codes. Uses `POPUP_TO_CONFIRM` and `POPUP_GET_VALUES` for user interaction.

#### Protected class data

| Name | Type | Description |
|---|---|---|
| `GR_PREVIOUS` | `REF TO /IDXGC/CX_GENERAL` | Catches last exception for message forwarding |
| `GV_MTEXT` | `STRING` | *Inferred: message text buffer (not directly used in implemented code)* |
| `GC_ANSWER_YES` | `C VALUE '1'` | POPUP_TO_CONFIRM answer constant |
| `GC_ANSWER_NO` | `C VALUE '2'` | POPUP_TO_CONFIRM answer constant |
| `GC_ANSWER_ABORT` | `C VALUE 'A'` | POPUP_TO_CONFIRM abort answer |

#### Methods

| Method | Signature summary | Description |
|---|---|---|
| `ON_CHCK` | `IR_PARTIN` → `ET_ERROR /HFQ/T_KD_ERROR`, `RV_SUCCESS BOOLEAN` | Calls `ir_partin->check_data( iv_strict = false )`; shows error popup if errors found |
| `ON_LEAVE_DRAFT` | `IR_PARTIN` → `RV_SUCCESS BOOLEAN` | If status is `UNSAVED`, asks user to save or discard; on "Yes" calls `save_data(draft)`, on "No" reloads previous sent version |
| `ON_SEND` | `IR_PARTIN`, `CV_RELOAD`, `CV_EDITABLE` → `RV_SUCCESS BOOLEAN` | Single-send: only allowed if status is `SENT`; opens receiver selection popup; triggers `/HFQ/TRIG_PARTIN_SND` for each selected receiver |
| `ON_SNDM` | `IR_PARTIN`, `CV_RELOAD`, `CV_EDITABLE` → `RV_SUCCESS BOOLEAN` | Mass-send: validates, saves draft, asks version confirmation, generates new version via `/HFQ/CL_PARTIN_HELPER=>GENERATE_VERSION`, opens validity popup, calls `prepare_for_send`, triggers `/HFQ/TRIG_PARTIN_MAS`, marks as sent, saves, deletes backlog versions |
| `ON_SWTC` | `IR_PARTIN`, `CV_EDITABLE`, `CV_RELOAD` | Toggle edit/display: checks own-service-provider + Sparte eligibility; if entering edit mode, offers to load existing draft or create new draft; if leaving edit mode with unsaved data, calls `ON_LEAVE_DRAFT` |
| `POPUP_ASK_FOR_SAVE` | `IR_PARTIN` → `RV_ANSWER BOOLEAN` | Shows "Save changes to draft?" confirm dialog |
| `POPUP_CONFIRM_GENERATE` | — → `RV_ANSWER BOOLEAN` | Shows "Really generate new version?" confirm dialog |
| `POPUP_DISPLAY_ERROR` | `IT_ERROR /HFQ/T_KD_ERROR` → `RV_SUCCESS BOOLEAN` | Displays error table in `POPUP_WITH_TABLE_DISPLAY_OK` |
| `POPUP_GET_RECEIVER` | — → `RT_RECEIVERS T_SERVICEID` | Opens `FREE_SELECTIONS_DIALOG` for `ESERVPROV-SERVICEID`; returns selected service IDs |
| `POPUP_GET_VALIDITY` | `IV_VERSION` optional, `CV_PREV_VERSION` optional → `EV_DATE_FROM /HFQ/DE_KD_VON` | Opens `POPUP_GET_VALUES` for validity-from date and predecessor version; validates that `PREV_VERSION < IV_VERSION` |

---

## Reports

### `/HFQ/RP_KOMDATA` — Main Report (Executable Program)

**Files**:
- `src/#hfq#rp_komdata.prog.abap` — Main program (entry point + include list)
- `src/#hfq#rp_komdata.prog.xml` — Program metadata + dynpro 0100 definition
- `src/#hfq#rp_komdata.prog.screen_0100.abap` — Screen flow logic for dynpro 0100

**Transaction**: `/HFQ/KOMDATA` — "Anzeige und Senden von Kontaktdaten"

The report is a pure shell that calls screen 0100 and then includes the four sub-includes. The actual screen logic lives in the includes.

#### Entry parameter

| Parameter | Type | Description |
|---|---|---|
| `P_SERVID` | `SERVICE_PROV` | Service provider ID (Serviceanbieter) whose PARTIN data is loaded |

#### Global variables (from TOP include)

| Variable | Type | Description |
|---|---|---|
| `GR_PARTIN` | `REF TO /HFQ/CL_PARTIN` | Current PARTIN domain object |
| `GV_EDITABLE` | `ABAP_BOOL` | Current edit/display mode flag |
| `GV_RELOAD` | `ABAP_BOOL` | Signals ALV handlers to refresh data |
| `GS_HEADER` | `/HFQ/S_KD_HDR` | Current header data (displayed/edited on screen) |
| `ALV_HANDLER_CONT` | `REF TO /HFQ/CL_KD_ALVHANDLER_CONT` | |
| `ALV_HANDLER_AVAIL` | `REF TO /HFQ/CL_KD_ALVHANDLER_AVAIL` | |
| `ALV_HANDLER_BIKREIS` | `REF TO /HFQ/CL_KD_ALVHANDLER_BIKR` | |
| `ALV_HANDLER_BANK` | `REF TO /HFQ/CL_KD_ALVHANDLER_BANK` | |

#### Function codes (toolbar buttons)

| Code | Key | Action |
|---|---|---|
| `BACK` | F3 | Leave screen (with draft check) |
| `EXIT` | Shift+F3 | Leave program (with draft check) |
| `CANC` | F12 | Cancel screen |
| `SWTC` | — | Toggle edit / display mode |
| `SAVE` | — | Save as draft locally |
| `UNDO` | — | Delete draft, reload current sent version |
| `CHCK` | — | Validate data |
| `SEND` | — | Single-send to selected receivers |
| `SNDM` | — | Generate version and mass-send |
| `RCNT` | — | Reload current valid version (display mode only) |

#### Dynpro 0100 layout

Screen `0100` (60 rows × 202 columns, next screen loops to itself):

- Header frame (rows 1–15): serviceid, version input field (with F4 help from `/HFQ/PTCOMV`), version validity text, service-provider name, status display
- `CONT_ALV_AVAILABILITY` custom control: row 5, col 102, 32 × 9
- `CONT_ALV_BILANZKREIS` custom control: row 5, col 146, 40 × 9
- `CONT_ALV_CONTACTS` custom control: row 17, col 3, 196 × 15
- `CONT_ALV_BANK_DATA` custom control: row 33, col 3, 196 × 15

Screen group `HDR` fields are editable only in edit mode; screen group `VER` (version) fields are editable only in display mode (to allow version navigation).

---

### `/HFQ/RP_KOMDATATOP` — TOP Include

**File**: `src/#hfq#rp_komdatatop.prog.abap` / `.xml`

Contains the `REPORT` statement, all `CONSTANTS`, `DATA`, and `PARAMETERS` declarations used throughout the report and its includes.

---

### `/HFQ/RP_KOMDATA_0100_I01` — INPUT Module Include

**File**: `src/#hfq#rp_komdata_0100_i01.prog.abap` / `.xml`

Contains module `USER_COMMAND_0100 INPUT` and helper FORMs:

| FORM | Description |
|---|---|
| `ACCEPT_VERSION` | Validates and loads the version entered in the version field by querying `/HFQ/PTCOMV` |
| `MANAGE_LOCK` | Acquires/releases `ENQUEUE_/HFQ/KOMDATA_SP` based on the current serviceid, version, and edit mode |
| `ACCEPT_DATA` | Delegates to `SET_DATA` and `SET_HEADER` |
| `SET_DATA` | Calls `update_data( )` on all four ALV handlers to flush pending edits |
| `SET_HEADER` | Calls `gr_partin->set_header( gs_header )` |

---

### `/HFQ/RP_KOMDATA_0100_O01` — OUTPUT Module Include

**File**: `src/#hfq#rp_komdata_0100_o01.prog.abap` / `.xml`

Contains three PBO modules:

| Module | Description |
|---|---|
| `STATUS_0100 OUTPUT` | Sets GUI status `PT_EDIT` and dynamically excludes toolbar buttons based on own-provider flag, status, and edit mode; sets titlebar to `PARTIN_DISPLAY` or `PARTIN_EDIT` |
| `CREATE_GUI_OBJECTS OUTPUT` | On first call: instantiates `GR_PARTIN` using `p_servid` + own-service-provider popup, then creates all four ALV handler instances |
| `FILL_DATA OUTPUT` | Populates screen fields from header; determines version validity text; controls screen-field editability via `LOOP AT SCREEN`; conditionally enables Bilanzkreis ALV for Lieferant service types by cross-checking `/HFQ/MAND_BIKR` and `/HFQ/INTCODES`; calls `set_editable` or `update_view` on all handlers depending on whether edit mode or data changed |

---

### `/HFQ/RP_KOMDATA_0100_V01` — Value-Request Include

**File**: `src/#hfq#rp_komdata_0100_v01.prog.abap` / `.xml`

Contains module `VALUE_VERSION INPUT`, which provides F4 help for the version field by selecting all versions for the current service provider from `/HFQ/PTCOMV` and displaying them via `F4IF_INT_TABLE_VALUE_REQUEST`. After selection it programmatically triggers `ENTER` via `SAPGUI_SET_FUNCTIONCODE`.

---

## Other Objects

### Transaction `/HFQ/KOMDATA`

**File**: `src/#hfq#komdata.tran.xml`

| Property | Value |
|---|---|
| Transaction code | `/HFQ/KOMDATA` |
| Linked program | `/HFQ/RP_KOMDATA` |
| Initial screen | `1000` (*Note: tran.xml shows 1000, but the report calls screen 0100 — the discrepancy is in the transaction metadata vs. actual program flow*) |
| Description | Anzeige und Senden von Kontaktdaten |

---

## External Dependencies (referenced, not defined in this package)

| Object | Type | Description |
|---|---|---|
| `/HFQ/CL_PARTIN` | Class | PARTIN domain object providing get/set methods for all data categories |
| `/HFQ/CX_PARTIN_ERROR` | Exception class | Standard PARTIN processing exception |
| `/HFQ/CL_PARTIN_DB` | Class | Database access for PARTIN data and customizing |
| `/HFQ/CL_PARTIN_HELPER` | Class | Helper: version management, F4 popups, service provider checks |
| `/HFQ/IF_PARTIN_CONSTANTS` | Interface | Named constants for version statuses, bank type codes, contact IDs |
| `/HFQ/PTCOMV` | Table | PARTIN version table |
| `/HFQ/MAND_BIKR` | Table | Customizing: balancing-group sender type assignments |
| `/HFQ/INTCODES` | Table | Internal codes (service type mapping) |
| `ENQUEUE_/HFQ/KOMDATA_SP` | Function | Lock object enqueue for PARTIN editing |
| `DEQUEUE_/HFQ/KOMDATA_SP` | Function | Lock object dequeue |
| `/HFQ/TRIG_PARTIN_SND` | Function | Trigger single PARTIN send CL-process |
| `/HFQ/TRIG_PARTIN_MAS` | Function | Trigger mass PARTIN send CL-process |
| `/IDXGC/CX_GENERAL` | Exception class | IDX general exception (base for `GR_PREVIOUS` in `CL_KD_UCOMM_HANDLER`) |
