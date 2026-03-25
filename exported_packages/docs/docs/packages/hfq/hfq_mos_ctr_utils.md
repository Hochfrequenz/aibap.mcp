# Package: HFQ / MOS_CTR_UTILS

**Description**: Unterpaket - Entwicklungen MSB-Seite (Sub-package for MSB-side developments, i.e. Messstellenbetreiber / metering point operator billing utilities)
**Original language**: German (D)
**Number of objects**: 23 source files across 1 class, 2 function groups, 1 report, 3 transparent tables, 1 internal structure, 1 table type, 2 view maintenance objects (TOBJ), 2 domains, 5 data elements, 1 message class, 1 namespace object, 1 classification assignment (AVAS)

---

## Executive Summary

MOS_CTR_UTILS is the MSB billing utility sub-package within the HFQ namespace. Its central purpose is **mass creation and lifecycle management of MOS (Messstellenbetrieb) contracts** during migration. The package covers:

1. **A constants class** (`/HFQ/MOS_CTR_CONSTANTS`) that holds all system-wide configuration values — Sparten, operands, EDI partner addresses, price classifications, voltage levels, and migration table names — and reads the customer-specific parameter table (`/HFQ/CTR_PARA`) at class construction time, making parameters available globally via `GT_MOSB_PARAMS`.

2. **A mass-creation report** (`/HFQ/RP_CREATE_MOS_CTR_MASS`) that drives the full migration cycle: upload of a spreadsheet into the migration staging table, creation of ISU contracts and QUOTES processes via the `/US4G/` contract API, and archiving of completed records.

3. **Two migration/configuration tables** with their SM30 view maintenance function groups: the migration staging table (`/HFQ/CTR_MIG_CT`) and the customer parameter table (`/HFQ/CTR_PARA`).

4. **Supporting DDIC objects**: an archive table, an upload structure, a table type, two domains, five data elements, and a message class with 21 messages (200–220) covering the entire migration log output.

The code uses OO-style local classes inside the report (refactored from FORMs), SAP Application Log (BAL), and the `/US4G/IF_CTR_API` contract interface.

---

## Classes

### `/HFQ/MOS_CTR_CONSTANTS`

**File**: `src/#hfq#mos_ctr_constants.clas.abap` / `.xml`
**Description** (from XML): "Kontanten MSB-Abrechnung" (Constants for MSB billing)
**Visibility**: `PUBLIC FINAL`
**Instantiation**: `CREATE PUBLIC`

#### Class Attribute: `GT_MOSB_PARAMS`

| Name | Type | Description |
|------|------|-------------|
| `GT_MOSB_PARAMS` | `/HFQ/T_MOS_CTR_PARA` | Table of customer-individual parameters, populated at class load time from `/HFQ/CTR_PARA` |

#### `CLASS_CONSTRUCTOR`

Performs a full `SELECT * FROM /hfq/ctr_para INTO TABLE gt_mosb_params`, making all configured parameters available as a class-level attribute.

#### Constants

| Constant | Type | Value | Description |
|----------|------|-------|-------------|
| `GC_MOSB_SPARTE_STR_AUS` | `SPARTE` | `'10'` | Division: electricity feed-out |
| `GC_MOSB_SPARTE_STR_EIN` | `SPARTE` | `'15'` | Division: electricity feed-in |
| `GC_MOSB_OPERAND_AUS_HT` | `E_OPERAND` | `''` | Operand: feed-out high tariff *(value left intentionally blank)* |
| `GC_MOSB_OPERAND_AUS_HT_ABZ` | `E_OPERAND` | `''` | Operand: feed-out HT deduction |
| `GC_MOSB_OPERAND_AUS_NT` | `E_OPERAND` | `''` | Operand: feed-out low tariff |
| `GC_MOSB_OPERAND_AUS_NT_ABZ` | `E_OPERAND` | `''` | Operand: feed-out NT deduction |
| `GC_MOSB_FIELD_BAS_PRI_CL` | `FIELDNAME` | `''` | Field name: base price class *(blank)* |
| `GC_MOSB_FIELD_BAS_ME` | `FIELDNAME` | `''` | Field name: base unit of measure *(blank)* |
| `GC_MOSB_OPERAND_EIN` | `E_OPERAND` | `''` | Operand: feed-in *(blank)* |
| `GC_SWITCHTYPE_MOS_QUOTES` | `/US4G/DE_PROC_TYPE` | `'40'` | Switch type for MOS QUOTES process |
| `GC_PROCID_MOS_QUOTES` | `/US4G/DE_PROC_ID` | `'MOSB0001'` | Process ID for MOS QUOTES |
| `GC_MOS_SERVICE_INTCODE` | `INTCODE` | `''` | Service internal code *(blank)* |
| `GC_MOSB_VKTYP_AN` | `VKTYP_KK` | `'MO'` | Contract account type |
| `GC_MOSB_PARA_VORL_VK` | `CHAR30` | `'MOSB_VK_MO'` | Parameter name: contract template |
| `GC_MOSB_STERKZ_DEF` | `ERMWSKZ` | `'E0'` | Default tax determination indicator |
| `GC_MOSB_REC_POR` | `EDI_RCVPOR` | `'SAPHFQ'` | EDI receiver port |
| `GC_MOSB_REC_PRN` | `EDI_RCVPRN` | `'HFQCLNT200'` | EDI receiver partner number |
| `GC_MOSB_REC_PRT` | `EDI_RCVPRT` | `'LS'` | EDI receiver partner type |
| `GC_MOSB_SND_POR` | `EDI_SNDPOR` | `'HFQ_100'` | EDI sender port |
| `GC_MOSB_SND_PRN` | `EDI_SNDPRN` | `'LS_HFQ'` | EDI sender partner number |
| `GC_MOSB_SND_PRT` | `EDI_SNDPRT` | `'SP'` | EDI sender partner type |
| `GC_MOSB_DEF_LF` | `SERVICE_PROV` | `'LS_HFQ'` | Default Lieferant (supplier) service provider |
| `GC_MOSB_DEF_MSB` | `SERVICE_PROV` | `'MS_HFQ'` | Default MSB service provider |
| `GC_MOSB_PRICL_MME` | `/US4G/DE_PRICE_CLASS` | `'Z25'` | Price classification: MME |
| `GC_MOSB_PRICL_TRA` | `/US4G/DE_PRICE_CLASS` | `'Z26'` | Price classification: TRA |
| `GC_MOSB_ADD_PRICL_NS` | `/US4G/DE_PRICE_CLASS_ADD` | `'Z11'` | Additional price classification: NS (low voltage) |
| `GC_MOSB_ADD_PRICL_MS` | `/US4G/DE_PRICE_CLASS_ADD` | `'Z10'` | Additional price classification: MS (medium voltage) |
| `GC_MOSB_ADD_PRICL_HS` | `/US4G/DE_PRICE_CLASS_ADD` | `'Z09'` | Additional price classification: HS (high voltage) |
| `GC_MOSB_ADD_PRICL_HOES` | `/US4G/DE_PRICE_CLASS_ADD` | `'Z08'` | Additional price classification: HöS (extra-high voltage) |
| `GC_MOSB_SPANNEB_NS_IN` | `SPEBENE` | `'IN'` | Voltage level: NS internal |
| `GC_MOSB_SPANNEB_NS_M3` | `SPEBENE` | `'M3'` | Voltage level: NS M3 |
| `GC_MOSB_SPANNEB_MS_IM` | `SPEBENE` | `'IM'` | Voltage level: MS internal |
| `GC_MOSB_SPANNEB_MS_AM` | `SPEBENE` | `'AM'` | Voltage level: MS external |
| `GC_MOSB_SPANNEB_HS_HM` | `SPEBENE` | `'HM'` | Voltage level: HS |
| `GC_MOSB_MSG_ID_MOSB` | `SY-MSGID` | `'/HFQ/MOS_CTR'` | Message class ID used throughout the package |
| `GC_MOSB_PRICL_INIT` | `/US4G/DE_PRICE_CLASS` | `'Z31'` | Initial price classification |
| `GC_MOSB_MIG_CREA_TAB` | `TABNAME` | `'/HFQ/CTR_MIG_CT'` | Migration staging table name |
| `GC_MOSB_MIG_ENDARC_TAB` | `TABNAME` | `'/HFQ/MOSB_END_AR'` | End-of-contract archive table name *(referenced externally)* |
| `GC_MOSB_MIG_END_TAB` | `TABNAME` | `'/HFQ/MOSB_END_CT'` | End-of-contract table name *(referenced externally)* |
| `GC_MOSB_MIG_ARC_TAB` | `TABNAME` | `'/HFQ/CTR_MIG_AR'` | Migration archive table name |
| `GC_MOSB_MIG_UPL_END_TAB` | `TABNAME` | `'/MOSB_MIG_UPL'` | Upload end-of-contract table name |
| `GC_MOSB_MIG_UPL_TAB` | `TABNAME` | `'/MOSB_END_UPL'` | Upload end structure name |
| `GC_MOSB_MIG_SUBLOG_CTR` | `BALSUBOBJ` | `'/HFQ/MOS_CTR'` | Application log sub-object for contract creation |
| `GC_MOSB_MIG_SUBLOG_UPL` | `BALSUBOBJ` | `'/HFQ/MOS_CTR'` | Application log sub-object for upload |
| `GC_MOSB_LOG_OBJ` | `BALOBJ_D` | `'/HFQ/'` | Application log object |
| `GC_MOSB_PARA_EIG_MSB` | `/HFQ/MOS_CTR_PARA_NAME` | `'EIG_MSB_SA'` | Parameter key: own MSB service provider |

---

## Function Groups

### `#HFQ#CTR_MIG_CT` — View Maintenance for Migration Staging Table

**Files**: `src/#hfq#ctr_mig_ct.fugr.*`
**Top include**: `/HFQ/LCTR_MIG_CTTOP` — declares the VIM state vector and table control `TCTRL_/HFQ/CTR_MIG_CT` (screen 0001)
**Message-ID**: `SV`

This is a standard SAP-generated SM30 View Maintenance function group for the table `/HFQ/CTR_MIG_CT`.

#### Function Modules

| Function Module | Purpose |
|-----------------|---------|
| `TABLEFRAME_/HFQ/CTR_MIG_CT` | Renders the overview screen (calls `PERFORM TABLEFRAME`) |
| `TABLEPROC_/HFQ/CTR_MIG_CT` | Processes user interactions (calls `PERFORM TABLEPROC`); global flag set |

**Parameters** (both FMs): `VIEW_ACTION`, `VIEW_NAME`, `CORR_NUMBER` (import); `DBA_SELLIST`, `DPL_SELLIST`, `EXCL_CUA_FUNCT`, `X_HEADER`, `X_NAMTAB` (tables); exceptions: `MISSING_CORR_NUMBER`, `SAVING_CORRECTION_FAILED` (TABLEPROC only).

**Screen 0001** — Dynpro overview for `/HFQ/CTR_MIG_CT`:
Table control `TCTRL_/HFQ/CTR_MIG_CT` with 5 columns (3 fixed):

| Column | Field | Key | Type |
|--------|-------|-----|------|
| 1 | `ANSCHLNUTZ` | Yes | ALPHA-converted, 10 chars |
| 2 | `MELO` | Yes | 50 chars, scrollable |
| 3 | `STARTDAT` | Yes | Date |
| 4 | `MOSB_CTR` | No | 12 chars, editable |
| 5 | `NO_CTR` | No | Checkbox |

---

### `#HFQ#CTR_PARA` — View Maintenance for Parameter Table

**Files**: `src/#hfq#ctr_para.fugr.*`
**Top include**: `/HFQ/LCTR_PARATOP` — declares the VIM state vector and table control `TCTRL_/HFQ/CTR_PARA` (screen 0001)
**Message-ID**: `SV`

Standard SAP-generated SM30 View Maintenance function group for `/HFQ/CTR_PARA`.

#### Function Modules

| Function Module | Purpose |
|-----------------|---------|
| `TABLEFRAME_/HFQ/CTR_PARA` | Renders the overview screen (calls `PERFORM TABLEFRAME`) |
| `TABLEPROC_/HFQ/CTR_PARA` | Processes user interactions (calls `PERFORM TABLEPROC`); global flag set |

**Parameters**: identical pattern to CTR_MIG_CT group above.

**Screen 0001** — Dynpro overview for `/HFQ/CTR_PARA`:
Table control `TCTRL_/HFQ/CTR_PARA` with 2 columns (1 fixed):

| Column | Field | Key | Type |
|--------|-------|-----|------|
| 1 | `NAME` | Yes | 10 chars, dropdown (F4 via domain fixed values) |
| 2 | `WERT` | No | 50 chars, editable |

---

## Reports

### `/HFQ/RP_CREATE_MOS_CTR_MASS`

**File**: `src/#hfq#rp_create_mos_ctr_mass.prog.abap` / `.xml`
**Title** (from text pool): "Report /HFQ/RP_CREATE_MOS_CTR_MASS"
**Description** (from source header): "Massenanlage MOS-Verträge (MSB-Abrechnung) — Refactored: FORMs → OO-Methoden, Bugfixes Error-Handling"
**Type**: Executable report (SUBC=1), fixed-point arithmetic, Unicode-checked

#### Selection Screen

The screen has two mutually exclusive modes selected by radio buttons:

- **Upload mode** (`P_UPL`): shows a file path field (`P_FILE`) and a "Download Vorlage" button
- **Execute mode** (`P_EXE`): shows single/mass selection group (`P_SEL`/`P_MASS`) with optional `P_MELO` and `P_AN` filter parameters

Three toolbar pushbuttons:
- `W_BUTTO1` (FC01): Open migration table maintenance (`/HFQ/CTR_MIG_CT`)
- `W_BUTTO2` (FC02): Archive completed contracts and open archive table
- `W_BUTTO4` (FC04): Download XLSX upload template

#### Local Classes

**`LCL_LOGGER`** — Application Log wrapper

| Method | Signature summary |
|--------|------------------|
| `CONSTRUCTOR` | Creates BAL log header (`BAL_LOG_CREATE`) with optional object/subobject |
| `ADD_MESSAGE` | Adds `ISU00_MESSAGE` to log via `BAL_LOG_MSG_ADD`; auto-assigns problem class from message type |
| `DISPLAY_LOG` | Shows log via `BAL_DSP_LOG_DISPLAY` |
| `SAVE_LOG` | Persists log via `BAL_DB_SAVE` |

**`LCL_APPLICATION`** — Main controller

| Method | Description |
|--------|-------------|
| `INITIALIZE` | Sets button icon labels and default radio button states |
| `BUILD_SCREEN` | Controls screen field visibility based on selected mode |
| `ON_USER_COMMAND` | Dispatches FC01/FC02/FC04/SEL user commands |
| `EXECUTE` | Top-level: creates log, delegates to `UPLOAD_FILE` or `CREATE_CONTRACTS`, saves and displays log |
| `CREATE_LOG` | Instantiates `LCL_LOGGER` with constants from `/HFQ/MOS_CTR_CONSTANTS` |
| `UPLOAD_FILE` | Reads XLSX via `TEXT_CONVERT_XLS_TO_SAP`; validates MELO (via `ISU_DB_EUITRANS_EXT_SINGLE`) and business partner (via `BUT000`); performs duplicate check; writes rows to `/HFQ/CTR_MIG_CT` |
| `CREATE_CONTRACTS` | Selects open migration rows; resolves own MSB from parameter `EIG_MSB_SA`; for each row: resolves internal MELO, determines connection object via `EUIINSTLN`/`EANL`/`EVBS`, creates contract via `/US4G/IF_CTR_API_MAIN~CREATE_CONTRACT`, creates position via `CREATE_POSITION`, checks for EE01 MALOs to skip, triggers QUOTES process via `/US4G/IF_CTR_API_PROC~TRIG_QUOTES_PROC`, writes contract ID and process document back to migration table |
| `ARCHIVE_CONTRACTS` | Moves rows with non-empty `MOSB_CTR` and `MOSB_PDOC` from `/HFQ/CTR_MIG_CT` to `/HFQ/CTR_MIG_AR` (with archiver name/date); uses COMMIT/ROLLBACK |
| `VIEW_MIGRATION_TABLE` | Calls `VIEW_MAINTENANCE_CALL` for `/HFQ/CTR_MIG_CT` |
| `VIEW_ARCHIVE_TABLE` | Calls `VIEW_MAINTENANCE_CALL` for `/HFQ/CTR_MIG_AR` |
| `DOWNLOAD_TEMPLATE` | Uses `REUSE_ALV_FIELDCATALOG_MERGE` on `/MOSB_END_UPL` structure, then `GUI_DOWNLOAD` (DBF/XLSX) with `CL_GUI_FRONTEND_SERVICES~FILE_SAVE_DIALOG` |
| `BUILD_MESSAGE` | Helper: fills `ISU00_MESSAGE` from explicit message ID/number/type/variables |
| `LOG_SYST_MESSAGE` | Helper: logs current `SY-MSG*` fields via `BUILD_MESSAGE` |

#### Key External API Dependencies

| API | Purpose |
|-----|---------|
| `ISU_DB_EUITRANS_EXT_SINGLE` | Resolve external MeLo to internal point |
| `ISU_DB_EUITRANS_INT_SINGLE` | Reverse lookup internal → external MeLo |
| `ISU_DB_ESERVPROV_SINGLE` | Validate MSB service provider |
| `/US4G/CL_CTR_FACTORY=>GET_CTR_API` | Factory: get contract API instance |
| `/US4G/IF_CTR_API_MAIN~CREATE_CONTRACT` | Create MOS contract |
| `/US4G/IF_CTR_API_MAIN~CREATE_POSITION` | Add MeLo position to contract |
| `/US4G/IF_CTR_API_MAIN~DELETE_CONTRACT` | Rollback: delete contract if position creation fails |
| `/US4G/IF_CTR_API_PROC~TRIG_QUOTES_PROC` | Trigger QUOTES (offer) process |
| `/US4G/IF_CONSTANTS_BILLING~GC_TS_REASON_SERVICE_CHANGE` | Creation reason constant |
| `BAL_LOG_CREATE`, `BAL_LOG_MSG_ADD`, `BAL_DSP_LOG_DISPLAY`, `BAL_DB_SAVE` | Application log |
| `TEXT_CONVERT_XLS_TO_SAP` | Excel upload |
| `GUI_DOWNLOAD` | Template download |

---

## Tables / Data Definitions

### `/HFQ/CTR_MIG_CT` — Migration Staging Table

**File**: `src/#hfq#ctr_mig_ct.tabl.xml`
**Description**: "Migrationstabelle für anzulegende MOS-Verträge" (Migration table for MOS contracts to be created)
**Table class**: Transparent, client-dependent, main table flag, customizing content (`CONTFLAG=C`), no buffering

| Field | Key | Data element / Roll | Description |
|-------|-----|---------------------|-------------|
| `MANDT` | Yes | `MANDT` | Client |
| `ANSCHLNUTZ` | Yes | `/US4G/DE_CU_BP` | Connection user (business partner) |
| `MELO` | Yes | `/US4G/DE_MELO_EXT` | External metering point identifier |
| `STARTDAT` | Yes | `/US4G/DE_START_DATE` | Start date of the MOS contract |
| `MOSB_CTR` | No | `/US4G/DE_CTR_ID` | Created MOS contract ID (filled after creation) |
| `MOSB_PDOC` | No | `/APE/DE_DOC_NO` | QUOTES process document number (filled after trigger) |
| `NO_CTR` | No | `/HFQ/MOS_CTR_NO_CTR` | Flag: suppress contract creation for this row |

**View maintenance**: TOBJ `/HFQ/CTR_MIG_CTS`, area `/HFQ/CTR_MIG_CT`, type 1 (table view), SM30-maintainable.

---

### `/HFQ/CTR_MIG_AR` — Migration Archive Table

**File**: `src/#hfq#ctr_mig_ar.tabl.xml`
**Description**: "Migrationstabelle für anzulegende MOS-Verträge" (archive variant)
**Table class**: Transparent, client-dependent, customizing content (`CONTFLAG=C`), no buffering
**Logging**: `TABLE_LOGGING_REQUIREMENT = NOT_REQUIRED` (per AVAS classification)

Identical structure to `/HFQ/CTR_MIG_CT` plus two archiving audit fields:

| Field | Key | Data element | Description |
|-------|-----|--------------|-------------|
| `MANDT` | Yes | `MANDT` | Client |
| `ANSCHLNUTZ` | Yes | `/US4G/DE_CU_BP` | Connection user |
| `MELO` | Yes | `/US4G/DE_MELO_EXT` | External metering point |
| `STARTDAT` | Yes | `/US4G/DE_START_DATE` | Start date |
| `MOSB_CTR` | No | `/US4G/DE_CTR_ID` | MOS contract ID |
| `MOSB_PDOC` | No | `/APE/DE_DOC_NO` | Process document |
| `NO_CTR` | No | `/HFQ/MOS_CTR_NO_CTR` | Suppress flag |
| `ARCHIV_VON` | No | `/HFQ/MOS_CTR_ARCHIVIERT_VON` | Archived by (user name) |
| `ARCHIV_AM` | No | `/HFQ/MOS_CTR_ARCHIVIERT_AM` | Archived on (date) |

---

### `/HFQ/CTR_PARA` — Customer Parameter Table

**File**: `src/#hfq#ctr_para.tabl.xml`
**Description**: "Tabelle für kundenindividuelle Parameter" (Table for customer-specific parameters)
**Table class**: Transparent, client-dependent, application content (`CONTFLAG=A`), no buffering

| Field | Key | Data element | Description |
|-------|-----|--------------|-------------|
| `MANDT` | Yes | `MANDT` | Client |
| `NAME` | Yes | `/HFQ/MOS_CTR_PARA_NAME` | Parameter name (domain-validated; allowed values: `VORL_VK_MO`, `EIG_MSB_SA`) |
| `WERT` | No | `/HFQ/MOS_CTR_PARA_WERT` | Parameter value (CHAR50) |

**View maintenance**: TOBJ `/HFQ/CTR_PARAS`, area `/HFQ/CTR_PARA`, type 1 (table view), SM30-maintainable, application category.

---

### `/HFQ/MOS_CTR_MIG_UPL` — Upload Structure

**File**: `src/#hfq#mos_ctr_mig_upl.tabl.xml`
**Description**: "Struktur zum Upload der Migrationstabelle" (Structure for uploading the migration table)
**Table class**: `INTTAB` (internal / append structure, no database table)

| Field | Data element | Description |
|-------|--------------|-------------|
| `MELO` | `/US4G/DE_MELO_EXT` | External metering point |
| `ANSCHLNUTZ` | `/US4G/DE_CU_BP` | Connection user |
| `BEGINN` | `/US4G/DE_START_DATE` | Start date |

Used as the row type for the file upload (`TEXT_CONVERT_XLS_TO_SAP`) and for generating the downloadable XLSX template.

---

## Domains and Data Elements

### Domains

| Domain | Base type | Length | Fixed values | Description |
|--------|-----------|--------|--------------|-------------|
| `/HFQ/MOS_CTR_NO_CTR` | CHAR | 1 | `X` (set), ` ` (unset) | Flag: whether MSB contract creation should be suppressed |
| `/HFQ/MOS_CTR_PARA_NAME` | CHAR | 10 | `VORL_VK_MO`, `EIG_MSB_SA` | Names of customer-specific parameters for MOS billing |

### Data Elements

| Data element | Domain | Description | Screen label (long) |
|--------------|--------|-------------|---------------------|
| `/HFQ/MOS_CTR_NO_CTR` | `/HFQ/MOS_CTR_NO_CTR` | Flag: suppress MSB contract creation | "Unterdrückung Anl. MSB-Vertrag" |
| `/HFQ/MOS_CTR_PARA_NAME` | `/HFQ/MOS_CTR_PARA_NAME` | Name of customer-specific parameter | "Parametername" |
| `/HFQ/MOS_CTR_PARA_WERT` | `CHAR50` | Parameter value (50 chars) | "Parameterwert" |
| `/HFQ/MOS_CTR_ARCHIVIERT_VON` | `XUBNAME` | User who archived the record | "Archiviert von" |
| `/HFQ/MOS_CTR_ARCHIVIERT_AM` | `DATUM` | Date of archiving | "Archiviert am" |

---

## Other Objects

### Message Class: `/HFQ/MOS_CTR`

**File**: `src/#hfq#mos_ctr.msag.xml`
**Master language**: German (D)

All messages belong to the migration report. Variables are `&1`–`&4`.

| Nr | Type | Text |
|----|------|------|
| 200 | — | `*****Migrationsreport MOS*****` (header separator line) |
| 201 | E | Row `&1`: business partner `&2` not found — row skipped |
| 202 | E | Row `&1`: connection object `&2` not found — row skipped |
| 203 | W | Row `&1`: key already exists — row not processed |
| 204 | I | Row `&1`: MELO `&2`, BP `&3`, start `&4` loaded into migration table |
| 205 | E | No entry found/relevant in table `&1` for the selection |
| 206 | E/W | Row `&1`: error creating MOS contract for MELO `&2`, BP `&3`, start `&4` |
| 207 | E | Row `&1`: error triggering QUOTES process for MOS contract `&2` |
| 208 | I | Row `&1`: MOS contract `&2` created — QUOTES process doc `&3` |
| 209 | E | Error opening/accessing table `&1` |
| 210 | E | Please enter a file path |
| 211 | E | Row `&1`: MeLo `&2` not found |
| 212 | I | `&1` records moved to archive table `ZMOS_MIG_ARC_CTR` |
| 213 | — | No data available for archiving |
| 214 | W | Row `&1`: entry for MOS contract `&2`, MeLo `&3` is in status `&4` — skipped |
| 215 | — | Row `&4`: MOS contract `&1` `&2` was terminated on `&3` |
| 216 | — | Row `&1`, MeLo `&2`: neither new BP nor a LW — please check manually |
| 217 | — | Row `&1`: data for MeLo `&2`, contract `&3`, timeslice `&4` determined |
| 218 | — | Row `&1`: no contract or timeslice found for MeLo `&2` |
| 219 | — | `&1` records moved to `ZMOS_MIG_END_ARC` |
| 220 | E | Parameter `&1` not maintained in table `/HFQ/CTR_PARA` |

### Table Type: `/HFQ/T_MOS_CTR_PARA`

**File**: `src/#hfq#t_mos_ctr_para.ttyp.xml`
Standard table of rows typed as `/HFQ/CTR_PARA` (STRU), default table key, used as the type of the class attribute `GT_MOSB_PARAMS`.

### View Maintenance Objects (TOBJ)

| Object | Table | Category | Description |
|--------|-------|----------|-------------|
| `/HFQ/CTR_MIG_CTS` | `/HFQ/CTR_MIG_CT` | Customizing | SM30 view maintenance object for the migration staging table |
| `/HFQ/CTR_PARAS` | `/HFQ/CTR_PARA` | Application | SM30 view maintenance object for the parameter table |

### Classification Assignment (AVAS)

**File**: `src/005056ba9bfb1fe0b5b4da71441c327a.avas.xml`
Assigns `TABLE_LOGGING_REQUIREMENT = NOT_REQUIRED` to the table `/HFQ/CTR_MIG_AR`. This suppresses table change logging for the archive table.

### Namespace Object

**File**: `src/#hfq#.nspc.xml`
Defines the `/HFQ/` namespace registration used by all objects in this package.
