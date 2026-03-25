# Package: HFQ / KOMDATA_MODEL

**Description**: Datenmodell Kommunikationsdaten (Communication Data Data Model)
**Original language**: German (D)
**Number of objects**: 172 files across ~100 distinct repository objects (transparent tables, structures, type pools, classes, function groups, domains, data elements, number range objects, lock object, search help, message class, enhancement implementation)

---

## Executive Summary

This package implements the data model for PARTIN (EDI party information) contact data sheets (German: *Kontaktdatenblätter*) used in the German energy market. It stores versioned, time-sliced contact master data for energy-market service providers, covering four data domains: contacts/persons (Ansprechpartner), bank accounts (Bankdaten), balance-group memberships (Bilanzkreise), and availability windows (Erreichbarkeitszeiten). The central business object is `CL_PARTIN`, which encapsulates read/write lifecycle operations (constructor load, draft creation, version generation, data mutation via SET methods, save, mark-as-sent) on top of a normalized relational database keyed by `(SERVICEID, SERVICEID_RECEIVER, VERSION)`. Customizing tables (`MAND_COM`, `MAND_BNK`, `MAND_BIKR`, `MAND_HDR`, `MAND_SPART`, `PTCUST`) govern which data fields and categories are mandatory for each sender/receiver service-type combination, and an IDoc exit function group (`FG_IDOC_EXIT`) bridges to the IDXGC/IDXGL IDoc mapping framework for inbound and outbound PARTIN EDI messages.

---

## Classes

### `/HFQ/CL_PARTIN`

**Description**: Data-provision class for PARTIN contact data sheets (*Datenbereitstellungsklasse für PARTIN-Kontaktdatenblätter*). Created December 2021 by Hochfrequenz Unternehmensberatung GmbH.

**Visibility**: `PUBLIC CREATE PUBLIC`

**Constant**:
- `GC_BIKREIS_CODE_Z48 TYPE CHAR3 VALUE 'Z48'` — balance-group type Z48
- `GC_DRAFT_VERSION TYPE /HFQ/DE_KD_VERSION VALUE '0'` (private) — draft version sentinel

**Public class method**:

```abap
CLASS-METHODS GET_INSTANCE_FROM_DATA
  IMPORTING
    IS_HEADER        TYPE /HFQ/S_KD_HDR
    IT_AVAILABILITY  TYPE /HFQ/T_KD_AVAIL
    IT_BILANZKREIS   TYPE /HFQ/T_KD_BIKREIS
    IT_BANK_DATA     TYPE /HFQ/T_KD_BANK
    IT_CONTACTS      TYPE /HFQ/T_KD_CONTACT
    IV_STATUS        TYPE /HFQ/DE_KD_VERSION_STATUS DEFAULT 'RECEIVED'
  RETURNING VALUE(RR_PARTIN) TYPE REF TO /HFQ/CL_PARTIN
  RAISING /HFQ/CX_PARTIN_ERROR
```
Creates an in-memory `CL_PARTIN` instance pre-populated with all four data domains from caller-supplied tables (no database read). *Verified: factory pattern, sets protected attributes directly.*

**Public instance methods** (all verified from implementation):

| Method | Signature summary | Behavior |
|---|---|---|
| `CONSTRUCTOR` | `IV_SERVICEID TYPE SERVICE_PROV`, `IV_DATE TYPE DATS optional`, `IV_VERSION TYPE /HFQ/DE_KD_VERSION optional`, `IV_SERVICEID_RECEIVER TYPE /HFQ/DE_KD_SERVICEID_EMPFANG optional` — raises `/HFQ/CX_PARTIN_ERROR` | Loads the most-applicable version from DB using `determine_version` in `CL_PARTIN_HELPER`; priority: explicit version > date lookup > latest active |
| `CHECK_DATA` | `IV_STRICT TYPE BOOLEAN DEFAULT ''` → `RT_ERROR TYPE /HFQ/T_KD_ERROR` | Validates consistency of all in-memory data; strict mode also checks for superfluous fields; returns error table, no exception |
| `ENTER_DRAFT` | `IV_FORCE_NEW_DRAFT TYPE ABAP_BOOL DEFAULT ''` — raises `/HFQ/CX_PARTIN_ERROR` | Switches the object to draft mode (version `'0'` / `GC_DRAFT_VERSION`) |
| `GENERATE_VERSION` | (protected) — raises `/HFQ/CX_PARTIN_ERROR` | Generates a new numeric version number via number range `/HFQ/KDVER` |
| `GET_AVAIL` | → `RT_AVAILABILITY TYPE /HFQ/T_KD_AVAIL` | Returns current-version availability windows (filtered from `GT_AVAIL`) |
| `GET_AVAIL_ALL` | → `RT_AVAILABILITY TYPE /HFQ/T_KD_AVAIL_ALL` | Returns all versions of availability windows |
| `GET_BANK` | → `RT_BANK_DATA TYPE /HFQ/T_KD_BANK` | Returns current-version bank data |
| `GET_BANK_ALL` | → `RT_BANK_DATA TYPE /HFQ/T_KD_BANK_ALL` | Returns all versions of bank data |
| `GET_BIKREIS` | → `RT_BILANZKREIS TYPE /HFQ/T_KD_BIKREIS` | Returns current-version balance-group data |
| `GET_BIKREIS_ALL` | → `RT_BILANZKREIS TYPE /HFQ/T_KD_BIKREIS_ALL` | Returns all versions of balance-group data |
| `GET_CONT` | → `RT_CONTACTS TYPE /HFQ/T_KD_CONTACT` | Returns current-version contact persons |
| `GET_CONT_ALL` | → `RT_CONTACTS TYPE /HFQ/T_KD_CONTACT_ALL` | Returns all versions of contact persons |
| `GET_DRAFT_EXISTS` | → `RV_EXISTS TYPE ABAP_BOOL` | Returns true if a draft version (version = `'0'`) exists for this service provider |
| `GET_HEADER` | → `RS_HEADER TYPE /HFQ/S_KD_HDR` | Returns current header data (version metadata, validity, status) |
| `LOAD_VERSION` | `IV_DATE TYPE DATS optional`, `IV_VERSION TYPE /HFQ/DE_KD_VERSION optional` — raises `/HFQ/CX_PARTIN_ERROR` | Loads a different version of the same service provider into this object |
| `MARK_AS_INVALID` | (no params, no exception) | Sets version status to `INVALID` in memory |
| `MARK_AS_SENT` | raises `/HFQ/CX_PARTIN_ERROR` | Persists `SENT` status to the database (`/HFQ/PTCOMV`) |
| `PREPARE_FOR_SEND` | `IV_DATE_FROM TYPE /HFQ/DE_KD_VON`, `IV_PREV_VERSION TYPE /HFQ/DE_KD_PREV_VERSION optional` — raises `/HFQ/CX_PARTIN_ERROR` | Sets validity start date and previous-version reference; transitions status to `READY` |
| `SAVE_DATA` | `IV_SAVE_AS_DRAFT TYPE ABAP_BOOL DEFAULT 'X'` — raises `/HFQ/CX_PARTIN_ERROR` | Persists all four data domains plus header to DB; delegates to `SAVE_HEADER`, `SAVE_CONT`, `SAVE_AVAIL`, `SAVE_BANK`, `SAVE_BIKREIS` |
| `SET_AVAIL` | `IS_AVAILABILITY TYPE /HFQ/S_KD_AVAIL_ALL` — raises `/HFQ/CX_PARTIN_ERROR` | Replaces in-memory availability data; calls `ON_CHANGE` |
| `SET_BANK` | `IS_BANK_DATA TYPE /HFQ/S_KD_BANK_ALL` — raises `/HFQ/CX_PARTIN_ERROR` | Replaces in-memory bank data; calls `ON_CHANGE` |
| `SET_BIKREIS` | `IS_BILANZKREIS TYPE /HFQ/S_KD_BIKREIS_ALL` — raises `/HFQ/CX_PARTIN_ERROR` | Replaces in-memory balance-group data; calls `ON_CHANGE` |
| `SET_CONT` | `IS_CONTACTS TYPE /HFQ/S_KD_CONTACT_ALL` — raises `/HFQ/CX_PARTIN_ERROR` | Replaces in-memory contact person data; calls `ON_CHANGE` |
| `SET_HEADER` | `IS_HEADER TYPE /HFQ/S_KD_HDR` — raises `/HFQ/CX_PARTIN_ERROR` | Replaces in-memory header data; calls `ON_CHANGE` |

**Protected attributes** (verified):
- `GT_CUST_AVAIL TYPE /HFQ/T_KD_CUST_AVAIL` — availability customizing rules
- `GT_CUST_BIKREIS TYPE /HFQ/T_KD_CUST_BIKREIS` — balance-group customizing rules
- `GT_CUST_BANK TYPE /HFQ/T_KD_CUST_BANK` — bank data customizing rules
- `GT_CUST_CONT TYPE /HFQ/T_KD_CUST_CONT` — contact customizing rules
- `GS_HEADER TYPE /HFQ/S_KD_HDR` — header (version/validity/status)
- `GT_CONT TYPE /HFQ/T_KD_CONTACT_ALL` — all-version contact data
- `GT_AVAIL TYPE /HFQ/T_KD_AVAIL_ALL` — all-version availability data
- `GT_BIKREIS TYPE /HFQ/T_KD_BIKREIS_ALL` — all-version balance-group data
- `GT_BANK TYPE /HFQ/T_KD_BANK_ALL` — all-version bank data

---

### `/HFQ/CL_PARTIN_DB`

**Description**: Database access layer for PARTIN data. `FINAL`, all methods are class-methods (static). Verified from implementation.

**Constant**: `GC_C_MANUAL_BANK_EDIT TYPE /HFQ/DE_KD_CUSTPARAM VALUE 'MANUAL_BANK_EDIT'`

**Public class methods**:

| Method | Signature summary | Behavior |
|---|---|---|
| `CLEAN_PTCONTI_PTCONTP` | raises `/HFQ/CX_PARTIN_ERROR` | Deletes orphaned rows from `/HFQ/PTCONTP` and `/HFQ/PTCONTI` where the `contact_id` / `info_contact_id` no longer exists in `/HFQ/PTCOMP` |
| `GET_CUST_MAND_AVAIL` | `IV_SENDER SERVICE_PROV optional`, `IV_RECEIVER SERVICE_PROV optional` → `RT_AVAIL_CODES TYPE /HFQ/T_KD_CUST_AVAIL` | Reads `/HFQ/PTTIMETYPE`; sets min/max occurrences: Z41 = 0..1, all others = 1..1 |
| `GET_CUST_MAND_BANK` | `IV_SENDER SERVICE_PROV optional`, `IV_RECEIVER SERVICE_PROV optional` → `RT_BANK_CODES TYPE /HFQ/T_KD_CUST_BANK` | Reads `/HFQ/MAND_BNK` filtered by sender intcode; raises error if no customizing found |
| `GET_CUST_MAND_BIKREIS` | `IV_SENDER SERVICE_PROV optional`, `IV_RECEIVER SERVICE_PROV optional` → `RT_BIKREIS_CODES TYPE /HFQ/T_KD_CUST_BIKREIS` | Reads `/HFQ/MAND_BIKR` directly |
| `GET_CUST_MAND_CONT` | `IV_SENDER SERVICE_PROV optional`, `IV_RECEIVER SERVICE_PROV optional` → `RT_CONTACT_CODES TYPE /HFQ/T_KD_CUST_CONT` | Reads `/HFQ/MAND_COM` filtered by sender/receiver intcode combination; min/max hardcoded to 1..1 per contact type |
| `SELECT_PTCOMP` | `IV_SERVICEID`, `IV_VERSION`, `IV_SERVICEID_RECEIVER` → `RT_PTCOMP TYPE /HFQ/T_KD_PTCOMP` | SELECT * FROM `/HFQ/PTCOMP` with exact key |
| `SELECT_PTCOMTIME` | same key → `RT_PTCOMTIME TYPE /HFQ/T_KD_PTCOMTIME` | SELECT * FROM `/HFQ/PTCOMTIME` with exact key |
| `SELECT_PTBIKREIS` | same key → `RT_PTBIKREIS TYPE /HFQ/T_KD_PTBIKREIS` | SELECT * FROM `/HFQ/PTBIKREIS` with exact key |
| `SELECT_PTCOMV` | same key → `RS_PTCOMV TYPE /HFQ/PTCOMV` | SELECT SINGLE FROM `/HFQ/PTCOMV` with exact key |
| `SELECT_PTCONTBK` | same key → `RT_PTCONTBK TYPE /HFQ/T_KD_PTCONTBK` | SELECT * FROM `/HFQ/PTCONTBK` with exact key |
| `SELECT_PTCONTI` | `IT_INFO_CONTACT_ID TYPE /HFQ/T_KD_INFO_CONTACT_ID` → `RT_PTCONTI TYPE /HFQ/T_KD_PTCONTI` | SELECT with range on `info_contact_id`; deduplicates input IDs |
| `SELECT_PTCONTP` | `IT_CONTACT_ID TYPE /HFQ/T_KD_CONTACT_ID`, `IV_SERVICEID TYPE SERVICE_PROV` → `RT_PTCONTP TYPE /HFQ/T_KD_PTCONTP` | SELECT with range on `contact_id` |
| `UPDATE_CONTACTS` | `CHANGING CT_PTCOMP`, `CT_PTCONTP`, `CT_PTCONTI` — raises `/HFQ/CX_PARTIN_ERROR` | For each PTCOMP entry: checks if linked PTCONTP/PTCONTI data has changed; if changed, generates new ID via number range and updates timestamps; then MODIFYs all three tables atomically |
| `UPDATE_PTCOMP` | `IT_PTCOMP` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCOMP` |
| `UPDATE_PTCOMTIME` | `CHANGING CT_PTCOMTIME` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCOMTIME` |
| `UPDATE_PTBIKREIS` | `CHANGING CT_PTBIKREIS` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTBIKREIS` |
| `UPDATE_PTCOMV` | `CHANGING CT_PTCOMV` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCOMV` |
| `UPDATE_PTCONTBK` | `CHANGING CT_PTCONTBK` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCONTBK` |
| `UPDATE_PTCONTI` | `IT_PTCONTI` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCONTI` |
| `UPDATE_PTCONTP` | `IT_PTCONTP` — raises `/HFQ/CX_PARTIN_ERROR` | MODIFY `/HFQ/PTCONTP` |
| `DELIMIT_TIMESLICES` | `IV_SERVICEID TYPE SERVICE_PROV`, `IV_DATE TYPE DATS`, `IV_SID_EMPFANG optional` | Sets `BIS` (end date) of the current active version in `/HFQ/PTCOMV` to `IV_DATE`, effectively ending that time slice |
| `QUERY_GENERAL_CUST` | `IV_PARAM TYPE /HFQ/DE_KD_CUSTPARAM` → `RV_VALUE TYPE /HFQ/DE_KD_CUSTVALUE` | Reads `/HFQ/PTCUST` scoped to the own logical system (via `OWN_LOGICAL_SYSTEM_GET`) |

---

### `/HFQ/CL_PARTIN_HELPER`

**Description**: Utility/helper class for PARTIN processing. `FINAL`, all methods are class-methods.

**NAD qualifier constants** (verified, all `TYPE /IDXGC/DE_PARTY_FUNC_QUAL` or `CHAR3`):
`Z10`, `Z11`, `Z12`, `Z13`, `Z14`, `Z16`, `Z17`, `Z18`, `Z19`, `Z20`, `Z21`, `Z22` (NAD role qualifiers), `SU` (supplier), `DDM` (Netzbetreiber), `DEB` (Messstellenbetreiber/MSB); FTX qualifiers `Z11`, `Z12`, `Z13`; RFF qualifiers `Z25`, `VA`, `FC`.

**Other constant**: `GC_NUM_BANK_DATA TYPE I VALUE 10`

**Public class methods** (verified):

| Method | Signature summary | Behavior |
|---|---|---|
| `CHECK_IF_OWN_SERVICE` | `IV_SERVICE_ID TYPE SERVICE_PROV` → `RV_OWN_SERVICE TYPE BOOLEAN` | SELECT SINGLE from `ESERVPROV` for `OWN_LOG_SYS` flag |
| `CHECK_IF_SPARTE_APPLICABLE` | `IV_SERVICEID TYPE SERVICE_PROV` → `RV_APPLICABLE TYPE BOOLEAN` | Reads `/HFQ/MAND_SPART` and looks up the division of the service provider via `ESERVPROV JOIN TECDE`; returns true if the division is in the allowed list |
| `COMPARE_CONTACTS` | `IS_CONTACT_DB TYPE /HFQ/S_KD_CONTACT`, `IS_CONTACT_ID TYPE /IDXGC/S_NAMEADDR_DETAILS`, `CHANGING CT_CHECK_RESULT TYPE /IDXGC/T_CHECK_RESULT` | Field-by-field comparison of DB contact record vs. IDXGC name-address structure; appends `EQUAL` or `NOT_EQUAL` to result table |
| `DELETE_ALL_DATA` | (no params) | Shows a confirmation popup (POPUP_TO_CONFIRM) and if confirmed deletes all rows from all six PARTIN runtime tables; intended for debug use only |
| `DELETE_VERSION` | `IV_SERVICEID TYPE SERVICE_PROV`, `IV_VERSION TYPE /HFQ/DE_KD_VERSION` — raises `/HFQ/CX_PARTIN_ERROR` | Deletes the specified version from `/HFQ/PTCOMV`, `/HFQ/PTCONTBK`, `/HFQ/PTCOMP`, `/HFQ/PTCOMTIME`; then calls `CLEAN_PTCONTI_PTCONTP` |
| `DETERMINE_VERSION` | `IV_DATE TYPE DATS optional`, `IV_SERVICEID TYPE SERVICE_PROV`, `IV_STATUS TYPE /HFQ/DE_KD_VERSION_STATUS optional`, `IV_SID_EMPFANG optional` → `RS_PTCOMV TYPE /HFQ/PTCOMV` | Reads `/HFQ/PTCOMV` for the service provider; filters by date range and status; returns the entry with the most recent `VON` start date |
| `GENERATE_CONTACT_ID` | → `RV_CONTACT_ID TYPE /HFQ/DE_KD_CONTACT_ID` — raises `/HFQ/CX_PARTIN_ERROR` | Calls `ISU_NUMBER_GET` on number range `/HFQ/KDCID` |
| `GENERATE_INFO_CONTACT_ID` | → `RV_INFO_CONTACT_ID TYPE /HFQ/DE_KD_INFO_CONTACT_ID` — raises `/HFQ/CX_PARTIN_ERROR` | Calls `ISU_NUMBER_GET` on number range `/HFQ/KDICI` |
| `GENERATE_VERSION` | `IV_SERVICEID TYPE SERVICE_PROV` → `RV_VERSION TYPE /HFQ/DE_KD_VERSION` — raises `/HFQ/CX_PARTIN_ERROR` | Calls `ISU_NUMBER_GET` on number range `/HFQ/KDVER` |
| `GET_BANK_FOR_SP` | `IV_SERVICEID TYPE SERVICE_PROV`, `IV_BKVID TYPE BU_BKVID` → `RS_BANK_DATA TYPE /HFQ/S_KD_BANK_DATA` — raises `/HFQ/CX_PARTIN_ERROR` | Retrieves bank account details from Business Partner bank data (BP/BU tables) |
| `GET_BUT_BANK_ACCS_FOR_SP` | `IV_SERVICEID TYPE SERVICE_PROV` → exports `ET_BNKA TYPE BF_BNKA`, returns `RT_BUT0BK TYPE BUT0BK_T` — raises `/HFQ/CX_PARTIN_ERROR` | Retrieves all bank accounts for a service provider from BP data |
| `GET_CONTACT_FOR_SP` | `IV_SERVICE_ID TYPE SERVICE_PROV` → `RS_PTCONTP TYPE /HFQ/S_KD_CONTACT_ADDR` — raises `/HFQ/CX_PARTIN_ERROR` | Retrieves primary contact address from BP master data |
| `GET_CONTACT_ID` | `IV_GENERATE_NEW_ID TYPE ABAP_BOOL`, `IS_CONTACT_ADDR TYPE /HFQ/S_KD_CONTACT_ADDR` → exports `EV_CONTACT_ID`, `EV_GENERATED_NEW_ID TYPE ABAP_BOOL` — raises `/HFQ/CX_PARTIN_ERROR` | Looks up existing `CONTACT_ID` for a given address in `/HFQ/PTCONTP`; optionally generates a new one |
| `GET_INFO_CONTACT_ID` | `IV_GENERATE_NEW_ID TYPE ABAP_BOOL`, `IS_CONTACT_COMM TYPE /HFQ/S_KD_CONTACT_COMM` → exports `EV_INFO_CONTACT_ID`, `EV_GENERATED_NEW_ID TYPE ABAP_BOOL` — raises `/HFQ/CX_PARTIN_ERROR` | Looks up or generates an `INFO_CONTACT_ID` for communication data |
| `GET_INTCODE` | `IV_SERVICEID TYPE SERVICE_PROV` → `RV_INTCODE TYPE INTCODE` — raises `/HFQ/CX_PARTIN_ERROR` | Looks up the internal code (Strom/Gas market code) for a service provider from `/HFQ/INTCODES` |
| `GET_PARTY_FUNC_QUAL` | `IV_SERVICEID TYPE SERVICE_PROV` → `RV_PARTY_FUNC_QUAL TYPE /IDXGC/DE_PARTY_FUNC_QUAL` — raises `/HFQ/CX_PARTIN_ERROR` | Derives the NAD qualifier (SU/DDM/DEB) from the service provider's service type |
| `GET_SERCODE` | `IV_SERVPROV TYPE SERVICE_PROV` → `RV_SERCODE TYPE SERCODE` — raises `/HFQ/CX_PARTIN_ERROR` | Retrieves the service code for a service provider |
| `POPUP_OWN_SERVPROV` | `IV_SERVICEID_VIEW TYPE SERVICE_PROV` → `RV_SERVICEID_RECEIVER TYPE SERVICE_PROV` | Shows a selection popup for own service providers |
| `TODO` | (no params) | *Inferred: placeholder/stub method. No implementation body visible in class definition.* |

---

### `/HFQ/CL_KOMDATA_IM_PROC_DOC_DB`

**Description**: BAdI implementation for `/IDXGC/BADI_PROCESS_DOC_DB` in enhancement spot `/IDXGC/ES_PROCESS`. Registered under enhancement implementation `/HFQ/IM_KOMDATA_PROCESS` with filter `IV_PROC_CLUSTER = '/HFQ/KOMDATA'`. Inherits from `/IDXGC/CL_IM_BADI_PROC_DOC_DB`. `FINAL`.

**Constants** (verified):
- `GC_TABLE_PROC_STEP_AVAIL TYPE TABNAME VALUE '/HFQ/PRST_AVAIL'`
- `GC_TABLE_PROC_STEP_BANK TYPE TABNAME VALUE '/HFQ/PRST_BANK'`
- `GC_TABLE_PROC_STEP_BIKREIS TYPE TABNAME VALUE '/HFQ/PRST_BIKR'`

**Redefined methods** (verified from implementation):

| Method | Behavior |
|---|---|
| `/IDXGC/IF_BADI_PROCESS_DOC_DB~GET_TABLE_CONTROL_DETAILS` | Calls super, then adds `/HFQ/PRST_AVAIL`, `/HFQ/PRST_BIKR`, and `/HFQ/PRST_BANK` to the table-control list |
| `/IDXGC/IF_BADI_PROCESS_DOC_DB~SELECT_MSG_MASS` | Calls super; then selects from all three process-step extension tables (`PRST_AVAIL`, `PRST_BIKR`, `PRST_BANK`) and attaches availability, balance-group, and bank step data to the corresponding `CT_MESSAGE_DATA` entries |
| `/IDXGC/IF_BADI_PROCESS_DOC_DB~UPDATE_PDOC` | Calls super; then for each message data entry: compares new vs. old avail/bikreis/bank step data; DELETEs old rows and MODIFYs new rows in the three process-step tables; updates table-control read flags accordingly |

---

### `/HFQ/CX_PARTIN_ERROR`

**Description**: Exception class for PARTIN/KOMDATA processing errors. Inherits from `/US4G/CX_GENERAL` (which itself implements `IF_T100_MESSAGE`). `FINAL`.

**Constructor**: Standard T100-based constructor accepting `TEXTID`, `PREVIOUS`, `SOURCE_OBJ_MAIN`, `SOURCE_OBJ_SUB`, `SOURCE_LINE`.

**Class method**:
```abap
CLASS-METHODS RAISE_PARTIN_EXCEPTION_MSG
  IMPORTING
    IR_PREVIOUS         TYPE REF TO CX_ROOT optional
    IV_MSGID            TYPE SYMSGID   DEFAULT SY-MSGID
    IV_MSGNO            TYPE SYMSGNO   DEFAULT SY-MSGNO
    IV_MSGV1..IV_MSGV4  TYPE SYMSGV    DEFAULT SY-MSGVx
    IS_TEXTID           TYPE SCX_T100KEY optional
    IV_EXCEPTION_CODE   TYPE CHAR20    DEFAULT 'TechError'
    IR_DATA             TYPE REF TO DATA optional
    IS_PROCESS_STEP_KEY TYPE /APE/S_PROC_STEP_KEY optional
  RAISING /HFQ/CX_PARTIN_ERROR
```
The implementation body is entirely commented out; the method currently raises no exception. *Verified: method body is a no-op stub; actual exception raising happens in callers via `RAISE EXCEPTION TYPE /HFQ/CX_PARTIN_ERROR` or via message-based patterns using the static call path.*

---

### `/HFQ/IF_PARTIN_CONSTANTS`

**Description**: Constants class (acts as an interface-style constants container). No instance data, no methods beyond implicit constructor. All members are public constants.

**Key constants** (all verified from implementation):

*Version status values* (used in `/HFQ/DO_KD_VERSION_STATUS`):
- `GC_VSTATUS_BACKLOG = 'BACKLOG'`
- `GC_VSTATUS_INVALID = 'INVALID'`
- `GC_VSTATUS_NO_DATA = 'NO_DATA'`
- `GC_VSTATUS_READY = 'READY'`
- `GC_VSTATUS_RECEIVED = 'RECEIVED'`
- `GC_VSTATUS_SENT = 'SENT'`
- `GC_VSTATUS_UNSAVED = 'UNSAVED'`

*Special version numbers*:
- `GC_DRAFT_VERSION = '999999999'` — in-memory draft sentinel
- `GC_DEFAULT_CONTACT_ID = '999999999'`

*Contact types* (`GC_CONTACT_Z10` .. `GC_CONTACT_Z22`, `GC_CONTACT_Z33`): German energy-market contact-type codes with display names, e.g.:
- Z10 = Übertragungsweg/Datenaustausch
- Z11 = Rahmenverträge
- Z12 = Kündigungsprozesse
- Z13 = Wechselprozesse
- Z14 = Stammdatenprozesse
- Z16 = Einspeiseprozesse
- Z17 = Abrechnungsprozesse
- Z18 = MMMA-Prozesse
- Z19 = Bewegungsdaten
- Z20 = Sperr-/Entsperrprozesse
- Z21 = Bilanzierungsprozesse/BK-Management
- Z33 = tech. Netzanschluss für Neuanlagen und Anlagenumbau

*Process IDs*: `GC_PROCID_KOMDAT_MAS = '/HFQ/KOMDAT_MAS'`, `GC_PROCID_KD_API_SND = '/HFQ/KD_API_SND'`, `GC_PROCID_KOMDAT_SND = '/HFQ/KOMDAT_SND'`

*Business message IDs*: `GC_BMID_PARTI`, `GC_BMID_PAR_D/E/M/O/S/U/V`; basic process: `CO_BASIC_PROC_PARTIN_IN = '/HFQ/I_PAR'`, `CO_BASIC_PROC_PARTIN_OUT = '/HFQ/E_PAR'`

*IDoc types*: `GC_IDOCTP_PARTIN = '/IDXGL/PARTIN_01'`, `GC_IDOCTP_PARTIN_02 = '/IDXGL/PARTIN_02'`

*Other*: `GC_BIKR_CODE_Z48 = 'Z48'`, `GC_TIME_CODE_Z41 = 'Z41'`, `GC_BANK_CODE_Z32 = 'Z32'`, `GC_CHECK_BIKR_BEGIN_DATE = '20250331'`, `GC_DATE_PARTIN_10B = '20230401'`

---

## Function Groups

### `/HFQ/FG_IDOC_EXIT` — IDoc Mapping Exit Functions

Contains four function modules used as IDoc mapping exits for PARTIN messages. All share the same interface pattern: `IV_DIRECTION TYPE E_DEXDIRECTION`, `IS_MAPEXIT TYPE /IDXGC/S_MAP_EXIT_CONFIG`, changing `CV_MAX_SEGNUM`, `CS_PROCESS_DATA TYPE /IDXGC/S_PROC_DATA`, `CS_IDOC_DATA TYPE EDEX_IDOCDATA`, `CT_IDOC_MAPPING TYPE /IDXGC/T_MAP_FIELD_CONFIG optional`; raises `/IDXGC/CX_IDE_ERROR`.

Each function instantiates `/HFQ/CL_PARTIN_MAPPING` and dispatches to outbound (`fill_sg4_nad04_segments_out`) or inbound (`proc_sg4_nad04_segments_in`) based on `IV_DIRECTION`.

| Function | Description |
|---|---|
| `/HFQ/EXIT_SG1_RFF_2` | Mapping exit for SG1 RFF segment (reference qualifier handling) |
| `/HFQ/EXIT_SG4_NAD_PARTIN` | Mapping exit for SG4 NAD segment (party identification, version `gc_msg_version_10`) |
| `/HFQ/EXIT_SG4_NAD_PARTIN_1A` | Mapping exit for SG4 NAD segment, variant 1A |
| `/HFQ/EXIT_SG4_NAD_PARTIN_1B` | Mapping exit for SG4 NAD segment, variant 1B |

*Note: `/HFQ/CL_PARTIN_MAPPING` referenced in the implementations is not in this package; it belongs to a sibling package in the HFQ namespace.*

### `/HFQ/FG_INTCOD` — Intcode View Maintenance

Standard SAP view-maintenance function group for `/HFQ/INTCODES`. Contains `TABLEFRAME_/HFQ/FG_INTCOD` and `TABLEPROC_/HFQ/FG_INTCOD` (generated table-maintenance functions). Dynpros 0001 (detail screen) and 3200 (overview with table control) for fields `SERVICE`, `DIVISION`, `INTCODE`.

### `/HFQ/FG_TABMAINT` — Customizing Table Maintenance

Standard SAP view-maintenance function group for the PARTIN customizing tables. Contains generated table-maintenance functions and a large set of dynpros (screens 0001, 0002, 0003, 0500, 1100, 1101, 1200, 1300, 1500, 1600, 1601, 2000, 3100, 3200, 3300, 4000, 4100, 4200, 4300) covering all mandatory-data customizing views. *Not fully documented due to package size (84,634 token file exceeds read limit).*

---

## Interfaces

This package uses a class-as-interface pattern: `/HFQ/IF_PARTIN_CONSTANTS` is technically declared as a `CLASS` with `PUBLIC` visibility and no implementation (empty `ENDCLASS.`) but functions as a constants interface. There are no ABAP `INTERFACE` objects in this package.

---

## Tables / Data Definitions

### Runtime Transparent Tables (Application Data, APPL0)

These tables store the actual PARTIN versioned master data, keyed by the structure `/HFQ/S_KD_KEY` (`SERVICEID TYPE SERVICE_PROV`, `SERVICEID_RECEIVER TYPE /HFQ/DE_KD_SERVICEID_EMPFANG`, `VERSION TYPE /HFQ/DE_KD_VERSION`).

| Table | Description |
|---|---|
| `/HFQ/PTCOMV` | PARTIN header (Kopftabelle): key + `/HFQ/S_KD_VALIDITY` (VON/BIS dates) + `/HFQ/S_KD_HDR_DATA` (status, prev-version, doc-status, etc.) + admin fields. Root table for lock object `/HFQ/KOMDATA_SP`. |
| `/HFQ/PTCOMP` | Contact persons per version (Ansprechpartner): key + `CONTACT_TYPE` + `CONTACT_ID` (FK to PTCONTP) + `INFO_CONTACT_ID` (FK to PTCONTI) |
| `/HFQ/PTCONTP` | Contact postal address master (PARTIN SG4): keyed by `CONTACT_ID` + address include `/HFQ/S_KD_CONTACT_ADDR` (name, street, house number, city, post code, country, email, fax, tel) |
| `/HFQ/PTCONTI` | Communication contact master (PARTIN SG7): keyed by `INFO_CONTACT_ID` + communication include `/HFQ/S_KD_CONTACT_COMM` (info_contact name, email, tel, fax) |
| `/HFQ/PTCONTBK` | Bank accounts per version: key + `/HFQ/S_KD_BANK_KEY` (bank_type, counter) + `/HFQ/S_KD_BANK_DATA` (IBAN, BIC, bank name, account holder) |
| `/HFQ/PTBIKREIS` | Balance groups per version: key + `/HFQ/S_KD_BIKREIS_KEY` (bikreis_type, counter) + `/HFQ/S_KD_BIKREIS_DATA` (EIC code, name) |
| `/HFQ/PTCOMTIME` | Availability windows per version: key + `/HFQ/S_KD_AVAIL_KEY` (avail_type, counter, days) + `/HFQ/S_KD_AVAIL_DATA` (from/to times) |

### Process Step Extension Tables (APPL0)

These tables extend the IDXGC process document with PARTIN-specific step data, managed by `/HFQ/CL_KOMDATA_IM_PROC_DOC_DB`.

| Table | Description |
|---|---|
| `/HFQ/PRST_AVAIL` | Process step data for availability windows |
| `/HFQ/PRST_BANK` | Process step data for bank accounts |
| `/HFQ/PRST_BIKR` | Process step data for balance groups |

### Customizing Transparent Tables (APPL2 — Customizing class C)

| Table | Description | Key fields |
|---|---|---|
| `/HFQ/INTCODES` | Service types and their internal codes (intcode mapping) | `SERVICE SERCODE`, `DIVISION SPARTE` → `INTCODE` |
| `/HFQ/MAND_COM` | Mandatory contact types per sender/receiver service type combination | `CONTACT_TYPE`, `STYPE_SENDER`, `STYPE_RECEIVER` |
| `/HFQ/MAND_BNK` | Mandatory bank data types per sender service type, with min/max cardinality | `BANK_TYPE`, `STYPE_SENDER` → `MIN_OCCURRENCES`, `MAX_OCCURRENCES` |
| `/HFQ/MAND_BIKR` | Mandatory balance-group types per sender/receiver service type | `BIKREIS_TYPE`, `STYPE_SENDER`, `STYPE_RECEIVER` → min/max occurrences |
| `/HFQ/MAND_HDR` | Mandatory header fields per PARTIN version | `PARTIN_VERSION`, `FIELD FIELDNAME` → `MANDATORY` |
| `/HFQ/MAND_SPART` | Divisions (Sparten) for which PARTIN is sent | `SPARTE` |
| `/HFQ/PTCUST` | General customizing parameters per logical system | `LOGSYS`, `PARAM` → `VALUE`; known parameter: `MANUAL_BANK_EDIT` |

### Customizing Type Tables (catalogue type: lookup/reference)

| Table | Description |
|---|---|
| `/HFQ/PTBANKTYPE` | Bank account type codes (Customizing Bankkonto-Typen) |
| `/HFQ/PTBIKRTYPE` | Balance-group type codes (Customizing Bilanzkreis) |
| `/HFQ/PTBIKRTYPE` | Balance-group type codes |
| `/HFQ/PTCONTTYPE` | Contact type codes (Customizing Kontakttypen) |
| `/HFQ/PTTIMETYPE` | Availability time type codes (Customizing Erreichbarkeitszeiten) |

### Structures (INTTAB — internal use)

**Key/composite structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_KEY` | Common key: `SERVICEID SERVICE_PROV`, `SERVICEID_RECEIVER`, `VERSION` |
| `/HFQ/S_KD_VALIDITY` | Validity period: `VON` (start date), `BIS` (end date) |
| `/HFQ/S_KD_HDR` | Full header for display (includes key, validity, data, admin) |
| `/HFQ/S_KD_HDR_DATA` | Header data fields (status, prev-version, doc-status, EDI email) |
| `/HFQ/S_KD_HDR_TIMESLICE` | Time-slice header data |

**Contact structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_CONTACT` | PARTIN contact for display (key + contact type + address + comm) |
| `/HFQ/S_KD_CONTACT_ALL` | All-version contact data |
| `/HFQ/S_KD_CONTACT_KEY` | Contact key fields (contact type, contact_id, info_contact_id) |
| `/HFQ/S_KD_CONTACT_ADDR` | Contact postal address (name, street, house num, city, post code, country, email, fax, tel) |
| `/HFQ/S_KD_CONTACT_COMM` | Information-contact communication fields (info_contact name, email, tel, fax) |
| `/HFQ/S_KD_CONTACT_DATA_ADD` | Additional/dependent contact fields |
| `/HFQ/S_NAMEADDR_DETAILS` | Extended name/address details for KOMDATA |
| `/HFQ/S_KD_NAMEADDR_INCLUDE` | Additional process address data for KOMDATA |

**Bank structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_BANK` | PARTIN bank data for display |
| `/HFQ/S_KD_BANK_ALL` | All-version bank data |
| `/HFQ/S_KD_BANK_KEY` | Bank key (bank type, counter) |
| `/HFQ/S_KD_BANK_DATA` | Bank data fields (IBAN, BIC, bank name, account holder) |
| `/HFQ/S_KD_BANK_DATA_ADD` | Additional bank data fields |
| `/HFQ/S_KD_BANKAVAIL` | Extension combining bank and availability data |

**Availability structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_AVAIL` | PARTIN availability time for display |
| `/HFQ/S_KD_AVAIL_ALL` | All-version availability data |
| `/HFQ/S_KD_AVAIL_KEY` | Availability key (avail type, counter, day-of-week flags) |
| `/HFQ/S_KD_AVAIL_DATA` | Availability data fields (from time, to time) |
| `/HFQ/S_KD_AVAIL_DATA_ADD` | Additional availability fields |

**Balance-group structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_BIKREIS` | PARTIN balance-group for display |
| `/HFQ/S_KD_BIKREIS_ALL` | All-version balance-group data |
| `/HFQ/S_KD_BIKREIS_KEY` | Balance-group key (type, counter) |
| `/HFQ/S_KD_BIKREIS_DATA` | Balance-group data fields (EIC code, name) |
| `/HFQ/S_KD_BIKREIS_DATA_ADD` | Additional balance-group fields |

**Customizing structures**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_CUST_AVAIL` | Customizing required availability times (time_type, min/max occurrences) |
| `/HFQ/S_KD_CUST_BANK` | Customizing required bank data (bank_type, min/max occurrences) |
| `/HFQ/S_KD_CUST_BIKREIS` | Customizing required balance groups (bikreis_type, min/max occurrences) |
| `/HFQ/S_KD_CUST_CONT` | Customizing required contact persons (contact_type, min/max occurrences) |

**Other**:

| Structure | Description |
|---|---|
| `/HFQ/S_KD_ERROR` | Error message structure |
| `/HFQ/S_KD_PROC_DATA2_INCLUDE` | Additional process step data for KOMDATA (includes avail, bikreis, bank sub-tables) |

### Table Types (TTYP)

One table type per data structure, following the naming pattern `T_KD_*` (e.g., `/HFQ/T_KD_AVAIL`, `/HFQ/T_KD_AVAIL_ALL`, `/HFQ/T_KD_BANK`, `/HFQ/T_KD_BIKREIS`, `/HFQ/T_KD_CONTACT`, `/HFQ/T_KD_ERROR`, `/HFQ/T_KD_CONTACT_ID`, `/HFQ/T_KD_INFO_CONTACT_ID`, `/HFQ/T_KD_CUST_AVAIL`, `/HFQ/T_KD_CUST_BANK`, `/HFQ/T_KD_CUST_BIKREIS`, `/HFQ/T_KD_CUST_CONT`, and types for all six PT* tables: `/HFQ/T_KD_PTBIKREIS`, `/HFQ/T_KD_PTCOMP`, `/HFQ/T_KD_PTCOMTIME`, `/HFQ/T_KD_PTCOMV`, `/HFQ/T_KD_PTCONTBK`, `/HFQ/T_KD_PTCONTI`, `/HFQ/T_KD_PTCONTP`).

---

## Domains and Data Elements

### Domains

| Domain | Type | Length | Fixed values | Description |
|---|---|---|---|---|
| `/HFQ/DO_KD_VERSION_STATUS` | CHAR | 10 | SENT, RECEIVED, BACKLOG, UNSAVED, READY, NO_DATA | Version status (Versionsstatus Kontaktdaten) |
| `/HFQ/DO_KD_AVAIL_TYPE` | *Inferred from data element* | — | — | Availability type code |
| `/HFQ/DO_KD_BANK_TYPE` | *Inferred* | — | — | Bank account type code |
| `/HFQ/DO_KD_BIKREIS_TYPE` | *Inferred* | — | — | Balance-group type code |
| `/HFQ/DO_KD_CONTACT_ID` | *Inferred* | — | — | Contact ID (number range /HFQ/KDCID based) |
| `/HFQ/DO_KD_CONTACT_TYPE` | *Inferred* | — | — | Contact type code |
| `/HFQ/DO_KD_DOCU_STATUS` | *Inferred* | — | — | Document status |
| `/HFQ/DO_KD_INFO_CONTACT_ID` | *Inferred* | — | — | Information contact ID (number range /HFQ/KDICI) |
| `/HFQ/DO_KD_INTCODES` | *Inferred* | — | — | Internal code values |
| `/HFQ/DO_KD_PARTY_FUNC_QUAL` | *Inferred* | — | — | NAD party function qualifier |
| `/HFQ/DO_KD_SERVICEID_EMPFANG` | *Inferred* | — | — | Receiver service provider ID |
| `/HFQ/DO_KD_VERSION` | NUM9 | 9 | — | Version number (9-digit numeric, fed by number range /HFQ/KDVER) |

*Note: Domain field lengths and fixed values for all domains except `/HFQ/DO_KD_VERSION_STATUS` are inferred from data element names and usage context. Not verified by reading individual domain XML files.*

### Data Elements (67 total)

All data elements are prefixed `/HFQ/DE_KD_*`. Selected key elements:

| Data Element | Description |
|---|---|
| `/HFQ/DE_KD_VERSION` | PARTIN current version number (domain: `/HFQ/DO_KD_VERSION`, NUM9) |
| `/HFQ/DE_KD_VERSION_STATUS` | Version status (domain: `/HFQ/DO_KD_VERSION_STATUS`) |
| `/HFQ/DE_KD_SERVICEID_EMPFANG` | Receiver service provider ID |
| `/HFQ/DE_KD_CONTACT_TYPE` | Contact type code (Z10–Z33) |
| `/HFQ/DE_KD_CONTACT_TYPE_TXT` | Contact type description text |
| `/HFQ/DE_KD_CONTACT_ID` | Unique contact person ID (9-digit, NR-managed) |
| `/HFQ/DE_KD_INFO_CONTACT_ID` | Unique information-contact ID (9-digit, NR-managed) |
| `/HFQ/DE_KD_BANK_TYPE` | Bank account type code |
| `/HFQ/DE_KD_BANK_TYPE_TXT` | Bank account type description |
| `/HFQ/DE_KD_BIKREIS_TYPE` | Balance-group type code |
| `/HFQ/DE_KD_BIKREIS_TYPE_TXT` | Balance-group type description |
| `/HFQ/DE_KD_AVAIL_TYPE` | Availability time type |
| `/HFQ/DE_KD_AVAIL_TYPE_TXT` | Availability type description |
| `/HFQ/DE_KD_IBAN` | IBAN |
| `/HFQ/DE_KD_BIC` | BIC |
| `/HFQ/DE_KD_ACC_HOLDER` | Account holder name |
| `/HFQ/DE_KD_BANK` | Bank name |
| `/HFQ/DE_KD_ADDRESS` | Street address |
| `/HFQ/DE_KD_CITY` | City |
| `/HFQ/DE_KD_POSTAL_CODE` | Postal code |
| `/HFQ/DE_KD_COUNTRY_CODE` | Country code |
| `/HFQ/DE_KD_HOUSENUM` | House number |
| `/HFQ/DE_KD_EMAIL` | Email address |
| `/HFQ/DE_KD_EDI_EMAIL` | EDI email address |
| `/HFQ/DE_KD_FAX` | Fax number |
| `/HFQ/DE_KD_TEL` | Telephone number |
| `/HFQ/DE_KD_URL` | URL |
| `/HFQ/DE_KD_CONTACT` | Contact person name |
| `/HFQ/DE_KD_VON` | Start date (validity from) |
| `/HFQ/DE_KD_BIS` | End date (validity to) |
| `/HFQ/DE_KD_PREV_VERSION` | Previous version reference |
| `/HFQ/DE_KD_DOCU_STATUS` | Document status |
| `/HFQ/DE_KD_CUSTPARAM` | Customizing parameter name |
| `/HFQ/DE_KD_CUSTVALUE` | Customizing parameter value |
| `/HFQ/DE_KD_MANDATORY` | Mandatory flag |
| `/HFQ/DE_KD_OCCURRENCE` | Occurrence count |
| `/HFQ/DE_KD_OCCURRENCE_MIN` | Minimum occurrences |
| `/HFQ/DE_KD_OCCURRENCE_MAX` | Maximum occurrences |
| `/HFQ/DE_KD_PARTY_FUNC_QUAL` | NAD party function qualifier (local copy of IDXGC type) |
| `/HFQ/DE_KD_STYPE_SENDER` | Sender service type (intcode) |
| `/HFQ/DE_KD_STYPE_RECEIVER` | Receiver service type (intcode) |
| `/HFQ/DE_KD_PARTIN_VERSION` | PARTIN version string (for header customizing) |
| `/HFQ/DE_KD_BIKREIS_COUNTER` | Balance-group counter |
| `/HFQ/DE_KD_INTCODES` | Internal code value |
| `/HFQ/DE_KD_OWN_SERV` | Own service flag |
| `/HFQ/DE_KD_REC_SERV` | Receiver service flag |
| `/HFQ/DE_KD_DOWN_LINK` | Download link |
| `/HFQ/DE_KD_COMMERCIAL_REG_NUM` | Commercial register number |
| `/HFQ/DE_KD_COURT` | Court (Registergericht) |
| `/HFQ/DE_KD_TAX_ID` | Tax identification number |
| `/HFQ/DE_KD_TAX_NUM` | Tax number |
| `/HFQ/DE_KD_ADD_NAME` | Additional name |
| `/HFQ/DE_KD_PO_BOX` | PO box |
| `/HFQ/DE_KD_INFO_CONTACT` | Information contact name |
| `/HFQ/DE_KD_INFO_CONTACT_ID` | Information contact ID (alias of domain) |

---

## Other Objects

### Number Range Objects

| Object | Domain length | Buffer | Description |
|---|---|---|---|
| `/HFQ/KDVER` | NUM9 | Yes (10 numbers) | PARTIN version numbers |
| `/HFQ/KDCID` | *Inferred from usage in `GENERATE_CONTACT_ID`* | *Inferred* | Contact person IDs |
| `/HFQ/KDICI` | *Inferred from usage in `GENERATE_INFO_CONTACT_ID`* | *Inferred* | Information contact IDs |

*KDCID and KDICI lengths and buffer settings: not verified by reading individual NROB XML files.*

### Lock Object

**`/HFQ/KOMDATA_SP`** — Enqueue object on `/HFQ/PTCOMV`, exclusive lock mode (E), locking by `(MANDT, SERVICEID, VERSION)`. The `SERVICEID` field uses parameter ID `EESERVPROVID` and the `SERVICEPROVIDER` search help.

### Message Class

**`/HFQ/MSG_KOMDATA`** — "Nachrichtenklasse für PARTIN-Kontaktdatenblatt". Selected messages (verified):

| No. | Text |
|---|---|
| 000 | Kontaktyp &1 muss gefüllt werden. |
| 001 | Serviceanbieter fehlt |
| 002 | Kontaktpartner für &1 fehlt |
| 003 | Infokontaktpartner für &1 fehlt |
| 004 | Infokontakt-ID &1 nicht gefunden |
| 005 | Name des Infokontakts &1 fehlt. |
| 006 | E-Mail des Infokontakts &1 fehlt. |
| 007 | Telefonnummer des Infokontakts &1 fehlt. |
| 008 | Kontakt-ID &1 nicht gefunden |
| 009 | Name des Kontakts &1 fehlt. |
| 010 | Adresse des Kontakts &1 fehlt. |
| 011 | Stadt des Kontakts &1 fehlt. |
| 051 | Ansprechpartner-Customizing für Senden von &1 nach &2 nicht vorhanden |
| 054 | Bankdaten-Customizing für Senden von &1 nicht vorhanden |
| 063 | Aufräumen der Datenbank &1 fehlgeschlagen |
| 064 | Löschen des Entwurfs fehlgeschlagen |
| 072 | Schreiben in Datenbank &1 fehlgeschlagen |

### Search Help

**`/HFQ/KOMDATA_SH_CONTACT_TYPE`** — Search help for contact types.

### Enhancement Implementation

**`/HFQ/IM_KOMDATA_PROCESS`** — BAdI implementation of `/IDXGC/BADI_PROCESS_DOC_DB` in spot `/IDXGC/ES_PROCESS`, implemented by class `/HFQ/CL_KOMDATA_IM_PROC_DOC_DB`. Active. Filter: `IV_PROC_CLUSTER = '/HFQ/KOMDATA'`.

### Include Program

**`/HFQ/IFG_IDOC_EXIT01`** — Include used by function group `/HFQ/FG_IDOC_EXIT`. *Content not documented due to secondary role.*

### Table Maintenance Objects (TOBJ)

View maintenance objects for all customizing tables: `/HFQ/INTCODESS`, `/HFQ/MAND_BIKRS`, `/HFQ/MAND_BNKS`, `/HFQ/MAND_COMS`, `/HFQ/MAND_HDRS`, `/HFQ/MAND_SPARTS`, `/HFQ/PTBANKTYPES`, `/HFQ/PTBIKRTYPES`, `/HFQ/PTCOMPS`, `/HFQ/PTCOMTIMES`, `/HFQ/PTCOMVS`, `/HFQ/PTCONTBKS`, `/HFQ/PTCONTIS`, `/HFQ/PTCONTPS`, `/HFQ/PTCONTTYPES`, `/HFQ/PTCUSTS`, `/HFQ/PTTIMETYPES`. These are generated by SM30 view maintenance and delegate to `/HFQ/FG_TABMAINT` or `/HFQ/FG_INTCOD`.
