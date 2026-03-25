# Package: HFQ / MOS_CTR

**Description**: Hauptpaket - Entwicklungen MSB-Abrechnung (main package); sub-package `utils` is described as "Unterpaket - Entwicklungen MSB-Seite" (MSB-side developments)
**Original language**: German (D)
**Number of objects**: 26 (across both package levels: 1 class, 1 report, 2 function groups, 4 transparent tables/structures, 2 domains, 5 data elements, 1 table type, 2 maintenance views (TOBJ), 1 message class, 1 attribute-value set (AVAS), 1 namespace object, 2 package definitions)

---

## Executive Summary

HFQ/MOS_CTR is the **MSB (Messstellenbetreiber) billing contract** infrastructure package developed by Hochfrequenz. Its sole purpose is the **mass creation and lifecycle management of MOS (Messstellenbetrieb) contracts** in SAP IS-U, supporting a data-migration and ongoing-creation use case.

The package provides:

1. **A constants class** (`/HFQ/MOS_CTR_CONSTANTS`) that centralises all configuration values used across the MSB billing landscape — price classes, voltage levels, EDI partner addresses, IDoc operands, migration table names, and a class-constructor that loads the customer-specific parameter table into a class attribute at startup.

2. **A mass-creation report** (`/HFQ/RP_CREATE_MOS_CTR_MASS`) that manages the full migration cycle:
   - Upload of an XLSX file (MELO / business partner / start date) into the migration staging table
   - Automated creation of MOS contracts via the `/US4G/` contract API
   - Triggering of the QUOTES process per contract time-slice
   - Archival of completed records from the active migration table to the archive table
   - Download of a template XLSX for data input

3. **Two customising/maintenance function groups** (`/HFQ/CTR_MIG_CT`, `/HFQ/CTR_PARA`) that expose SM30-style view-maintenance screens for the migration and parameter tables respectively.

4. **Four database/structure objects** that back the above: the active migration table, the archive table, the customer-parameter table, and a flat upload structure.

5. **A message class** (`/HFQ/MOS_CTR`) with 21 numbered messages covering all error, warning, and success scenarios of the migration workflow.

External dependencies include the `/US4G/` framework (contract API `create_contract`, `create_position`, `trig_quotes_proc`, `delete_contract`, exception `cx_general`, constants interface `if_constants_billing`), standard IS-U functions (`ISU_DB_EUITRANS_EXT_SINGLE`, `ISU_DB_EUITRANS_INT_SINGLE`, `ISU_DB_ESERVPROV_SINGLE`), and standard SAP utilities (`BAL_*` application log, `TEXT_CONVERT_XLS_TO_SAP`, `GUI_DOWNLOAD`, `VIEW_MAINTENANCE_CALL`).

---

## Classes

### `/HFQ/MOS_CTR_CONSTANTS`

| Property | Value |
|---|---|
| File | `src/utils/#hfq#mos_ctr_constants.clas.abap` + `.xml` |
| Description (DE) | Kontanten MSB-Abrechnung (Constants for MSB billing) |
| Visibility | `PUBLIC FINAL CREATE PUBLIC` |
| Language | D |

**Purpose**: Single authoritative source for all hard-coded values and runtime-loaded parameters used in MSB contract creation.

**Class data (loaded at runtime)**:

| Attribute | Type | Description |
|---|---|---|
| `GT_MOSB_PARAMS` | `/HFQ/T_MOS_CTR_PARA` | Full content of `/HFQ/CTR_PARA` (loaded by `CLASS_CONSTRUCTOR`) |

**Constants — billing / division**:

| Constant | Value | Type |
|---|---|---|
| `GC_MOSB_SPARTE_STR_AUS` | `'10'` | `SPARTE` (Strom Ausspeiser) |
| `GC_MOSB_SPARTE_STR_EIN` | `'15'` | `SPARTE` (Strom Einspeiser) |

**Constants — operands** (all currently empty strings — *Inferred*: placeholders to be filled per project):

`GC_MOSB_OPERAND_AUS_HT`, `GC_MOSB_OPERAND_AUS_HT_ABZ`, `GC_MOSB_OPERAND_AUS_NT`, `GC_MOSB_OPERAND_AUS_NT_ABZ`, `GC_MOSB_OPERAND_EIN`

**Constants — price classification**:

| Constant | Value | Type |
|---|---|---|
| `GC_MOSB_PRICL_MME` | `'Z25'` | `/US4G/DE_PRICE_CLASS` |
| `GC_MOSB_PRICL_TRA` | `'Z26'` | `/US4G/DE_PRICE_CLASS` |
| `GC_MOSB_PRICL_INIT` | `'Z31'` | `/US4G/DE_PRICE_CLASS` |
| `GC_MOSB_ADD_PRICL_NS` | `'Z11'` | `/US4G/DE_PRICE_CLASS_ADD` |
| `GC_MOSB_ADD_PRICL_MS` | `'Z10'` | `/US4G/DE_PRICE_CLASS_ADD` |
| `GC_MOSB_ADD_PRICL_HS` | `'Z09'` | `/US4G/DE_PRICE_CLASS_ADD` |
| `GC_MOSB_ADD_PRICL_HOES` | `'Z08'` | `/US4G/DE_PRICE_CLASS_ADD` |

**Constants — voltage levels** (`SPEBENE`):

| Constant | Value |
|---|---|
| `GC_MOSB_SPANNEB_NS_IN` | `'IN'` |
| `GC_MOSB_SPANNEB_NS_M3` | `'M3'` |
| `GC_MOSB_SPANNEB_MS_IM` | `'IM'` |
| `GC_MOSB_SPANNEB_MS_AM` | `'AM'` |
| `GC_MOSB_SPANNEB_HS_HM` | `'HM'` |

**Constants — EDI/IDoc communication**:

| Constant | Value | Description |
|---|---|---|
| `GC_MOSB_REC_POR` | `'SAPHFQ'` | Receiver port |
| `GC_MOSB_REC_PRN` | `'HFQCLNT200'` | Receiver partner number |
| `GC_MOSB_REC_PRT` | `'LS'` | Receiver partner type |
| `GC_MOSB_SND_POR` | `'HFQ_100'` | Sender port |
| `GC_MOSB_SND_PRN` | `'LS_HFQ'` | Sender partner number |
| `GC_MOSB_SND_PRT` | `'SP'` | Sender partner type |

**Constants — service providers**:

| Constant | Value | Description |
|---|---|---|
| `GC_MOSB_DEF_LF` | `'LS_HFQ'` | Default Lieferant (supplier) |
| `GC_MOSB_DEF_MSB` | `'MS_HFQ'` | Default MSB |

**Constants — process / contract type**:

| Constant | Value | Description |
|---|---|---|
| `GC_SWITCHTYPE_MOS_QUOTES` | `'40'` | Switch type for MOS QUOTES process |
| `GC_PROCID_MOS_QUOTES` | `'MOSB0001'` | Process ID for MOS QUOTES |
| `GC_MOSB_VKTYP_AN` | `'MO'` | Contract account type |
| `GC_MOSB_STERKZ_DEF` | `'E0'` | Default tax determination indicator |
| `GC_MOS_SERVICE_INTCODE` | `''` | Service internal code (empty — *Inferred*: placeholder) |

**Constants — parameters**:

| Constant | Value | Description |
|---|---|---|
| `GC_MOSB_PARA_VORL_VK` | `'MOSB_VK_MO'` | Template contract account parameter key |
| `GC_MOSB_PARA_EIG_MSB` | `'EIG_MSB_SA'` | Parameter key for own MSB service provider |

**Constants — migration table names** (`TABNAME`):

| Constant | Value |
|---|---|
| `GC_MOSB_MIG_CREA_TAB` | `'/HFQ/CTR_MIG_CT'` |
| `GC_MOSB_MIG_ARC_TAB` | `'/HFQ/CTR_MIG_AR'` |
| `GC_MOSB_MIG_ENDARC_TAB` | `'/HFQ/MOSB_END_AR'` |
| `GC_MOSB_MIG_END_TAB` | `'/HFQ/MOSB_END_CT'` |
| `GC_MOSB_MIG_UPL_END_TAB` | `'/MOSB_MIG_UPL'` |
| `GC_MOSB_MIG_UPL_TAB` | `'/MOSB_END_UPL'` |

**Constants — application log**:

| Constant | Value |
|---|---|
| `GC_MOSB_LOG_OBJ` | `'/HFQ/'` |
| `GC_MOSB_MIG_SUBLOG_CTR` | `'/HFQ/MOS_CTR'` |
| `GC_MOSB_MIG_SUBLOG_UPL` | `'/HFQ/MOS_CTR'` |
| `GC_MOSB_MSG_ID_MOSB` | `'/HFQ/MOS_CTR'` |

**`CLASS_CONSTRUCTOR`**: Executes `SELECT * FROM /hfq/ctr_para INTO TABLE gt_mosb_params` so that all customer-individual parameters are available as a class attribute from the first use of the class.

---

## Reports

### `/HFQ/RP_CREATE_MOS_CTR_MASS`

| Property | Value |
|---|---|
| File | `src/utils/#hfq#rp_create_mos_ctr_mass.prog.abap` + `.xml` |
| Program type | Executable report (`SUBC=1`) |
| Description (DE) | Report /HFQ/RP_CREATE_MOS_CTR_MASS (Massenanlage MOS-Verträge / MSB-Abrechnung) |
| Unicode check | X |
| Fixed-point arithmetic | X |

**Purpose**: Mass creation of MOS contracts for MSB billing. Refactored from FORM-based to object-oriented (local classes). Manages the full lifecycle: data upload from XLSX, validation, contract creation via `/US4G/` API, QUOTES process triggering, and archival.

**Selection screen**:

| Parameter / Button | Text | Purpose |
|---|---|---|
| `P_UPL` (radio group R2) | Upload | Select upload mode |
| `P_EXE` (radio group R2) | Ausführung | Select contract-creation mode |
| `P_FILE` | Dateipfad | Path to XLSX upload file (visible in upload mode) |
| Button `W_BUTTO4` (`FC04`) | Download Vorlage | Download an XLSX template pre-filled with up to 5 existing migration records |
| Button `W_BUTTO1` (`FC01`) | Pflege Mig.-Tabelle | Open SM30 view for `/HFQ/CTR_MIG_CT` |
| Button `W_BUTTO2` (`FC02`) | Archiv. abgeschl. Vorgänge | Archive completed records and open archive view |
| `P_SEL` / `P_MASS` (radio group R1) | Einzelselektion / Massenselektion | In execute mode: single record or all open records |
| `P_MELO` | Melo (Ext. Bezeichnung) | Filter by MeLo (visible in single-selection mode) |
| `P_AN` | Anschlussnutzer | Filter by business partner (visible in single-selection mode) |

**Local class `lcl_logger`**:
A thin wrapper around SAP's `BAL_*` application log API. Created with object/sub-object, supports `add_message`, `display_log`, and `save_log` methods. Message type determines problem class automatically (E/A → 2, W → 3, S/I → 4).

**Local class `lcl_application`** — methods:

| Method | Description |
|---|---|
| `initialize` | Writes button labels, sets default radio-button selection |
| `build_screen` | Shows/hides screen groups (`BL1`/`BL3`/`BL4`) based on current radio selection |
| `on_user_command` | Dispatches `FC01`, `FC02`, `FC04`, `SEL` user commands |
| `execute` | Creates application log, branches to `upload_file` or `create_contracts`, saves and displays log |
| `create_log` | Creates `lcl_logger` instance with sub-object from constants |
| `view_migration_table` | Calls `VIEW_MAINTENANCE_CALL` for `/HFQ/CTR_MIG_CT` (action 'S') |
| `view_archive_table` | Calls `VIEW_MAINTENANCE_CALL` for `/HFQ/CTR_MIG_AR` (action 'S') |
| `download_template` | Builds field catalogue from upload structure, opens file-save dialog, downloads XLSX with up to 5 existing migration records |
| `archive_contracts` | Selects completed records (CTR and PDOC filled) from `/HFQ/CTR_MIG_CT`, moves them to `/HFQ/CTR_MIG_AR` with user/date stamp, deletes originals; commits atomically |
| `upload_file` | Reads XLSX via `TEXT_CONVERT_XLS_TO_SAP`; validates each row: MeLo existence via `ISU_DB_EUITRANS_EXT_SINGLE`, business partner existence via `BUT000`, duplicate check in `/HFQ/CTR_MIG_CT`; on success inserts record |
| `create_contracts` | Reads open migration records (filtered by selection mode / partner / MeLo); resolves own MSB from `/HFQ/CTR_PARA`; for each record: resolves MeLo to internal ID, resolves connection object via `EUIINSTLN`/`EANL`/`EVBS`, creates contract (`/us4g/if_ctr_api_main~create_contract`), creates position (`create_position`), filters EE01 MALOs, triggers QUOTES process (`trig_quotes_proc`), writes CTR ID and PDOC back to `/HFQ/CTR_MIG_CT` |
| `build_message` / `log_syst_message` | Message structure helpers for `lcl_logger` |

**Contract creation detail**:
- Uses `/US4G/CL_CTR_FACTORY=>GET_CTR_API( )` to obtain the contract actions object
- Create reason is set to `gc_ts_reason_service_change` from `/US4G/IF_CONSTANTS_BILLING`
- On position creation failure, the already-created contract is deleted before continuing
- EE01 service MALOs (Erzeugungsanlage) found on positions are logged as warnings

**Events used**: `INITIALIZATION`, `AT SELECTION-SCREEN OUTPUT`, `AT SELECTION-SCREEN`, `AT SELECTION-SCREEN ON VALUE-REQUEST FOR p_file` (F4 via `F4_FILENAME`), `START-OF-SELECTION`

---

## Function Groups

### `/HFQ/CTR_MIG_CT`

| Property | Value |
|---|---|
| Files | `src/utils/#hfq#ctr_mig_ct.fugr.*` |
| Message ID | SV |
| Description | SM30 view maintenance for migration creation table `/HFQ/CTR_MIG_CT` |

**Functions**:

| Function module | Description |
|---|---|
| `TABLEFRAME_/HFQ/CTR_MIG_CT` | Generated view-maintenance frame function (SM30 display). Imports: `VIEW_ACTION`, `VIEW_NAME`, `CORR_NUMBER`; Tables: `DBA_SELLIST`, `DPL_SELLIST`, `EXCL_CUA_FUNCT`, `X_HEADER`, `X_NAMTAB`; Exception: `MISSING_CORR_NUMBER` |
| `TABLEPROC_/HFQ/CTR_MIG_CT` | Generated view-maintenance processor (SM30 edit). Additional export `LAST_ACT_ENTRY`, `UCOMM`, `UPDATE_REQUIRED`; Tables add `CORR_KEYTAB`, `EXTRACT`, `TOTAL`; Exception adds `SAVING_CORRECTION_FAILED` |

**Screen 0001**: Table-control overview screen for `/HFQ/CTR_MIG_CT`. Displays columns: `ANSCHLNUTZ` (key, ALPHA conversion, 10 chars), `MELO` (key, rolling, 50 chars), `STARTDAT` (key, date), `MOSB_CTR` (contract ID, editable, 12 chars), `NO_CTR` (checkbox, suppress-contract flag). 3 fixed columns. Multiple-line selection.

---

### `/HFQ/CTR_PARA`

| Property | Value |
|---|---|
| Files | `src/utils/#hfq#ctr_para.fugr.*` |
| Message ID | SV |
| Description | SM30 view maintenance for parameter table `/HFQ/CTR_PARA` |

**Functions**:

| Function module | Description |
|---|---|
| `TABLEFRAME_/HFQ/CTR_PARA` | Generated view-maintenance frame function (identical structure to `CTR_MIG_CT` counterpart) |
| `TABLEPROC_/HFQ/CTR_PARA` | Generated view-maintenance processor |

**Screen 0001**: Table-control overview for `/HFQ/CTR_PARA`. Displays columns: `NAME` (key, dropdown, 10 chars) and `WERT` (value, editable, 50 chars). 1 fixed column.

---

## Tables / Data Definitions

### `/HFQ/CTR_MIG_CT` — Migration staging table (active)

| Property | Value |
|---|---|
| File | `src/utils/#hfq#ctr_mig_ct.tabl.xml` |
| Class | Transparent, client-dependent, customising (`CONTFLAG=C`) |
| Description (DE) | Migrationstabelle für anzulegende MOS-Verträge |
| Buffering | Not allowed |

| Field | Key | Data element | Description |
|---|---|---|---|
| `MANDT` | X | `MANDT` | Client |
| `ANSCHLNUTZ` | X | `/US4G/DE_CU_BP` | Connection user (business partner) |
| `MELO` | X | `/US4G/DE_MELO_EXT` | Metering location (external ID) |
| `STARTDAT` | X | `/US4G/DE_START_DATE` | Contract start date |
| `MOSB_CTR` | — | `/US4G/DE_CTR_ID` | Created MOS contract ID (written back after creation) |
| `MOSB_PDOC` | — | `/APE/DE_DOC_NO` | Process document number from QUOTES (written back after triggering) |
| `NO_CTR` | — | `/HFQ/MOS_CTR_NO_CTR` | Flag to suppress contract creation for this entry |

**Maintenance view**: `/HFQ/CTR_MIG_CT` (TOBJ, type `S`, category `CUST`)

---

### `/HFQ/CTR_MIG_AR` — Migration archive table

| Property | Value |
|---|---|
| File | `src/utils/#hfq#ctr_mig_ar.tabl.xml` |
| Class | Transparent, client-dependent, customising (`CONTFLAG=C`) |
| Description (DE) | Migrationstabelle für anzulegende MOS-Verträge (same text as staging) |
| Buffering | Not allowed |
| Table logging | NOT_REQUIRED (AVAS classification) |

All fields from `/HFQ/CTR_MIG_CT` are present, plus:

| Field | Data element | Description |
|---|---|---|
| `ARCHIV_VON` | `/HFQ/MOS_CTR_ARCHIVIERT_VON` | User who archived the record |
| `ARCHIV_AM` | `/HFQ/MOS_CTR_ARCHIVIERT_AM` | Date of archival |

---

### `/HFQ/CTR_PARA` — Customer-specific parameter table

| Property | Value |
|---|---|
| File | `src/utils/#hfq#ctr_para.tabl.xml` |
| Class | Transparent, client-dependent, application (`CONTFLAG=A`) |
| Description (DE) | Tabelle für kundenindividuelle Parameter |
| Buffering | Not allowed |

| Field | Key | Data element | Description |
|---|---|---|---|
| `MANDT` | X | `MANDT` | Client |
| `NAME` | X | `/HFQ/MOS_CTR_PARA_NAME` | Parameter name (fixed-value domain: `VORL_VK_MO`, `EIG_MSB_SA`) |
| `WERT` | — | `/HFQ/MOS_CTR_PARA_WERT` | Parameter value (CHAR50) |

**Maintenance view**: `/HFQ/CTR_PARA` (TOBJ, type `S`, category `APPL`)

**Known parameter names** (from domain fixed values):
- `VORL_VK_MO` — Template contract account for MO-type (*Inferred*: template VK identifier)
- `EIG_MSB_SA` — Own MSB service provider SA (used in `create_contracts` to look up `mo_sp`)

---

### `/HFQ/MOS_CTR_MIG_UPL` — Upload structure (internal table type / flat structure)

| Property | Value |
|---|---|
| File | `src/utils/#hfq#mos_ctr_mig_upl.tabl.xml` |
| Class | Internal table (`INTTAB`) — used as flat structure for XLSX upload/download |
| Description (DE) | Struktur zum Upload der Migrationstabelle |

| Field | Data element | Description |
|---|---|---|
| `MELO` | `/US4G/DE_MELO_EXT` | Metering location (external) |
| `ANSCHLNUTZ` | `/US4G/DE_CU_BP` | Business partner (connection user) |
| `BEGINN` | `/US4G/DE_START_DATE` | Start date |

---

## Domains and Data Elements

### Domains

| Domain | Type | Length | Fixed values | Description (DE) |
|---|---|---|---|---|
| `/HFQ/MOS_CTR_NO_CTR` | CHAR | 1 | `X` / ` ` | Prüfung, ob Anlegen MSB-Vertrag unterdrückt werden soll |
| `/HFQ/MOS_CTR_PARA_NAME` | CHAR | 10 | `VORL_VK_MO`, `EIG_MSB_SA` | Bez. Kundenindividuelle Parameter für MOS-Billing |

### Data Elements

| Data element | Domain | Description (DE) | Screen texts |
|---|---|---|---|
| `/HFQ/MOS_CTR_NO_CTR` | `/HFQ/MOS_CTR_NO_CTR` | Prüfung, ob Anlage MSB-Vertrag unterdrückt werden | S: "No CTR", M: "Kein Vertrag", L: "Unterdrückung Anl. MSB-Vertrag" |
| `/HFQ/MOS_CTR_PARA_NAME` | `/HFQ/MOS_CTR_PARA_NAME` | Name des kundenindividuellen Parameters | S: "Name", M: "ParaName", L: "Parametername" |
| `/HFQ/MOS_CTR_PARA_WERT` | `CHAR50` | Parameterwert | S: "Wert", M: "Para-Wert", L: "Parameterwert" |
| `/HFQ/MOS_CTR_ARCHIVIERT_VON` | `XUBNAME` | Archiviert von | S: "Arch. von", M/L: "Archiviert von" |
| `/HFQ/MOS_CTR_ARCHIVIERT_AM` | `DATUM` | Archiviert am | S: "Arch. am", M/L: "Archiviert am" |

*Note*: The labels in the XML for `ARCHIVIERT_VON` and `ARCHIVIERT_AM` contain a typo "Archviert" (missing 'i'); the correct German spelling is "Archiviert". The `ARCHIVIERT_AM` domain is the standard `DATUM` type.

---

## Other Objects

### `/HFQ/T_MOS_CTR_PARA` — Table type

| Property | Value |
|---|---|
| File | `src/utils/#hfq#t_mos_ctr_para.ttyp.xml` |
| Row type | `/HFQ/CTR_PARA` (structure) |
| Access mode | Table (T) |
| Key definition | Default (D), no unique key (N) |

Used as the type of `GT_MOSB_PARAMS` in `/HFQ/MOS_CTR_CONSTANTS`.

---

### `/HFQ/MOS_CTR` — Message class

| Property | Value |
|---|---|
| File | `src/utils/#hfq#mos_ctr.msag.xml` |
| Master language | D |

| Nr | Type | Text (DE) |
|---|---|---|
| 200 | I | `*************Migrationsreport MOS*************************` |
| 201 | E | Zeile &1 GP &2 nicht vorhanden. Zeile wird nicht übernommen |
| 202 | E | Zeile &1 Anschlussobj. &2 nicht vorhanden. Zeile wird nicht übernommen |
| 203 | W | Zeile &1 Schlüssel schon vorhanden. Zeile wird nicht berücksichtigt |
| 204 | I | Zeile &1 Schlüssel MELO &2 GP&3 Beginn &4 in ZMOS_MIG_CRE_CTR übernommen |
| 205 | E | Zur Selektion kein Eintrag in Tabelle &1 gefunden/relevant |
| 206 | E | Zeile &1 Fehler beim Anlegen des MOS-Vertrags MELO &2 GP &3 Beginn: &4 |
| 207 | E | Zeile &1 Fehler beim Auslösen des QUOTES-Prozess zu MOS-Vertrag &2 |
| 208 | I | Zeile &1 MOS-Vertrag &2 Angelegt - QUOTES-Prozess: PDOc &3 |
| 209 | E | Fehler beim öffnen der Tabelle &1 |
| 210 | E | Bitte Dateipfad eingeben |
| 211 | E | Zeile &1 MeLo &2 nicht vorhanden |
| 212 | I | &1 Datensätze in Tabelle ZMOS_MIG_ARC_CTR überführt |
| 213 | W | Keine Daten zur Archivierung vorhanden |
| 214 | W | Eintrag &1 zur MeLo &2 befindet sich in Status &3 und wird übersprungen |
| 215 | I | Zeile &4 MOS Vertrag &1 &2 wurde zum &3 beendet |
| 216 | W | Zeile &1, MeLo &2: weder neuer GP, noch ein LW. Bitte manuell prüfen |
| 217 | I | Zeile &1: Die Daten für MeLo &2, Vertrag &3, Zeitsch. &4 wurden ermittelt |
| 218 | W | Zeile &1: Zur MeLo &2 wurde kein Vertrag oder keine Zeitscheibe gefunden |
| 219 | I | &1 Datensätze in Tabelle ZMOS_MIG_END_ARC überführt |
| 220 | E | Parameter &1 in Tabelle /HFQ/CTR_PARA nicht gepflegt |

*Note*: Messages 212, 215, 219 reference table names `ZMOS_MIG_ARC_CTR` and `ZMOS_MIG_END_ARC` — these appear to be legacy names from before the namespace migration to `/HFQ/`. The actual tables used in code are `/HFQ/CTR_MIG_AR` and related. Messages 215–219 are defined but not referenced in the report code in this package (*Inferred*: used elsewhere in the broader MSB billing suite).

---

### Maintenance View Objects (TOBJ)

| Object | Table | Category | Description |
|---|---|---|---|
| `/HFQ/CTR_MIG_CT` | `/HFQ/CTR_MIG_CT` | CUST | SM30 view for migration staging table |
| `/HFQ/CTR_PARA` | `/HFQ/CTR_PARA` | APPL | SM30 view for parameter table |

Both are type `S` (Simple), client-dependent, not directly transportable (`IMPORTABLE=3`).

---

### AVAS (Attribute-Value Assignment)

| GUID | Attribute | Object | Value |
|---|---|---|---|
| `005056BA9BFB1FE0B5B4DA71441C327A` | `TABLE_LOGGING_REQUIREMENT` | TABL `/HFQ/CTR_MIG_AR` | `NOT_REQUIRED` |

Classifies the archive table as not requiring table change logging.
