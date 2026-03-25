# Package: HFQ / COLLECT_EXCEPTIONS_TOEXL

**Namespace:** `/HFQ/` (Hochfrequenz)
**Master language:** German (D)
**abapGit folder logic:** PREFIX

## Executive Summary

This package collects and consolidates exception (Klärfall / Ausnahmecode) information from SAP APE process configurations and exports the result to an ALV grid or an Excel file (`.xlsx`). The entry point is transaction `/HFQ/COLLEC_EXCEPT`, which drives report `/HFQ/RP_COLL_EXCP_METHODS`. The report instantiates the class `/HFQ/CL_COLLECT_EXCEPTIONS`, which queries multiple `/APE/` tables to build a flat, denormalised result table that links each APE process step to every exception code that can be raised there — including steps inherited from template processes and embedded sub-processes.

The package description in `package.devc.xml` reads: *"Pacakge to generate Excel List with all Exceptions"* (sic — typo is in the source).

Key dependencies are APE (Application Process Engine) objects in the `/APE/` namespace and SAP case-management objects in the `EMMAC_` namespace. All those objects live outside this package.

---

## Classes

### `/HFQ/CL_COLLECT_EXCEPTIONS`

**File:** `src/#hfq#cl_collect_exceptions.clas.abap` / `.clas.xml`
**Description (XML):** "Class to collect and oder Exceptions"
**Instantiation:** PUBLIC FINAL CREATE PUBLIC

#### Public interface

| Visibility | Member | Kind | Type / Signature | Notes |
|---|---|---|---|---|
| public | `go_alv` | attribute | `REF TO cl_salv_table` | ALV table instance built by `set_salv` |
| public | `mt_collec_proc_Excp` | attribute | `/HFQ/T_COLLECTION_EXCEPTIONS` | Main result table (process + exception rows) |
| public | `mt_proc_steps` | attribute | `/HFQ/T_EXCPCOLL_PROCESS` | Process steps for directly selected processes |
| public | `mt_checkinfo` | attribute | `/HFQ/T_EXCPCOLL_CHECKS` | Check groups with exception codes |
| public | `mt_excpetion_cases` | attribute | `/HFQ/T_EXCPCOLL_CASES` | Exception codes with case categories |
| public | `mt_proc_templates` | attribute | `/HFQ/T_COLLEC_PROC_TEMP` | Processes that have a pattern/template process |
| public | `mt_proc_template_steps` | attribute | `/HFQ/T_EXCPCOLL_PROCESS` | Steps belonging to template processes |
| public | `lt_template_Range` | attribute | `ty_t_procID_range` | RANGE table of template process IDs |
| public | `mt_sub_Proc` | attribute | `/HFQ/T_EXCEPCOLL_SUBPROC` | Embedded sub-process relations |
| public | `mt_sub_Proc_Steps` | attribute | `/HFQ/T_EXCPCOLL_PROCESS` | Steps belonging to sub-processes |
| public | `lt_subProc_Range` | attribute | `ty_t_procID_range` | RANGE table of sub-process IDs |
| public | `mt_collec_subProc_Excp` | attribute | `/HFQ/T_COLLECTION_EXCEPTIONS` | Exception collection for sub-processes |
| public | `constructor` | method | — | Empty constructor |
| public | `main` | method | `IT_PROCESSID TYPE ty_t_procID_range`, `IV_VERSION TYPE /APE/DE_VERSION`, `IV_ONLY_NEWEST_VERSION TYPE abap_bool` | Orchestration method — see flow below |
| public | `create_excl` | method | — | Converts ALV to XLSX via `CL_SALV_TABLE->TO_XML` and downloads via `CL_GUI_FRONTEND_SERVICES` |
| public | `display_table` | method | — | Calls `go_alv->display()` with all toolbar functions enabled |

#### Protected interface

| Visibility | Member | Kind | Signature / Notes |
|---|---|---|---|
| protected | `mt_newest_Proc_Vers` | attribute | `/HFQ/T_EXCEPCOLL_NEWESTVERS` — newest version per process ID |
| protected | `mt_process_Version_Overview` | attribute | `/HFQ/T_EXCEPCOLL_NEWESTVERS` — all versions in scope (filtered by inputs) |
| protected | `set_exception_categories` | method | SELECTs `/APE/EXCPCODE` joined with `EMMAC_CCAT_HDRT` (language = 'D'); populates `mt_excpetion_cases` |
| protected | `set_checkinfo` | method | SELECTs `/APE/C_CHECKACTIONALLBUFFERED` joined with `/APE/CHECK` and `/APE/CHECKT`; populates `mt_CheckInfo`; only rows where `exception_code <> ''` |
| protected | `set_process_info` | method | `IT_PROCESSID TYPE ty_t_procID_range`, `IV_VERSION TYPE /APE/DE_VERSION`; `CHANGING CT_PROCSTEPS TYPE /HFQ/T_EXCPCOLL_PROCESS` — SELECTs `/APE/C_PROCESSSTEPBUFFERED` with joins to `/APE/PROCID`, `/APE/PROCIDT`, `/APE/PRSTPT`, `/APE/PROC`, `/APE/VERSION`; filters by check group membership |
| protected | `set_template_processes` | method | `IT_PROCESSID`, `IV_VERSION`; SELECTs `/APE/PROC` to find processes with a `pattern_proc_uuid`; populates `mt_proc_templates` |
| protected | `set_salv` | method | Creates `CL_SALV_TABLE` instance on `mt_collec_proc_Excp`; enables column optimisation and striped pattern |
| protected | `combine_tables` | method | Calls `combine_process_exception` for main steps and sub-process steps; calls `addTemplateSteps` if template steps exist; sorts result |
| protected | `addtemplatesteps` | method | Merges template process steps into `mt_collec_proc_Excp`, replacing template IDs with the consuming process's IDs and version; deduplicates by `seq_no` + `proc_uuid` |
| protected | `change_columnnames` | method | `IV_COLUMN_TO_RENAME TYPE lvc_fname`, optional short/medium/long text; wraps `CL_SALV_COLUMN` setters |
| protected | `disable_column` | method | `IV_COLUMNNAME TYPE lvc_fname`; sets column visibility to `abap_false` |
| protected | `addentry` | method | `IS_CHECKGROUP TYPE /HFQ/S_EXCPCOLL_CHECKS`, `IS_EXCEPTION TYPE /HFQ/S_EXCPCOLL_CASES`, `IS_PROCESSSTEP TYPE /HFQ/S_EXCP_COLLECTION_PROCESS`, `IV_PARENTPROC TYPE /APE/DE_PROC_ID`; `CHANGING IT_COLL_PROCSTEP_COLLEC TYPE /HFQ/T_COLLECTION_EXCEPTIONS`; appends one result row |

#### Private interface

| Visibility | Member | Signature / Notes |
|---|---|---|
| private | `filter_newest_version` | Loops `mt_newest_proc_vers`; deletes rows from `mt_collec_proc_Excp` whose `version_timestamp` does not match the newest timestamp for that process ID |
| private | `find_newest_version` | `IT_PROCESSID`; SELECTs `/APE/PROCID` + `/APE/PROC` + `/APE/VERSION`; keeps only one row per `proc_id` (newest by `valid_from` descending) in `mt_newest_proc_vers` |
| private | `set_sub_processes` | `IT_PROCESSID`; SELECTs `/APE/C_PROCESSSTEPBUFFERED` joined with `/APE/PROC` on trigger keys; filters `category = 'Embedded'`; populates `mt_sub_Proc` |
| private | `add_sub_processes` | `IT_PROCESSID`, `IV_ONLY_NEWEST_VERSION`; appends sub-process exception rows to `mt_collec_proc_excp`; annotates parent process rows with a `sub_proc` string listing embedded process IDs and versions |
| private | `combine_process_exception` | `IT_PROCSTEP TYPE /HFQ/T_EXCPCOLL_PROCESS`; `CHANGING IT_COLL_PROCSTEP_COLLEC TYPE /HFQ/T_COLLECTION_EXCEPTIONS`; three-way nested LOOP over process steps, check groups, and exception cases; calls `addentry` for each matching combination |
| private | `set_process_keys` | `IT_PROCESSID`, `IV_VERSION`, `IV_ONLY_NEWEST_VERSION`; builds `mt_process_version_overview`; iteratively expands it with template and sub-process UUIDs (WHILE loop until stable) |

#### `main` orchestration flow

1. `set_exception_categories` — load all Ausnahmecode/category mappings
2. `set_checkinfo` — load all check groups with their exception codes
3. `set_template_processes` — find which input processes reference a pattern process
4. `set_sub_processes` — find which process steps trigger embedded sub-processes
5. `find_newest_version` — determine the most recent version per process
6. `set_process_keys` — build the working set of process UUIDs (expands templates and sub-processes transitively)
7. `set_process_info` for template IDs — populate `mt_proc_template_steps`
8. `set_process_info` for sub-process IDs — populate `mt_sub_proc_steps`
9. `set_process_info` for input IDs — populate `mt_proc_steps`
10. `combine_tables` — merge everything into `mt_collec_proc_Excp`
11. `filter_newest_version` (conditional) — keep only newest version rows
12. `add_sub_processes` — annotate parent rows and append sub-process rows
13. `set_salv` — create ALV instance
14. `disable_column` — hide UUID and timestamp columns (`PROC_UUID`, `PROC_STEP_UUID`, `VERSION_TIMESTAMP`)
15. `change_columnnames` — apply German display labels to all visible columns

#### ALV column label mapping (German)

| Field name | Short | Medium | Long |
|---|---|---|---|
| `PROCESS_DESCRIPTION` | Proc.Bes. | Prozess Beschreibung | Prozess Beschreibung |
| `PROCESS_STEP_DESCRIPTION` | ProcSBesc. | Proc.Sch.Beschr. | Prozess-Schritt Beschreibung |
| `AUSNAHMECODE` | KFallCo | Klärfall Code | Klärfall Code |
| `KLAERFALLKATEGORIE` | KlFallKat | Klärfallkat | Klärfallkategorie |
| `CHECK_DESCRIPTION` | Prü.Bes. | Prüf. Beschreibung | Prüfungs Beschreibung |
| `VERSION` | Pr.Vers | Prozess Version | Prozess Version |
| `SEQ_NO` | Pr.SNr. | Prozess S.Nr. | Prozessschritt Nummer |
| `CHECK_RESULT` | Pr.Erg. | Prüfergebnis | Prüfergebnis |
| `KATEGORITEXT` | KlBesch. | Klärfallbeschreibung | Klärfallbeschreibung |
| `SUB_PROC` | SubProc. | Sub Prozess ID | Eingebetter Prozess ID |

---

## Reports

### `/HFQ/RP_COLL_EXCP_METHODS`

**Files:** `src/#hfq#rp_coll_excp_methods.prog.abap` / `.prog.xml`
**Transaction:** `/HFQ/COLLEC_EXCEPT` (screen 1000)
**Title (transaction):** "Collect Proc. and Exceptions to Excl"
**Program title (text pool):** "Programm /HFQ/RP_COLLect_EXCEPCTIONS"

Note: the internal `REPORT` statement names the program `/HFQ/RP_COLLECL_EXCEPCTIONS` (extra 'L'), while the object name is `/HFQ/RP_COLL_EXCP_METHODS`. *Inferred: this is a residual naming inconsistency from development.*

#### Selection screen

**Block 1 — Prozess (TEXT-001)**

| Element | Type | Label | Notes |
|---|---|---|---|
| `S_PROCID` | SELECT-OPTIONS (NO INTERVALS) | Prozess-ID | Range over `/APE/DE_PROC_ID` |

**Block 2 — Prozess Version (TEXT-002)**

| Element | Type | Label | Notes |
|---|---|---|---|
| `P_NVONLY` | RADIOBUTTON (default ON) | Neuste Prozessversionen | Group VER; triggers UC |
| `P_ALLVER` | RADIOBUTTON | Alle Prozessversionen | Group VER |
| `P_VERCH` | RADIOBUTTON | Prozessversion auswählen | Group VER |
| `P_VERS` | PARAMETER `/APE/DE_VERSION` | Prozessversion | MODIF ID A — visible and editable only when `P_VERCH` is selected |

**Block 3 — Output (TEXT-003)**

| Element | Type | Label | Default |
|---|---|---|---|
| `P_EXCL` | CHECKBOX | Excel-Export | unchecked |
| `P_DISP` | CHECKBOX | Tabelle anzeigen | checked (`abap_true`) |

#### `AT SELECTION-SCREEN OUTPUT`

Dynamically enables/disables `P_VERS` (MODIF ID A) based on which radio button is active. `P_VERS` is inactive when `P_NVONLY` or `P_ALLVER` is selected.

#### `START-OF-SELECTION`

1. If `P_VERCH` is not active, clears `P_VERS`.
2. If `P_VERCH` is active but `P_VERS` is initial, raises error message `E000` from message class `/HFQ/COLL_EXCEP_MESS` ("Bitte Prozessversion auswählen").
3. Instantiates `/HFQ/CL_COLLECT_EXCEPTIONS`.
4. Calls `main( it_processid = S_PROCID[] iv_version = P_VERS iv_only_newest_version = P_NVONLY )`.
5. If `P_EXCL` is checked: calls `create_excl( )`.
6. If `P_DISP` is checked: calls `display_table( )`.

---

## Tables / Data Definitions

All structures and table types defined in this package are internal tables (TABCLASS = INTTAB) — they have no database table counterpart.

### Structures

#### `/HFQ/S_COLLECTIONS_EXCEPTIONS`

**Description:** "collect all Exceptions with necessary information" — the main output row type.

| Field | Domain / Data element | Notes |
|---|---|---|
| `PROC_ID` | `/APE/DE_PROC_ID` | APE Process ID |
| `PROCESS_DESCRIPTION` | `/APE/DE_DESCRIPTION` | Process description text |
| `VERSION` | `/APE/DE_VERSION` | Process version |
| `SEQ_NO` | `/APE/DE_SEQUENCE_NO_CHAR` | Process step sequence number |
| `SUB_PROC` | `/HFQ/DE_EXCEPCOLL_SUBPROCESSES` | Concatenated list of embedded sub-process IDs and versions (STRING, length 300) |
| `PROC_STEP_ID` | `/APE/DE_PROC_STEP_ID` | Process step ID |
| `PROCESS_STEP_DESCRIPTION` | `/APE/DE_DESCRIPTION` | Process step description |
| `CHECK_GROUP` | `/APE/DE_CHECK_GROUP` | APE check group |
| `CHECK_ID` | `/APE/DE_CHECK_ID` | APE check ID |
| `CHECK_DESCRIPTION` | `/APE/DE_DESCRIPTION` | Check description text |
| `CHECK_RESULT` | `/APE/DE_CHECK_RESULT` | Check result value |
| `AUSNAHMECODE` | `/APE/DE_EXCEPTION_CODE` | Exception code (Ausnahmecode) |
| `KATEGORITEXT` | `EMMA_CCTXT` | Case category short text |
| `KLAERFALLKATEGORIE` | `/APE/DE_EXCEPTION_CODE_EXT` | External exception code / Klärfall category |
| `PROC_STEP_UUID` | `/APE/DE_CONFIG_DB_KEY` | Process step UUID (hidden in ALV) |
| `PROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | Process UUID (hidden in ALV) |
| `VERSION_TIMESTAMP` | `/APE/DE_FROM_TIMESTAMP` | Version valid-from timestamp (hidden in ALV) |

#### `/HFQ/S_EXCP_COLLECTION_PROCESS`

**Description:** "collect all Exceptions with necessary information" — intermediate type for process step data.

| Field | Domain / Data element | Notes |
|---|---|---|
| `PROC_ID` | `/APE/DE_PROC_ID` | |
| `PROCESS_DESCRIPTION` | `/APE/DE_DESCRIPTION` | |
| `CHECK_GROUP` | `/APE/DE_CHECK_GROUP` | |
| `PROC_STEP_ID` | `/APE/DE_PROC_STEP_ID` | |
| `VERSION` | `/APE/DE_VERSION` | |
| `PROCESS_STEP_DESCRIPTION` | `/APE/DE_DESCRIPTION` | |
| `PROC_STEP_NO` | `/APE/DE_SEQUENCE_NO_CHAR` | Sequence number of step within process |
| `PROC_STEP_UUID` | `/APE/DE_CONFIG_DB_KEY` | |
| `PROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | |
| `VERSION_TIMESTAMP` | `/APE/DE_FROM_TIMESTAMP` | |

#### `/HFQ/S_EXCPCOLL_CHECKS`

**Description:** "collect all Exceptions Chekcs with necessary information_te" (typo in source)

| Field | Domain / Data element | Notes |
|---|---|---|
| `CHECK_GROUP` | `/APE/DE_CHECK_GROUP` | |
| `CHECK_ID` | `/APE/DE_CHECK_ID` | |
| `CHECK_DESCRIPTION` | `/APE/DE_DESCRIPTION` | |
| `AUSNAHMECODE` | `/APE/DE_EXCEPTION_CODE` | |
| `AUSNAHMECODE_ID` | `/APE/DE_CONFIG_DB_KEY` | UUID of the exception code |
| `SEQ_NO` | `/APE/DE_SEQUENCE_NO_CHAR` | |
| `CHECK_RESULT` | `/APE/DE_CHECK_RESULT` | |

#### `/HFQ/S_EXCPCOLL_CASES`

**Description:** "collect all Exceptions with necessary information"

| Field | Domain / Data element | Notes |
|---|---|---|
| `AUSNAHMECODE_ID` | `/APE/DE_CONFIG_DB_KEY` | UUID linking to `/HFQ/S_EXCPCOLL_CHECKS` |
| `KLAERFALLKATEGORIE` | `/APE/DE_EXCEPTION_CODE_EXT` | |
| `KATEGORITEXT` | `EMMA_CCTXT` | Case category text |
| `FALLTEXT` | `EMMA_CASETXT_RAW` | Case long text |

#### `/HFQ/S_COLLEC_PROC_TEMP`

**Description:** "collect all processes with template processes"

| Field | Domain / Data element | Notes |
|---|---|---|
| `PROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | UUID of the consuming process |
| `PROC_ID` | `/APE/DE_PROC_ID` | ID of the consuming process |
| `TEMPLATE_UUID` | `/APE/DE_CONFIG_DB_KEY` | UUID of the template (pattern) process |
| `TEMPLATE_ID` | `/APE/DE_PROC_ID` | ID of the template process |
| `PROC_DESCRIPTION` | `/APE/DE_DESCRIPTION` | |
| `VERSION` | `/APE/DE_VERSION` | |
| `VERSION_TIMESTAMP` | `/APE/DE_FROM_TIMESTAMP` | |

#### `/HFQ/S_EXCEPCOLL_NEWPROCVERS`

**Description:** "newest version of all processes"

| Field | Domain / Data element | Notes |
|---|---|---|
| `PROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | |
| `PROC_ID` | `/APE/DE_PROC_ID` | |
| `VERSION` | `/APE/DE_VERSION` | |
| `VERSION_TIMESTAMP` | `/APE/DE_FROM_TIMESTAMP` | |

#### `/HFQ/S_EXCEPCOLL_SUBPROC`

**Description:** "Porcesses with Subprocesses" (typo in source)

| Field | Domain / Data element | Notes |
|---|---|---|
| `PROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | UUID of the parent process |
| `PROC_ID` | `/APE/DE_PROC_ID` | ID of the parent process |
| `SUBPROC_UUID` | `/APE/DE_CONFIG_DB_KEY` | UUID of the embedded sub-process |
| `SUBPROC_ID` | `/APE/DE_PROC_ID` | ID of the embedded sub-process |

### Table types

All table types use transparent access mode (`ACCESSMODE = T`) with default key (non-unique).

| Table type | Row structure | Description |
|---|---|---|
| `/HFQ/T_COLLECTION_EXCEPTIONS` | `/HFQ/S_COLLECTIONS_EXCEPTIONS` | Main result collection |
| `/HFQ/T_EXCPCOLL_PROCESS` | `/HFQ/S_EXCP_COLLECTION_PROCESS` | Process step intermediate table |
| `/HFQ/T_EXCPCOLL_CHECKS` | `/HFQ/S_EXCPCOLL_CHECKS` | Check/exception code pairs |
| `/HFQ/T_EXCPCOLL_CASES` | `/HFQ/S_EXCPCOLL_CASES` | Exception code/category pairs |
| `/HFQ/T_COLLEC_PROC_TEMP` | `/HFQ/S_COLLEC_PROC_TEMP` | Template process relations |
| `/HFQ/T_EXCEPCOLL_NEWESTVERS` | `/HFQ/S_EXCEPCOLL_NEWPROCVERS` | Newest version tracking |
| `/HFQ/T_EXCEPCOLL_SUBPROC` | `/HFQ/S_EXCEPCOLL_SUBPROC` | Sub-process relations |

---

## Other Objects

### Data element: `/HFQ/DE_EXCEPCOLL_SUBPROCESSES`

**Type:** STRG (string), length 300
**Description:** "Collect multiple subProcess IDs"

Used as the type of `SUB_PROC` in `/HFQ/S_COLLECTIONS_EXCEPTIONS`. At runtime the field is filled with a comma-separated string such as `SUBPROC_A (V. 1.0), SUBPROC_B (V. 2.0)` to list all embedded process IDs visible on a parent process row.

### Message class: `/HFQ/COLL_EXCEP_MESS`

**Description:** "Messages for Exception"

| Number | Text (DE) | Raised when |
|---|---|---|
| 000 | Bitte Prozessversion auswählen | User selects radio button "Prozessversion auswählen" but leaves `P_VERS` empty |

### Transaction: `/HFQ/COLLEC_EXCEPT`

**Program:** `/HFQ/RP_COLL_EXCP_METHODS`
**Screen:** 1000
**Title:** "Collect Proc. and Exceptions to Excl"

---

## Source files

| File | Object type | Object name |
|---|---|---|
| `.abapgit.xml` | abapGit metadata | — |
| `src/package.devc.xml` | Package | `COLLECT_EXCEPTIONS_TOEXL` |
| `src/#hfq#.nspc.xml` | Namespace | `/HFQ/` |
| `src/#hfq#cl_collect_exceptions.clas.abap` | Class (ABAP) | `/HFQ/CL_COLLECT_EXCEPTIONS` |
| `src/#hfq#cl_collect_exceptions.clas.xml` | Class (metadata) | `/HFQ/CL_COLLECT_EXCEPTIONS` |
| `src/#hfq#rp_coll_excp_methods.prog.abap` | Report | `/HFQ/RP_COLL_EXCP_METHODS` |
| `src/#hfq#rp_coll_excp_methods.prog.xml` | Report (metadata/text pool) | `/HFQ/RP_COLL_EXCP_METHODS` |
| `src/#hfq#collec_except.tran.xml` | Transaction | `/HFQ/COLLEC_EXCEPT` |
| `src/#hfq#coll_excep_mess.msag.xml` | Message class | `/HFQ/COLL_EXCEP_MESS` |
| `src/#hfq#de_excepcoll_subprocesses.dtel.xml` | Data element | `/HFQ/DE_EXCEPCOLL_SUBPROCESSES` |
| `src/#hfq#s_collections_exceptions.tabl.xml` | Structure | `/HFQ/S_COLLECTIONS_EXCEPTIONS` |
| `src/#hfq#s_excp_collection_process.tabl.xml` | Structure | `/HFQ/S_EXCP_COLLECTION_PROCESS` |
| `src/#hfq#s_excpcoll_checks.tabl.xml` | Structure | `/HFQ/S_EXCPCOLL_CHECKS` |
| `src/#hfq#s_excpcoll_cases.tabl.xml` | Structure | `/HFQ/S_EXCPCOLL_CASES` |
| `src/#hfq#s_collec_proc_temp.tabl.xml` | Structure | `/HFQ/S_COLLEC_PROC_TEMP` |
| `src/#hfq#s_excepcoll_newprocvers.tabl.xml` | Structure | `/HFQ/S_EXCEPCOLL_NEWPROCVERS` |
| `src/#hfq#s_excepcoll_subproc.tabl.xml` | Structure | `/HFQ/S_EXCEPCOLL_SUBPROC` |
| `src/#hfq#t_collection_exceptions.ttyp.xml` | Table type | `/HFQ/T_COLLECTION_EXCEPTIONS` |
| `src/#hfq#t_excpcoll_process.ttyp.xml` | Table type | `/HFQ/T_EXCPCOLL_PROCESS` |
| `src/#hfq#t_excpcoll_checks.ttyp.xml` | Table type | `/HFQ/T_EXCPCOLL_CHECKS` |
| `src/#hfq#t_excpcoll_cases.ttyp.xml` | Table type | `/HFQ/T_EXCPCOLL_CASES` |
| `src/#hfq#t_collec_proc_temp.ttyp.xml` | Table type | `/HFQ/T_COLLEC_PROC_TEMP` |
| `src/#hfq#t_excepcoll_newestvers.ttyp.xml` | Table type | `/HFQ/T_EXCEPCOLL_NEWESTVERS` |
| `src/#hfq#t_excepcoll_subproc.ttyp.xml` | Table type | `/HFQ/T_EXCEPCOLL_SUBPROC` |
