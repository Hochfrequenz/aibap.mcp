# Package: HFQ / BO4E

**Description**: Business Objects for Energy
**Original language**: German (master language `D`; abapGit export master `E`)
**Component**: IS-U (I110004800)
**Number of objects**: 371 files (source directory), covering ~140 distinct ABAP objects

---

## Executive Summary

HFQ/BO4E is a Hochfrequenz custom IS-U add-on that maps SAP Utilities master data to the [BO4E](https://www.bo4e.de) (Business Objects for Energy) standard format, exposing German energy-market objects — Marktlokation (MaLo), Messlokation (MeLo), Energiemenge, Benachrichtigung (EMMA case), Geschäftspartner, Vertrag, Zähler, and related components — via OData V2 services. The core approach uses CDS views (`@OData.publish: true`) backed by IS-U database tables (EUITRANS, EUIINSTLN, EANL, EMMA\_CASE, EPROFHEAD, etc.) with an enum-mapping table (`/HFQ/DB_BO_ENUM`) that translates SAP domain values to BO4E string constants. Post-processing of OData results is done through a BAdI (`/HFQ/BO4E_POST_PROCESSING`) whose default implementations are no-ops; a custom REST API (`/HFQ/CL_BO4E_ACTIONS`) exists in parallel for EMMA case Benachrichtigung and Aufgabe actions. The package contains known incomplete areas: `bilanzierungsmethode` is hardcoded as `'DUMMY'`; `lokationstyp` is hardcoded as `'DUMMY'`; DST-conversion code in `CONVERT_GERMAN_LOCAL_AND_UTC` replaced an earlier commented-out manual Sunday-calculation with an IETF-based approach (*verified in implementation*).

---

## Classes

### `/HFQ/BO4E_TOOLS` — Utility class (final, public, no-instance required)

Static helper methods for date/time conversion and string formatting. All methods are class-methods.

| Method | Signature | Verified behaviour |
|---|---|---|
| `GERMAN_LOCAL_DATETIME_TO_UTC` | `iv_string TYPE string → rv_string TYPE string` | Delegates to `convert_german_local_and_utc` with `utc_to_local = false`. Converts `YYYYMMDDHHMMSS` German local time to UTC using IETF DST data for timezone `CET`. Returns `'00000000000000'` on invalid input. |
| `UTC_TO_GERMAN_LOCAL_DATETIME` | `iv_string TYPE string → rv_string TYPE string` | Same, with `utc_to_local = true`. |
| `IS_GERMAN_DST` | `iv_date TYPE dats, iv_time TYPE uzeit, is_utc TYPE abap_bool → rv_dst TYPE abap_bool` | Calls `TZON_INTERNAL_TO_IETF` for CET/year, parses DAYLIGHT/STANDARD DTSTART lines, compares local or converted-UTC datetime. Returns true when within CET summer time. |
| `GET_LAST_SUNDAY` | `iv_month TYPE string, iv_year TYPE string → rv_last_sunday TYPE string` | Iterates from day 21 forward calling `DATE_COMPUTE_DAY` (function module), returns last date in month whose weekday = 7. *Used only in old commented-out DST code; retained in public section but no longer called internally.* |
| `YYYYMMDDHHMMSS_TO_ISO_UTC` | `iv_yyyymmddhhmmss TYPE string → rv_iso TYPE string` | Regex-parses `YYYYMMDDHHMMSS`, returns `YYYY-MM-DDTHH:MM:SSZ`. |
| `ISO_UTC_TO_YYYYMMDDHHMMSS` | `iv_iso TYPE string → rv_yyyymmddhhmmss TYPE string` | Reverse: parses ISO 8601 UTC string, returns concatenated `YYYYMMDDHHMMSS`. |
| `NUMC_TO_CASE_NR10` | `iv_casenr TYPE string, iv_target_length TYPE i default 10 → rv_casenr_leading_zeros TYPE string` | Pads/strips to exactly `iv_target_length` digits with leading zeros. EMMA case numbers need 10 digits; leading-zero stripping is done in some places by the system which is incompatible with direct DB reads. |
| `STRING_TO_OPBEL_KEY` | `iv_string TYPE string, iv_target_length TYPE i default 12 → rv_opbel_leading_zeros TYPE string` | Delegates to `NUMC_TO_CASE_NR10` with `target_length = 12`. |
| `USER_EXISTS` | `iv_username TYPE xubname → rv_exists TYPE abap_bool` | Calls `BAPI_USER_EXISTENCE_CHECK`; returns true only if return `type='I'`, `id='01'`, `number='088'`. |
| `STXL_TO_COM_NOTIZ` | `iv_tdid, iv_tdname, iv_tdobject TYPE stxl-*, iv_language TYPE thead-tdspras → et_notizen TYPE tt_notiz` | Reads SAP long text via `READ_TEXT`, splits by header lines matching `DD.MM.YYYY HH:MM <name>` regex, converts timestamps from German local to ISO UTC. |
| `ADD_NOTE_TO_EMMA_CASE` | `iv_date TYPE uzeit, iv_time TYPE dats, iv_emma_case_no TYPE string, iv_note_text TYPE string → rv_success TYPE abap_bool` | Authority-checks `cl_emma_case=>co_auth_object`, reads EMMA case via `cl_emma_dbl`, appends separator + header + text, calls `change_case`. |

**Type definitions (public):**
- `ts_notiz`: `autor TYPE string`, `zeitpunkt TYPE string`, `inhalt TYPE string`
- `tt_notiz TYPE TABLE OF ts_notiz`

**Constants:**
- `LEADING_ZERO_REGEX = '(0*)(\d+)'`
- `GERMAN_DATE_FORMATTER_REGEX = '(\d{1,2})\.(\d{1,2})\.(\d{4})\s(\d{2}):(\d{2})\s*(.*)\s*'`
- `SAP_DATE_FORMATTER_REGEX = '(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})'`
- `ISO_DATE_FORMATTER_REGEX = '(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z?'`
- `MINUSEMPTY_REGEX = '[\s-]*'`
- `ENUM_TYP_PRIORITAET = 'Prioritaet'`
- `ENUM_TYP_BEARBEITUNGSSTATUS = 'Bearbeitungsstatus'`

**Unit tests** exist in `/HFQ/BO4E_TOOLS_TESTS` (local test class in `.testclasses.abap`), covering: `numc_to_case_nr10`, `german_local_datetime_to_utc`, `get_last_sunday`, `yyyymmddhhmmss_to_iso_utc`, `stxl_to_com_notiz`.

---

### `/HFQ/CL_BO4E_01_DPC_EXT` — OData Data Provider Extension (inherits `/HFQ/CL_BO4E_01_DPC`)

The central OData V2 data provider for the main BO4E service. Overrides both `GET_EXPANDED_ENTITY` and `GET_EXPANDED_ENTITYSET` from the SAP SADL gateway base class, adding post-processing for expanded navigation properties.

**Public class-methods (post-processing hooks, all verified as side-effect-free no-ops in the BADI default implementations but containing real logic here):**

| Method | Signature |
|---|---|
| `POST_PROCESS_MARKTLOKATION` | `importing ir_entity TYPE …TS_XHFQXCDS_BO_MARKTLOKATIONTY → r_entity (same type)` |
| `POST_PROCESS_MESSLOKATION` | `importing ir_entity TYPE …TS_XHFQXCDS_BO_MESSLOKATIONTYP → r_entity` |
| `POST_PROCESS_ENERGIEMENGE` | `importing ir_entity TYPE …TS_XHFQXCDS_BO_EMTYPE → r_entity` |
| `POST_PROCESS_BENACHRICHTIGUNG` | `importing ir_entity TYPE …TS_XHFQXCDS_BO_BENACHRICHTIGUT → r_entity` |
| `POST_PROCESS_ENERGIEMENGE_ZS` | `changing cr_entity TYPE …TS_XHFQXCDS_BO_EM_ZAEHLERSTATY` |
| `POST_PROCESS_ENERGIEMENGE_PROF` | `changing cr_entity TYPE …TS_XHFQXCDS_BO_EM_PROFILETYPE` |
| `POST_PROCESS_AUFGABE` | `importing ir_entity TYPE …TS_XHFQXCDS_HLP_AUFGABETYPE → r_entity` |
| `POST_PROCESS_BENACH_INFOS` | `importing iv_emma_case_no TYPE emma_case-casenr; changing ct_infos_field_symbols TYPE ANY TABLE` |

**Verified behaviour of `GET_EXPANDED_ENTITY`:** Delegates first to SADL DPC, then inspects `et_expanded_tech_clauses` for `'TONOTIZEN'`, `'TOINFOS'`, `'TOAUFGABEN'`. For `TONOTIZEN` it calls `STXL_TO_COM_NOTIZ` (using language from `/HFQ/CUSTOMIZING` key `EMMA_CASE_LANGUAGE_STXL`, defaulting to `'EN'`) and replaces the notiz table fields `AUTOR`, `INHALT`, `ZEITPUNKT` in-place via field symbols. Dispatches entity-type-specific post-processing for Benachrichtigung, Messlokation, Marktlokation, Energiemenge, EM_ZS, EM_Profile.

**Protected methods redefined (entity-set CRUD handlers):**
- `xhfqxcds_bo_bena_get_entity`, `xhfqxcds_bo_bena_get_entityset`, `xhfqxcds_bo_bena_update_entity`
- `xhfqxcds_bo_em01_get_entity`, `xhfqxcds_bo_em01_get_entityset`
- `xhfqxcds_bo_em_get_entity`, `xhfqxcds_bo_em_get_entityset`
- `xhfqxcds_bo_mark_get_entity`, `xhfqxcds_bo_mark_get_entityset`
- `xhfqxcds_bo_mess_get_entity`, `xhfqxcds_bo_mess_get_entityset`
- `xhfqxcds_com_not_create_entity`, `xhfqxcds_com_not_get_entityset`
- `xhfqxcds_hlp_auf_get_entity`, `xhfqxcds_hlp_auf_get_entityset`
- `xhfqxcds_profil_get_entityset`

*(The implementations of these protected methods are not fully documented due to package size — the DPC class body exceeds 10,000 tokens.)*

---

### `/HFQ/CL_BO4E_01_MPC` — OData Metadata Provider

Generated by SAP OData framework. Contains type constants used by DPC\_EXT:
- `gc_xhfqxcds_bo_benachrichtigut`, `gc_xhfqxcds_bo_messlokationtyp`, `gc_xhfqxcds_bo_marktlokationty`, `gc_xhfqxcds_bo_emtype`, `gc_xhfqxcds_bo_em_zaehlerstaty`, `gc_xhfqxcds_bo_em_profiletype`

### `/HFQ/CL_BO4E_01_MPC_EXT` — OData Metadata Provider Extension

Inherits `/HFQ/CL_BO4E_01_MPC`. Implementation is empty — no overrides.

### `/HFQ/CL_BO4E_01_DPC` — OData Data Provider Base

Base class generated by SAP OData framework. Extended by `DPC_EXT`.

### `/HFQ/CL_BO4E_ACTIONS` — REST Application Router (inherits `CL_REST_HTTP_HANDLER`)

Routes REST calls to Benachrichtigung and Aufgabe handlers.

**Public method:**
- `IF_REST_APPLICATION~GET_ROOT_HANDLER` (redefinition): Creates a `CL_REST_ROUTER` with routes:
  - `GET/POST /benachrichtigung/{caseid}` → `/HFQ/CL_BO4E_BENA`
  - `GET/POST /` → `/HFQ/CL_BO4E_BENA`
  - `POST /benachrichtigung/{benachrichtigungsId}/aufgabe/{aufgabenId}` → `/HFQ/CL_BO4E_AUFGABE`

### `/HFQ/CL_BO4E_BENA` — REST Resource: Benachrichtigung (inherits `CL_REST_RESOURCE`)

- **GET**: Returns HTTP 501 Not Implemented with message `'Use OData, not REST'` in JSON.
- **POST**: Reads `caseid` URI attribute and request body, deserializes JSON via `/UI2/CL_JSON`, returns HTTP 200 with empty JSON body. *The POST body is deserialized but not actually used — the handler is effectively a stub.*

### `/HFQ/CL_BO4E_AUFGABE` — REST Resource: Aufgabe execution (inherits `CL_REST_RESOURCE`, final)

**POST only.** Executes an EMMA case solution path step.

Verified logic:
1. Extracts `benachrichtigungsId` and `aufgabenId` from URI.
2. Authority-checks `B_EMMA_CAS / ACTVT = 'ACTIV_AUTH'`.
3. Loads EMMA case via `cl_emma_dbl→read_case_detail` (using `NUMC_TO_CASE_NR10` for ID padding).
4. Loops solution path (`get_solpath`) looking for `method = aufgabenId` to get `seqnr`.
5. Calls `cl_emma_case→execute_process( iv_sequnr = seqnr )`.
6. On success (sy-subrc=0): inserts/updates `EMMA_CSOLP` with execution date/time/user; returns HTTP 200.
7. On system_error (sy-subrc=2): uses `/HFQ/AUTH_FAIL_CHECKER` to distinguish auth failures (HTTP 403) from generic server errors (HTTP 500).
8. Handles: process_not_found (404), invalid_case_object (400), case_ccat_not_found (404), dataflow_error (500).

### `/HFQ/AUTH_FAIL_CHECKER` — Authorization failure detector (final, public)

Detects whether failed SU53 auth-checks occurred between construction and invocation.

- **`constructor`**: Snapshots current SU53 buffer via `CL_SUSR_TOOLS_KERNEL→GET_SU53_BUFFER_NO_PARMS` into `gt_reference`.
- **`checks_have_failed`**: Re-reads buffer; if current count > reference count, subtracts reference entries (by `abapline + timestamp` match) and returns `rv_checks_failed = true` with `et_failed_checks`.

### `/HFQ/CL_CDS_BO_EM_PROFILE` — SADL MPC exposure class (final, inherits `CL_SADL_GTK_EXPOSURE_MPC`)

Exposes CDS view `/HFQ/CDS_BO_EM_PROFILE` via the SAP SADL gateway.

- `GET_PATHS`: Returns `( 'CDS~/HFQ/CDS_BO_EM_PROFILE' )`.
- `GET_TIMESTAMP`: Returns fixed timestamp `20191015134950`.

### `/HFQ/CL_SADL_GW_GENERIC_DPC` — Custom SADL gateway data provider (inherits `CL_SADL_GW_GENERIC_DPC`)

Overrides `IF_SADL_GW_DPC~CREATE_ENTITY` to add create-by-association support and proper error propagation from SADL exceptions to MGW exceptions. All internal methods (`_INIT`, `_PREPARE_CREATE`, `_GET_TRANSACT_RUNTIME`, `_RAISE_BUSINESS_EXCEPTION`, etc.) are private overrides of base class patterns. *This class appears to be a patched version of the SAP standard `CL_SADL_GW_GENERIC_DPC` with `CREATE_ENTITY` fix for navigation-based creation.*

### `/HFQ/CL_A_LOCK_HFQ_CDS_BO_MARK` — Lock class for Marktlokation

*Not fully documented — implementation file read but empty for content beyond class definition.*

### `/HFQ/CL_A_LOCK_HFQ_CDS_INSERT0` — Lock class for INSERT_TEST

*Not fully documented — test artifact.*

---

## Interfaces

### `/HFQ/BO4E_POST_PROCESSING_IF` — BAdI interface for OData post-processing

Extends `IF_BADI_INTERFACE`. All methods `CHANGING` the entity struct.

| Method | Changing parameter type |
|---|---|
| `POST_PROCESS_MARKTLOKATION` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_MARKTLOKATIONTY` |
| `POST_PROCESS_MESSLOKATION` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_MESSLOKATIONTYP` |
| `POST_PROCESS_ENERGIEMENGE` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_EMTYPE` |
| `POST_PROCESS_BENACHRICHTIGUNG` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_BENACHRICHTIGUT` |
| `POST_PROCESS_ENERGIEMENGE_ZS` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_EM_ZAEHLERSTATY` |
| `POST_PROCESS_ENERGIEMENGE_PROF` | `/HFQ/CL_BO4E_01_MPC=>TS_XHFQXCDS_BO_EM_PROFILETYPE` |

### `/HFQ/IF_BO4E_SEARCH` — BAdI interface for custom search

Extends `IF_BADI_INTERFACE`. Provides class-methods for searching IS-U objects by free-text string.

| Class-method | Parameters | Returns |
|---|---|---|
| `SEARCH_ENERGIEMENGE` | `iv_search_string TYPE string` | `ET_ANLAGEN TYPE TT_INSTALLATION` |
| `SEARCH_MESSLOKATION` | `iv_search_string TYPE string` | `ET_INT_UIS TYPE TT_INT_UI` |
| `SEARCH_MARKTLOKATION` | `iv_search_string TYPE string` | `ET_INT_UIS TYPE TT_INT_UI` |

**Type definitions:**
- `t_installation`: `anlage TYPE anlage`
- `tt_installation TYPE STANDARD TABLE OF t_installation WITH EMPTY KEY`
- `t_int_ui`: `int_ui TYPE int_ui`
- `tt_int_ui TYPE STANDARD TABLE OF t_int_ui WITH EMPTY KEY`

### `/HFQ/CL_BO4E_SEARCH` — Default BAdI implementation of `/HFQ/IF_BO4E_SEARCH`

All three methods query `EUITRANS JOIN EUIINSTLN` with a `LIKE '%<search>%'` on `ext_ui` (and a truncated version on `anlage` if `strlen(search) >= 10`, to prevent SQL dumps). Filters by current date on `datefrom`/`dateto`. MaLo uses `uistrutyp = 'MA'`; MeLo uses `uistrutyp = 'ME'`; Energiemenge returns `anlage` by joining to the installation.

### `/HFQ/BO4E_POST_PROCESSING_CL` and `/HFQ/BO4E_POST_PROC_UNITY_CL` — Default BAdI implementations

Both implement `/HFQ/BO4E_POST_PROCESSING_IF`. All six post-processing methods contain only `RETURN` — they are verified no-ops. Customer systems are expected to provide custom BAdI implementations.

### `/HFQ/IF_EMMA_CASE_DETAILS` — BAdI interface for EMMA case detail retrieval

Extends `IF_BADI_INTERFACE`. Class-methods for resolving case attributes:

| Method | Importing | Exporting/Returning |
|---|---|---|
| `GET_MARKTTEILNEHMER` | `iv_emma_case_no TYPE emma_cnr`, `iv_category TYPE emma_ccat OPTIONAL` | `ev_service TYPE sercode`; returns `rv_external_id TYPE dunsnr` |
| `GET_MESSLOKATION` | `iv_emma_case_no`, `iv_category OPTIONAL` | returns `rv_ext_ui TYPE ext_ui` |
| `GET_ZEITRAUM` | `iv_emma_case_no`, `iv_category OPTIONAL` | `ev_timestring_from TYPE string`, `ev_timestring_to TYPE string` |
| `GET_PROC_REF` | `iv_emma_case_no`, `iv_category OPTIONAL` | returns `rv_proc_ref TYPE /ape/de_doc_no` |
| `GET_PROCESS_ID` | `iv_emma_case_no`, `iv_category OPTIONAL` | returns `rv_proc_id TYPE /ape/de_proc_id` |

### `/HFQ/IF_CDS_BO_MARKTLOKATION_C` — BOPF constants interface for Marktlokation BO

Contains BOPF key constants (`SC_BO_KEY`, `SC_BO_NAME`, `SC_NODE`, `SC_ACTION`, `SC_ASSOCIATION`, `SC_ALTERNATIVE_KEY`, `SC_NODE_ATTRIBUTE`, `SC_NODE_CATEGORY`). Nodes: `CDS_BO_MARKTLOKATION` (root), `CDS_BO_MARKTLOKATION_LOCK`, `CDS_BO_MARKTLOKATION_MESS`, `CDS_BO_MARKTLOKATION_PROP`. Actions: CREATE, DELETE, LOCK, SAVE, UNLOCK, UPDATE, VALIDATE.

### `/HFQ/IF_CDS_INSERT_TEST_C` — BOPF constants interface for INSERT_TEST BO

Test artifact with analogous BOPF structure. Node attributes: `MEINKEY`, `MEINVALUE`.

---

## Function Groups

### `/HFQ/FG_BO` — View maintenance for `/HFQ/DB_BO_ENUM`

Standard SAP table-maintenance function group generated for the enum table. Contains:
- `TABLEFRAME_/HFQ/FG_BO`: Renders the table-control overview screen (Dynpro 0001, `TCTRL_/HFQ/DB_BO_ENUM`) for `/HFQ/DB_BO_ENUM` maintenance.
- `TABLEPROC_/HFQ/FG_BO`: Processes table-control actions (FCODE-driven).

### `/HFQ/CUSTOMIZING` — View maintenance for `/HFQ/CUSTOMIZING`

Standard SAP table-maintenance function group for the key-value customizing table. Contains equivalent `TABLEFRAME_/HFQ/CUSTOMIZING` and `TABLEPROC_/HFQ/CUSTOMIZING` functions plus two screens (0001 overview, 0002 detail).

---

## Reports / Transactions

### `/HFQ/ORDER_REQUEST` (transaction `/HFQ/ORDER_REQUEST`)

Selection-screen report that sends an IS-U ORDERS message (load-profile or master-data request) to the market. Parameters: sender DUNS, receiver DUNS, document type, service type, Zählpunkt (`ext_ui`), date-from/to (ISO UTC strings).

Verified logic:
- Resolves `lv_sender` and `lv_receiver` service IDs from ESERVPROV by DUNS number, falling back to lookup via the Zählpunkt contract chain if direct DUNS lookup fails.
- Converts ISO UTC from/to timestamps to German local time via `BO4E_TOOLS`.
- Creates an `/IDEXGE/CL_ORDERS_REQ` instance and calls `send_orders`.

### `/HFQ/EEDM08_SHORTCUT` (transaction `/HFQ/EEDM08_SHORTCUT`)

Shortcut report to open EEDM08 (SAP profile display transaction) with pre-filled parameters from a `.sap` file launch. Parameters: profile number, date/time from/to, UTC checkbox. If UTC is checked, converts from UTC to German local time via `BO4E_TOOLS` before calling `ISU_S_PROFLIST_DISPLAY`.

---

## CDS Views

All views use `@OData.publish: true` and are backed by an auto-generated OData service each (`.iwmo` + `.iwsv` pairs in the source). Views marked as `#CHECK` authorization enforce standard IS-U authorization objects.

### Business Object Views (BO_*)

#### `/HFQ/CDS_BO_MARKTLOKATION` — SQL view `/HFQ/V_BO_MALO`
**Composition root, read-only.** Joins: `EANL → EUIINSTLN → EUITRANS`. Extends with: enum translations (service type, Sparte, Verbrauchsart, Netzebene); profile role via `CDS_HLP_PROFILE`; MSB via `CDS_HLP_MSB`; previous month's consumption via `CDS_MONTHLY_CONSUMPTION`; address via association `CDS_COM_ADRESSE`; business partner via association `CDS_BO_GPartner`; MeLo assignments via association `CDS_COM_MESSLOKATIONSZUOR`.

Key fields: `marktlokationsId` (= `euitrans.ext_ui`, search HIGH), `internZP`, `sparte`, `energierichtung` (EINSP/AUSSP from `eanl.bezug`), `netzebene` (from `euigrid`, defaulting `'NSP'`), `grundversorgerCodeNr`, `netzgebietNr`, `bilanzierungsgebiet`, `verbrauchsart`, `netzbetreiberCodeNr`, `bilanzierungsmethode` (hardcoded `'DUMMY'`), `isSLP` (X when no logikzw), `profilRolle`, `gasqualitaet` (hardcoded `'HGAS'` for GAS), `MSBCodenr/Ext/Name`, `verbrauch`, `verbrauchsmonat`.

Filter: `uistrutyp = 'MA'`, `dateto = '99991231'` on both EUITRANS and EUIINSTLN.

#### `/HFQ/CDS_BO_MESSLOKATION` — SQL view `/HFQ/BO4E_MELO`
**Read-only (update/delete disabled; create annotation inconsistency noted in source comment).** Joins: `EANL → EUIINSTLN → EUITRANS`. Enum lookups for gas/electricity network level. MSB via MaLo relation (`/UCOM/POD_REL`). Associations: `CDS_HLP_HWARE_ZAEHLER` (meters), `CDS_COM_ADRESSE` (address), `CDS_COM_Geokoord` (geo), `CDS_COM_HARDWARE` (devices).

Key: `messlokationsId` (= `_euitrans.ext_ui`). Fields: `sparte`, `netzebeneMessung`, `bilanzierungsmethode` (hardcoded `'DUMMY'`), `isSLP`, `profilRolle`, `MSBCodenr/Ext/Name`.

Filter: `uistrutyp = 'ME'`, `dateto = '99991231'`.

#### `/HFQ/CDS_BO_EM` — SQL view `/HFQ/V_BO_EM` — Energiemenge (profile-based)
Read-only. Selects from `CDS_COM_HARDWARE`, outer-joins `CDS_BO_MESSLOKATION` and `CDS_BO_MARKTLOKATION` and `CDS_BO_ZAEHLER` and `CDS_COM_ZAEHLWERK`. Key: `anlagennummer`. Fields: `messlokationsId`, `marktlokationsId`, `lokationsId` (whichever is not null), `lokationstyp` (hardcoded `'DUMMY'`), `isMelo`, `energieverbrauch` (association to `CDS_PROFIL`), `zw`, `obiskennzahl`. Excludes records where both MeLo and MaLo ID are null, and where `logikzw` is null.

#### `/HFQ/CDS_BO_EM_ZAEHLERSTA` — SQL view `/HFQ/V_BO_EM_ZS1` — Energiemenge (meter-reading-based)
Read-only. Same structure as `CDS_BO_EM` but uses `CDS_VERBRAUCH_ABLESU` association instead of profile. Key: `anlagennummer`. Fields: `messlokationsId`, `marktlokationsId`, `lokationsId`, `lokationstyp` (hardcoded `'DUMMY'`), `isMelo`, `energieverbrauch` (association to `CDS_VERBRAUCH_ABLESU`), `zaehlernummer`.

#### `/HFQ/CDS_BO_EM_PROFILE` — SQL view `/HFQ/V_EM_PROFIL` — Energiemenge (profile-head-based)
Read-only. Joins: `EPROFHEAD → EPROFASS → EASTS → EUIINSTLN → EUITRANS`, outer-join `ETDZ`. Key: `profil`, `profilRolle`, `lokationsId`. Fields: `lokationstyp` (MeLo/MaLo from `uistrutyp`), `anlagennummer`, `zw`, `obiskennzahl`, `sap_time_zone`, `sap_profdecimals`, association `cdsprofile` to `CDS_PROFIL`. Filter: current-active records only.

#### `/HFQ/CDS_BO_BENACHRICHTIGU` — SQL view `/HFQ/V_BO_BENACH` — Benachrichtigung
**Create and update enabled.** Write persistence: `EMMA_CASE`. Selects `EMMA_CASE` with inner joins to `CDS_HLP_ENUM` for priority (domain `EMMA_CPRIO`) and status (domain `EMMA_CSTATUS`). Associations: `CDS_HLP_AUFGABE` (Aufgaben), `CDS_COM_NOTIZ` (Notizen), `CDS_KEY_VALUE_DUMMY` (Infos).

Key: `benachrichtigungsId = ltrim(emma_case.casenr,'0')`. Fields: `prioritaet`, `bearbeitungsstatus`, `kurztext`, `erstellungsZeitpunkt` (concatenated date+time with placeholder `'--T::zzzz'` — *not a real ISO timestamp; post-processing is commented out*), `kategorie`, `bearbeiter`, associations `aufgaben`, `notizen`, `infos`.

#### `/HFQ/CDS_BO_VERTRAG` — SQL view `/HFQ/V_VERTRAG` — Vertrag
Read-oriented (no create/delete). Joins `EVER → EANL → TESPT`; left-outer-joins `EUIINSTLN → EUITRANS`. Key: `vertragsnummer`. Fields: `beschreibung`, `sparte` (CASE on `tespt.sparte`), `vertragsart` (hardcoded `'ENERGIELIEFERVERTRAG'`), `vertragstatus` (hardcoded `'AKTIV'`), `vertragsbeginn`, `vertragsende`, `lokationsId` (= MaLo `ext_ui` — *source comment notes this is not BO4E-conform*). Filter: `ever.auszdat = '99991231'` (*potential bug noted in source*).

#### `/HFQ/CDS_BO_GPARTNER` — SQL view `/HFQ/V_BO_GPart` — Geschäftspartner
Composition root, read-only. Joins `BUT000` with `FKKVKP → EVER → EANL`; outer-joins `TSAD3T`, `BUT001`, `DFKKBPTAXNUM`, `BUT020/ADR6`, `BUT020/ADR12`, `SEPA_MANDATE`. Associations: `CDS_Kontaktweg`, `CDS_GPRolle`, `CDS_COM_ADRESSE`. Key: `gpnummer + anlage`. Name fields use CASE on `but000.type` (1=person, 2=org, 3=group). Includes `umsatzsteuerId`, `glaeubigerId`, `eMailAdresse`, `website`.

#### `/HFQ/CDS_BO_ZAEHLER` — SQL view `/HFQ/V_BO_ZAEHL` — Zähler
Joins `EASTL → EGERH/EGERR → ETYP → /US4G/DCX_DEVCAT → /US4G/DCX_ATTR`; enum lookups for Sparte, Zaehlerauspraegung (`'Zaehlerauspraegung'`), Zaehlertyp (`'Zaehlertyp'`), Tarifart. Association `CDS_COM_ZAEHLWERK`. Key: `anlage + logiknr`. Uses COALESCE for all enum fields with defaults: `'STROM'`, `'EINRICHTUNGSZAEHLER'`, `'DREHSTROMZAEHLER'`, `'EINTARIF'`. Filter: `eastl.bis = '99991231'`.

#### `/HFQ/CDS_BO_MARKTTEILNEHMER` — SQL view `/HFQ/V_BO_MARPAR`
Thin wrapper over `CDS_MARKTTEILNEHMER` adding association to `CDS_BO_GPartner`. Key: `rollencodenummer`. Data category `#TEXT`.

---

### Component Views (COM_*)

#### `/HFQ/CDS_COM_ADRESSE` — SQL view `/HFQ/V_COM_ADR`
Joins `ADRC → ILOA → IFLOT → EVBS → EANL`; outer-join `BUT020`. Fields: `postleitzahl` (post\_code1 or post\_code2 depending on `po_box`), `ort`, `strasse`, `hausnummer` (house\_num1 + house\_num2), `postfach`, `adresszusatz`, `coErgaenzung`, `landescode`, `vstelle`, `anlage`, `partner`, `is_default_address`. Authorization: S\_ADDRESS1, S\_ADDRESS2, S\_ADDRESS3.

#### `/HFQ/CDS_COM_GEOKOORD` — SQL view `/HFQ/V_COM_GEOK`
Joins `EANL → EVBS → ILOA → ADRC → GEOZ5GOLD` (ZIP-code geo-table). Key: `Anlage`. Fields: `breitengrad`, `laengengrad`. *Geo data sourced from postcode lookup table, not from direct coordinate storage.*

#### `/HFQ/CDS_COM_HARDWARE` — SQL view `/HFQ/BO4E_HWARE`
UNION of two selects from `EUIINSTLN → EUITRANS → EASTL → EGERH` (hardware meters) and `EUIINSTLN → EUITRANS → EASTL → EGERR → CDS_HLP_ENUM` (regular meters), both filtered `euirole_dereg = 'X'`. Key: `internZP`. Fields: `anlagennummer`, `zaehlernummer`, `logNr`, `abDatum`, `bisDatum`, `geraetetyp`, `bezeichnung`.

#### `/HFQ/CDS_COM_ZAEHLWERK` — SQL view `/HFQ/V_C_ZAEHLW`
Read-only. Joins `ETDZ → EPROFASS → CDS_HLP_ENUM` (Energierichtung, Mengeneinheit). Key: `zaehlwerkId + equipmentId`. Fields: `bezeichnung`, `richtung` (default `'AUSSP'`), `obisKennzahl`, `wandlerfaktor`, `einheit` (default `'KWH'`), `rolle`, `logikzw`. Filter: `etdz.bis = '99991231'`.

#### `/HFQ/CDS_COM_NOTIZ` — SQL view `/HFQ/V_COM_NOTIZ`
Selects from `STXL` where `tdobject = 'EMMA_CASE'`. Key: `klaerfallnummer = ltrim(tdname,'0')`. Fields `autor`, `inhalt`, `zeitpunkt` are dummy literals — real values are populated at runtime by `STXL_TO_COM_NOTIZ` in the DPC\_EXT post-processing. Retains `tdid`, `tdname`, `tdobject` for post-processing lookup.

#### `/HFQ/CDS_COM_MESSLOKATIONSZUOR` — SQL view `/HFQ/V_COM_MELOZ`
Minimal: selects `malo_int_ui` (= `pod_rel.int_ui2`) from `/UCOM/POD_REL`. No OData publish. Association-oriented — used by `CDS_BO_MARKTLOKATION` to link MaLo→MeLo.

---

### Helper Views (HLP_*)

#### `/HFQ/CDS_HLP_ENUM` — SQL view `/HFQ/V_BO_ANLAGE`
Simple passthrough of `/HFQ/DB_BO_ENUM`. Authorization: `#NOT_REQUIRED`. Used as the universal enum lookup in virtually every other CDS view. Key: `bo4e_wert`. Fields: `filter`, `sap_wert`, `domname`, `enum_typ`.

#### `/HFQ/CDS_HLP_AUFGABE` — SQL view `/HFQ/V_COM_AUFGA`
Joins `EMMA_CASE → EMMAC_CCAT_SOP → EMMA_CSOLP`. Key: `ccat + casenr + objtype`. Fields: `aufgabenId` (= `solmeths.method`), `ausgefuehrt` (true/false based on `procex_date`), `ausfuehrender`, `ausfuehrungszeitpunkt`.

#### `/HFQ/CDS_HLP_MSB` — SQL view `/HFQ/V_HLP_MSB`
Determines Messstellenbetreiber (MSB). Selects from `TECDE → ESERVICE → ESERVPROV → ESERVPROVT` where `tecde.intcode = 'M1'` and language `'D'`. Key: `int_ui + serviceid + datefrom`. Fields: `dateto`, `externalid`, `sp_name`.

#### `/HFQ/CDS_HLP_PROFILE` — SQL view `/HFQ/V_HLP_PROF`
Joins `EASTS → EPROFASS` with overlap condition. Key: `anlage + profile`. Fields: `logikzw`, `datefrom`, `dateto`, `profrole`.

#### `/HFQ/CDS_HLP_HWARE_ZAEHLER` — SQL view `/HFQ/BO4E_HW_ZL`
Links `CDS_COM_HARDWARE → CDS_BO_ZAEHLER`; associations to `CDS_COM_ZAEHLWERK` and `CDS_BO_MESSLOKATION`. Key: `Anlagennummer`. Fields: `zaehlernummer`, `sparte`, `zaehlerauspraegung`, `zaehlertyp`, `tarifart`, associations `zaehlwerke`, `messlokationen`.

---

### Standalone / Lookup Views

#### `/HFQ/CDS_MARKTTEILNEHMER` — SQL view `/HFQ/V_MARPAR`
Resolves market participants. Joins `ESERVPROVP → ESERVPROV → TECDE → CDS_HLP_ENUM` (Marktrolle, RollenCodeTyp). Outer-joins `EDEXDEFSERVPROV/EDEXCOMMFORMMAIL/EDEXCOMMMAILADDR` for `makoadresse` (MaKo email). Key: `rollencodenummer + rollencodetyp`.

#### `/HFQ/CDS_NETZ` — SQL view `/HFQ/BO4E_Netz`
Joins `EUIGRID → EGRID → EGRIDVL`; association `DB_BO_ENUM`. Key: `internZP`. Fields: `Netz`, `Netzebene`, `Sparte`, `Sparte_Text`, `netzebeneMessung`.

#### `/HFQ/CDS_GPROLLE` — SQL view `/HFQ/V_GPRolle`
Joins `BUT100 → TB003T → CDS_HLP_ENUM` (Geschaeftspartnerrolle). Language filter `'D'`. Key: `geschaeftspartnerrolle` (BO4E string).

#### `/HFQ/CDS_KONTAKTWEG` — SQL view `/HFQ/V_GPkw`
Joins `TSACT → ADRC → CDS_HLP_ENUM → BUT020` (Kontaktart). Language filter `'D'`. Key: `kontaktweg`.

#### `/HFQ/CDS_ANLAGE` — SQL view `/HFQ/BO4E_ANLAGE`
Parameterized (`p_keydate` with `#SYSTEM_DATE`). Joins `EANL → EUIINSTLN`, association to `EANLH`. Key: `Anlage`. Fields: `Anlagenart`, `Sparte`, `Spannungsebene`, `internerZaelpunkt`, `Startdatum`, `Endedatum`, `Tariftyp`, `Ableseeinheit`, `Abrechnungsklasse`, `Branche`.

#### `/HFQ/CDS_PROFIL` — SQL view `/HFQ/V_PROFIL`
UNION of 96 calls to `CDS_PROFIL_SLICE` covering all 15-minute time slots of a day (00:00–23:45) in pairs, using parametrized slice view calls `(p_from:'000000', p_to:'001500')` etc. Key: `zw`. Fields: `startdatum`, `enddatum`, `wert`, `status`, `obiskennzahl`, `wertermittlungsverfahren`, `einheit`, `sap_timezone`, `sap_profdecimals`. *This view has 96 UNION branches — one per 15-minute slot — as a workaround for the absence of SQL window functions in CDS.*

#### `/HFQ/CDS_PROFIL_SLICE` — SQL view `/HFQ/V_PROFSLICE`
Parameterized (`p_from CHAR6`, `p_to CHAR6`). Joins `EPROFHEAD → EPROFASS → ETDZ → CDS_HLP_ENUM → EPROFVAL15 → EPROFVALSTAT`. Uses a 96-branch CASE on `p_from` to pick the correct `eprofval15.val####` column. Fields: `zw`, `sap_timezone`, `sap_profdecimals`, `startdatum`, `enddatum`, `wert`, `status`, `obiskennzahl`, `wertermittlungsverfahren` (hardcoded `'MESSUNG'`), `einheit`.

#### `/HFQ/CDS_VERBRAUCH_ABLESU` — SQL view `/HFQ/V_VERBR_AB`
Calculates energy consumption from consecutive meter readings. Self-joins `EABL` twice (eabl1=start reading, eabl2=end reading) with complex date comparison logic across `adat`/`atim`/`adattats`/`atimtats`/`adatsoll` fields. Outer-joins `ETDZ` and `CDS_HLP_ENUM` (Wertermittlungsverfahren). Computed `wert = eabl2.(v+n)_zwstand - eabl1.(v+n)_zwstand`. `wertermittlungsverfahren` = `'PROGNOSE'` if either reading is PROGNOSE, else `'MESSUNG'`.

#### `/HFQ/CDS_MONTHLY_CONSUMPTION` — SQL view `/HFQ/V_MONTH_CON`
Joins `CDS_MONTH_FOR_CONSUMPTION → ETTIFN`. Returns `anlage`, `keydate`, `value`. Requires `MCONS_OPERAND` in `/HFQ/CUSTOMIZING`.

#### `/HFQ/CDS_MONTH_FOR_CONSUMPTION` — SQL view `/HFQ/V_M_CONS`
Reads operand name from `/HFQ/CUSTOMIZING` (key `'MCONS_OPERAND'`), inner-joins `ETTIFN`, returns `MAX(ab)` per `anlage+operand`. Provides the "most recent billed month" for consumption lookup.

#### `/HFQ/KEY_VALUE_DUMMY_CDS` — SQL view `/HFQ/KV_DUMMY`
Selects from `TDUMMY` (always one row). Provides columns `keyColumn CHAR50`, `value CHAR50`, `boolean_true_column BOOLE_D = 'X'`. Used as a join anchor for adding dynamic info to Benachrichtigung at OData runtime.

---

## Tables / Data Definitions

### `/HFQ/DB_BO_ENUM` — Transparent table, client-dependent
**Description**: BO4E ENUM (kundeneigene) Werte. Mapping table between SAP domain values and BO4E string constants.

| Field | Type | Key | Description |
|---|---|---|---|
| `MANDT` | MANDT | PK | Client |
| `ENUM_TYP` | `/HFQ/DE_BO_ENUM_TYP` | PK | BO4E enum group name (e.g. `'Sparte'`, `'Marktrolle'`) |
| `BO4E_WERT` | `/HFQ/DE_BO_ENUM_WERT` | PK | BO4E string value (e.g. `'STROM'`, `'GAS'`) |
| `DOMNAME` | DOMNAME | PK | SAP domain (e.g. `'SERCODE'`) |
| `SAP_WERT` | `/HFQ/DE_BO_ENUM_SWERT` | PK | SAP domain value |
| `FILTER` | `/HFQ/DE_BO_ENUM_FILTER` | — | Optional filter (e.g. Sparte for Gebietstyp) |
| `BESCHREIBUNG` | `/HFQ/DE_BO_ENUM_DESC` | — | Human-readable description |

Table category: customizing (CCLASS `&NC&`). Maintained via function group `/HFQ/FG_BO`.

### `/HFQ/CUSTOMIZING` — Transparent table, client-dependent
**Description**: Universelller Key-Value-Store für /HFQ/-Lösungen.

| Field | Type | Key |
|---|---|---|
| `MANDT` | MANDT | PK |
| `KEYNAME` | CHAR(100) | PK |
| `VALUE` | CHAR(100) | — |

Table category: customizing (CCLASS `SCUS`). Known keys used by the package:
- `EMMA_CASE_LANGUAGE_STXL`: Language code for reading EMMA case text via `READ_TEXT` (defaults to `'EN'`)
- `MCONS_OPERAND`: Operand name in ETTIFN for monthly consumption

### `/HFQ/INSERT_TEST` — Transparent table (test artifact)

Test table for BOPF insert tests. Referenced by `IF_CDS_INSERT_TEST_C` and related lock/type objects.

### `/HFQ/TYPE_COMPLEX_BOOL`, `/HFQ/S_CDS_BO_MARKTLOKATION`, `/HFQ/S_CDS_BO_MARKTLOKATION_D`, `/HFQ/S_CDS_INSERT_TEST`, `/HFQ/S_CDS_INSERT_TEST_D`, `/HFQ/S_K_CDS_BO_MARKTLOKATION0`, `/HFQ/S_K_CDS_INSERT_TEST_DB_KE`

Structure types generated for BOPF / lock objects. Not documented in detail.

### `/HFQ/T_CDS_BO_MARKTLOKATION`, `/HFQ/T_CDS_INSERT_TEST`, `/HFQ/T_K_CDS_BO_MARKTLOKATION0`, `/HFQ/T_K_CDS_INSERT_TEST_DB_KE`

Table types generated for BOPF. Not documented in detail.

### `/HFQ/V_BO_ZAEHLW` — Database view

*Not fully documented — file is an `.view.xml` metadata file; content not read due to file count.*

---

## Domains and Data Elements

### Domains

| Name | Type | Length | Notes |
|---|---|---|---|
| `/HFQ/DO_BO_KCHAR50` | CHAR | 50 | With lowercase; base domain for enum key fields |
| `/HFQ/DO_BO_KCHAR100` | CHAR | 100 | With lowercase |

### Data Elements (all reference `/HFQ/DO_BO_KCHAR50` unless noted)

| Name | Label | Used in `/HFQ/DB_BO_ENUM` field |
|---|---|---|
| `/HFQ/DE_BO_ENUM_TYP` | BO4E - ENUM-Typ | `ENUM_TYP` |
| `/HFQ/DE_BO_ENUM_WERT` | (BO4E value) | `BO4E_WERT` |
| `/HFQ/DE_BO_ENUM_SWERT` | (SAP value) | `SAP_WERT` |
| `/HFQ/DE_BO_ENUM_FILTER` | (filter) | `FILTER` |
| `/HFQ/DE_BO_ENUM_DESC` | (description) | `BESCHREIBUNG` |

---

## Access Control Definitions (DCLS)

The package includes CDS access control roles (`.asdcls` files) for all major views. All use `@MappingRole: true`.

| Role | View protected | Authorization objects |
|---|---|---|
| `/HFQ/AC__ADRESSE` | `CDS_COM_ADRESSE` | `S_ADDRESS1` (ADGRP=`*`, ACTVT=03), `S_ADDRESS2`, `S_ADDRESS3` |
| `/HFQ/AC__MARKTTEILNEHMER` | `CDS_BO_MARKTTEILNEHMER` | *Not read — inferred from naming pattern* |
| `/HFQ/AC_ANLAGE` | `CDS_ANLAGE` | *Not read* |
| `/HFQ/AC_BENACHRICHTIGUNG` | `CDS_BO_BENACHRICHTIGU` | *Not read* |
| `/HFQ/AC_EM` | `CDS_BO_EM` | *Not read* |
| `/HFQ/AC_EM_PROFILE` | `CDS_BO_EM_PROFILE` | *Not read* |
| `/HFQ/AC_EM_ZAEHLERSTA` | `CDS_BO_EM_ZAEHLERSTA` | *Not read* |
| `/HFQ/AC_GEOKOORD` | `CDS_COM_GEOKOORD` | *Not read* |
| `/HFQ/AC_GPARTNER` | `CDS_BO_GPARTNER` | *Not read* |
| `/HFQ/AC_GPROLLE` | `CDS_GPROLLE` | *Not read* |
| `/HFQ/AC_HARDWARE` | `CDS_COM_HARDWARE` | *Not read* |
| `/HFQ/AC_HLP_AUFGABE` | `CDS_HLP_AUFGABE` | *Not read* |
| `/HFQ/AC_HLP_HWARE_ZAEHLER` | `CDS_HLP_HWARE_ZAEHLER` | *Not read* |
| `/HFQ/AC_KONTAKTWEG` | `CDS_KONTAKTWEG` | *Not read* |
| `/HFQ/AC_MARKTLOKATION` | `CDS_BO_MARKTLOKATION` | `E_POD` (ISU_ACTIVT=`01` or `1`) AND `S_TCODE` (TCD=`EEDM11`) |
| `/HFQ/AC_MARKTTEILNEHMER` | `CDS_MARKTTEILNEHMER` | *Not read* |
| `/HFQ/AC_MELOZUORDNUNG` | `CDS_COM_MESSLOKATIONSZUOR` | *Not read* |
| `/HFQ/AC_MESSLOKATION` | `CDS_BO_MESSLOKATION` | *Not read* |
| `/HFQ/AC_NETZ` | `CDS_NETZ` | *Not read* |
| `/HFQ/AC_NOTIZ` | `CDS_COM_NOTIZ` | *Not read* |
| `/HFQ/AC_PROFIL` | `CDS_PROFIL` | *Not read* |
| `/HFQ/AC_PROFIL_SLICE` | `CDS_PROFIL_SLICE` | *Not read* |
| `/HFQ/AC_VERBRAUCH_ABLESU` | `CDS_VERBRAUCH_ABLESU` | *Not read* |
| `/HFQ/AC_VERTRAG` | `CDS_BO_VERTRAG` | *Not read* |
| `/HFQ/AC_ZAEHLER` | `CDS_BO_ZAEHLER` | *Not read* |
| `/HFQ/AC_ZAEHLWERK` | `CDS_COM_ZAEHLWERK` | *Not read* |

*Entries marked "Not read" are inferred from filename and naming convention. Individual authorization objects not verified against implementation.*

---

## Other Objects

### OData Services and Models

Each CDS view with `@OData.publish: true` has an auto-generated OData model (`.iwmo`) and service (`.iwsv`). The primary composed OData service for the BO4E suite is:

| Object | Name | Description |
|---|---|---|
| OData provider | `/HFQ/BO4E` (`.iwpr.xml`) | Service provider registration |
| OData model | `/HFQ/BO4E_MDL` | Base model (generated) |
| OData model | `/HFQ/BO4E_MDL_01` | Extended model |
| OData service | `/HFQ/BO4E_SRV` | Base service |
| OData service | `/HFQ/BO4E_SRV_01` | Extended service (active, registered) |
| Backend extension | `/HFQ/BO4E_MDL_01_0001_BE` | `.iwom.xml` — backend extension definition |
| Service group | `/HFQ/BO4E_SRV_01_0001` | `.iwsg.xml` — service group |
| Vocabulary binding | `/HFQ/BO4E_ANNO_MDL` / `_01` | `.iwvb.xml` — annotation vocabulary bindings |

Individual CDS-based OData services (auto-published, one per view group):
`CDS_BO_MARKTLOKATION_CDS`, `CDS_BO_MESSLOKATION_CDS`, `CDS_BO_EM_CDS`, `CDS_BO_EM_ZAEHLERSTA_CDS`, `CDS_BO_EM_PROFILE_CDS`, `CDS_BO_BENACHRICHTIGU_CDS`, `CDS_BO_GPARTNER_CDS`, `CDS_BO_VERTRAG_CDS`, `CDS_BO_ZAEHLER_CDS`, `CDS_BO_MALO_202106_CDS`, `CDS_COM_ADRESSE_CDS`, `CDS_COM_GEOKOORD_CDS`, `CDS_COM_HARDWARE_CDS`, `CDS_COM_NOTIZ_CDS`, `CDS_COM_ZAEHLWERK_CDS`, `CDS_GPROLLE_CDS`, `CDS_HLP_AUFGABE_CDS`, `CDS_HLP_ENUM_CDS`, `CDS_HLP_HWARE_ZAEHLER_CDS`, `CDS_HLP_MSB_CDS`, `CDS_HLP_PROFILE_CDS`, `CDS_KONTAKTWEG_CDS`, `CDS_MARKTTEILNEHMER_CDS`, `CDS_NETZ_CDS`, `CDS_PROFIL_CDS`, `CDS_PROFIL_SLICE_CDS`, `CDS_VERBRAUCH_ABLESU_CDS`, `KEY_VALUE_DUMMY_CDS_CDS`.

### Enhancement Spots and Implementations

| Object | Type | Purpose |
|---|---|---|
| `/HFQ/BO4E_POST_PROCESSING` | `.enhs.xml` Enhancement Spot | BAdI spot for post-processing |
| `/HFQ/BO4E_POST_PROCESSING` | `.enho.xml` Enhancement Implementation | Default implementation binding |
| `/HFQ/BO4E_SEARCH` | `.enhs.xml` | BAdI spot for search |
| `/HFQ/EMMA_CASE_DETAILS` | `.enhs.xml` | BAdI spot for EMMA case detail resolution |

### ICF Nodes

| File | Service path |
|---|---|
| `bo4e ... .sicf.xml` | BO4E REST service node |
| `bo4e_srv_01 ... .sicf.xml` | BO4E OData service node |
| `hfq ... .sicf.xml` | HFQ namespace ICF node |

### Authorization Objects (SUSH)

16 authorization object check (`.sush.xml`) files are present, covering IS-U and BO4E authorization checks. Individual object names are GUIDs — not documented further.

### Namespace

`/HFQ/` namespace registered via `src/#hfq#.nspc.xml`.

### Object type registrations (TOBJ)

- `/HFQ/CUSTOMIZINGS` — customizing object for `/HFQ/CUSTOMIZING` table
- `/HFQ/DB_BO_ENUMS` — customizing object for `/HFQ/DB_BO_ENUM` table

### AVAS (attribute value assignment)

`00505694359e1ee8ba9ed4dfcfbc7809.avas.xml` — one AVAS object, likely a classification attribute for the BOPF Marktlokation BO. Not documented in detail.

---

*Not fully documented due to package size (371 files): all `.xml`-only metadata files for BOPF lock and test objects (`CL_A_LOCK_*`), the full implementation bodies of `/HFQ/CL_BO4E_01_DPC` and `/HFQ/CL_BO4E_01_DPC_EXT` (protected entity-set handlers), the 16 `.sush.xml` authorization check files, and all `.iwvb` vocabulary binding contents.*
