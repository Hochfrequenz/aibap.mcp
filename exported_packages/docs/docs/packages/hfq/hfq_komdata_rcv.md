# Package: HFQ / KOMDATA_RCV

**Package description (DE):** Empfang PARTIN
**Namespace:** `/HFQ/`
**Master language:** German (D)
**abapGit folder logic:** PREFIX

## Executive Summary

This sub-package implements the inbound processing pipeline for EDIFACT PARTIN messages in the German energy market (BDEW data exchange). PARTIN carries master data about service providers (Serviceanbieter, SA): names, addresses, bank accounts, tax identifiers, contact persons per process type, availability times, balance group assignments (Bilanzkreis), and court/commercial-register entries.

The package covers three concerns:

1. **IDoc parsing and BMID routing** — `CL_PARS_PARTIN_01` and `CL_MESSAGE_PARTIN_IN_01` receive the raw EDIFACT IDoc, set the correct message type, resolve the associated service provider by external identifier, and dispatch to the correct business message ID (`/HFQ/PARTI` and its variants).
2. **Data persistence** — `CL_PARTIN_RCV_SAVE` extracts all semantic fields from the process-step container and saves them as a versioned PARTIN data object via `CL_PARTIN` / `CL_PARTIN_DB`. Deactivation messages (document status `11`) trigger time-slice delimitation instead of a normal save.
3. **Change detection and update notification (BAdI framework)** — Enhancement spot `/HFQ/ES_KOMDATA_RCV` exposes two BAdIs (`/HFQ/BADI_KOMDATA_RCV_CHK` and `/HFQ/BADI_KOMDATA_RCV_UPDATE`). The default implementations (`CL_BADI_KOMDATA_RCV_CHK`, `CL_BADI_KOMDATA_RCV_UPD`) compare each data category of the incoming message against the previously stored version and return `EQUAL` / `NOT_EQUAL` / `ERROR`. The update BAdI writes a process-log entry for every changed category but performs no customer-specific write-back (returns `NOT_IMPLEMENTED`). A static facade class `KOMDATA_RCV_CHECK_METHODS` wraps the BAdI calls so process-step handlers can invoke individual checks without dealing with the BAdI machinery directly.

---

## Classes

### `/HFQ/CL_PARS_PARTIN_01`

**Description:** Parsing class for PARTIN (`Parsing Class for PARTIN`)
**Superclass:** `/IDXGL/CL_PARS_IDOCMAP`
**Visibility:** PUBLIC

Handles the very first step of inbound IDoc processing.

**Methods:**

| Method | Visibility | Description |
|---|---|---|
| `CONSTRUCTOR` | PUBLIC | Initialises the parser. Sets the message type to `/HFQ/PARTIN` constant on inbound direction; sets the IDoc type to `/IDXGL/PARTIN_02` on outbound if not yet filled. Configures segment split markers (`UNS_01`, `UNT_01`, `UNZ_01`) for inbound. |
| `DET_INBOUND_BASICPROC` | PROTECTED (redefinition) | Hard-codes the basic process to `CO_BASIC_PROC_PARTIN_IN` from `/HFQ/IF_PARTIN_CONSTANTS`. |
| `GET_PROCESS_PARAMETERS` | PROTECTED (redefinition) | Resolves the associated service provider from the `MS` party qualifier in the process data. If the service provider cannot be found by external ID and code list (`ISU_DATEX_IDENT_SP_BY_CODELIST`), a BPEM clarification case is created and an IDE error is raised to halt processing. On success delegates to `super->get_process_parameters`. |
| `CREATE_BPEM_CASE` | PRIVATE | Creates a BPEM clarification case (category `IDX5`, priority 2) anchored to the split IDoc number. Used when the sender service provider is unknown. |

---

### `/HFQ/CL_MESSAGE_PARTIN_IN_01`

**Description:** PARTIN class for message inbound (`PARTIN-Klasse für den Nachrichteneingang`)
**Superclass:** `/IDXGL/CL_MESSAGE`
**Visibility:** PUBLIC

Handles BMID determination and response/reversal reference linking.

**Methods:**

| Method | Visibility | Description |
|---|---|---|
| `DETERMINE_BMID` | PUBLIC (redefinition) | Maps all incoming PARTIN BMIDs to the canonical `/HFQ/PARTI` value. Exceptions: API messages stay as `GC_BMID_API`. Recognised BMIDs: `/HFQ/PARTI`, `/HFQ/PAR_D`, `/HFQ/PAR_M`, `/HFQ/PAR_S`, `/HFQ/PAR_E`, `/HFQ/PAR_O`, `/HFQ/PAR_U`, `/HFQ/PAR_V`. After BMID mapping, calls `HANDLE_REFERENCE_RESP_REV`. |
| `HANDLE_REFERENCE_RESP_REV` | PROTECTED | For response messages, retrieves the preceding outbound process step by the reference transaction number and copies the process reference and original message data into the current step, filling the response scenario ID `SCENARIO_ID_RESPONSE`. |

---

### `/HFQ/CL_PARTIN_RCV_SAVE`

**Description:** Class for saving the received PARTIN (`Klasse zum Speichern der eingegangenen PARTIN`)
**Superclass:** `/IDXGC/CL_PROCESS_STEP_DATA`
**Visibility:** PUBLIC

Executed as a process step action to persist inbound PARTIN data.

**Method:**

| Method | Visibility | Description |
|---|---|---|
| `PROCESS` | PROTECTED (redefinition) | Full save logic. Reads `AGK` version reference from the process container. For document status `11` (deactivation / "Inaktivkennzeichen"), calls `CL_PARTIN_DB=>DELIMIT_TIMESLICES` and returns without further processing. Otherwise: extracts header fields (service ID, version, validity date, EDI e-mail, download link, URL, fax, tax ID/number, court, commercial register number, own contact/address), collects contacts per `party_func_qual`, availability times (`/HFQ/AVAIL`), balance group data (`/HFQ/BIKREIS`), and bank data (`/HFQ/BANK`). Constructs a `CL_PARTIN` instance via `GET_INSTANCE_FROM_DATA` and calls `SAVE_DATA( iv_save_as_draft = false )`. |

**Key behaviour notes:**
- UTC timezone conversion is present in the code but commented out.
- Process step `0010` is read directly to get the document status because it is not automatically propagated to later steps.
- `party_func_qual` of the sender NAD segment is determined at runtime by `CL_PARTIN_HELPER=>GET_PARTY_FUNC_QUAL`.

---

### `/HFQ/CL_BADI_KOMDATA_RCV_CHK`

**Description:** Default implementation of BAdI `/HFQ/BADI_KOMDATA_RCV_CHK` (`Default-Impl. BAdI /HFQ/CL_BADI_KOMDATA_RCV_CHK`)
**Interfaces:** `IF_BADI_INTERFACE`, `/HFQ/IF_BADI_KOMDATA_RCV_CHK`
**Visibility:** PUBLIC

Default implementation of the check BAdI. Each method receives the process step key plus generic data references (`CR_DATA` → `/IDXGC/IF_PROCESS_DATA_EXTERN`, `CR_DATA_LOG` → `/IDXGC/IF_PROCESS_LOG`) and appends a result code to `CT_CHECK_RESULT`.

**Common check logic pattern:**
1. Dereference `CR_DATA` / `CR_DATA_LOG` via field symbols.
2. Call `get_process_step_data` on the data reference; on error, log and append `gc_cr_error`.
3. For checks that compare against a previously stored PARTIN version: call `CL_PARTIN_HELPER=>DETERMINE_VERSION`; if version is initial or all-zeros, return `EQUAL` (first-receive, no previous data to compare).
4. Instantiate `CL_PARTIN` with the previous version number.
5. Compare fields; on mismatch append `gc_cr_not_equal` and return; otherwise append `gc_cr_equal`.

**Methods (all implement `/HFQ/IF_BADI_KOMDATA_RCV_CHK~`):**

| Method | Data compared | Source |
|---|---|---|
| `CHECK_NAME_SA` | Company name (`NAME1`) | BP master (`BUT000`) |
| `CHECK_ADR_SA` | Street, house number, postal code, city, country | `ADRC` joined via `BUT020` / `ESERVPROVP`; validity date from `AGK` reference |
| `CHECK_BANK_SA` | IBAN + account holder (`BUT0BK`), bank name + BIC (`BNKA`) | `CL_PARTIN_HELPER=>GET_BUT_BANK_ACCS_FOR_SP` |
| `CHECK_BIKREIS_SA` | Balance group / settlement unit assignments | `CL_PARTIN=>GET_BIKREIS` (stored version) |
| `CHECK_COURT` | Court name + commercial register number (FTX qualifier `Z15`, sub-numbers 1 and 2) | `CL_PARTIN=>GET_HEADER` |
| `CHECK_TAX_ID_SA` | Tax ID (Steuer-ID) | *Inferred: stored PARTIN header.* |
| `CHECK_TAX_ID_NUMBER` | VAT ID (Umsatzsteuer-ID) | *Inferred: stored PARTIN header.* |
| `CHECK_TIMES_SA` | Availability times | *Inferred: stored PARTIN availability data.* |
| `CHECK_MAIL_DOWN_SA` | Download certificate e-mail | *Inferred: FTX qualifier Z11.* |
| `CHECK_MAIL_COM_SA` | Communication e-mail | *Inferred: FTX qualifier Z12.* |
| `CHECK_FAX_COM_SA` | Fax number | *Inferred: stored PARTIN header.* |
| `CHECK_INTERNET_COM_SA` | Website URL | *Inferred: FTX qualifier Z13.* |
| `CHECK_PART_Z10` | Contact person — transmission path (Übertragungsweg) | *Inferred: NAD qualifier Z10.* |
| `CHECK_PART_Z11` | Contact person — framework contracts (Rahmenverträge) | *Inferred: NAD qualifier Z11.* |
| `CHECK_PART_Z12` | Contact person — cancellation processes (Kündigungsprozesse) | *Inferred: NAD qualifier Z12.* |
| `CHECK_PART_Z13` | Contact person — switching processes (Wechselprozesse) | *Inferred: NAD qualifier Z13.* |
| `CHECK_PART_Z14` | Contact person — master data processes (Stammdatenprozesse) | *Inferred: NAD qualifier Z14.* |
| `CHECK_PART_Z16` | Contact person — feed-in processes (Einspeiseprozesse) | *Inferred: NAD qualifier Z16.* |
| `CHECK_PART_Z17` | Contact person — billing processes (Abrechnungsprozesse) | *Inferred: NAD qualifier Z17.* |
| `CHECK_PART_Z18` | Contact person — MMMA processes | *Inferred: NAD qualifier Z18.* |
| `CHECK_PART_Z19` | Contact person — movement data (Bewegungsdaten) | *Inferred: NAD qualifier Z19.* |
| `CHECK_PART_Z20` | Contact person — lock/unlock processes (Sperr-/Entsperrprozesse) | *Inferred: NAD qualifier Z20.* |
| `CHECK_PART_Z21` | Contact person — balancing processes (Bilanzierungsprozesse) | *Inferred: NAD qualifier Z21.* |
| `CHECK_PART_Z33` | Contact person — technical grid connection (tech. Netzanschluss) | *Inferred: NAD qualifier Z33.* |

---

### `/HFQ/CL_BADI_KOMDATA_RCV_UPD`

**Description:** Default implementation of BAdI `/HFQ/BADI_KOMDATA_RCV_UPDATE` (`Default-Impl. BAdI /HFQ/CL_BADI_KOMDATA_RCV_CHK`)
**Interfaces:** `IF_BADI_INTERFACE`, `/HFQ/IF_BADI_KOMDATA_RCV_UPD`
**Visibility:** PUBLIC

Default implementation of the update BAdI. Each method's purpose is to react to a detected change in a specific data category. In this default implementation all methods follow the same pattern: write a process-log message from message class `/HFQ/MSG_KOMDATA`, save the log, then append `gc_cr_not_implemented` — indicating that no customer-specific write-back has been implemented.

**Message numbers per method** (all from `/HFQ/MSG_KOMDATA`):

| Method | Message No. | Variable `MSGV1` |
|---|---|---|
| `UPDATE_ADR_SA` | 100 | associated service provider |
| `UPDATE_BANK_SA` | 102 | associated service provider |
| `UPDATE_BIKREIS_SA` | 116 | associated service provider |
| `UPDATE_COURT` | 098 | associated service provider |
| `UPDATE_FAX_COM_SA` | *Inferred — follows same pattern* | associated service provider |
| `UPDATE_INTERNET_COM_SA` | *Inferred* | associated service provider |
| `UPDATE_MAIL_COM_SA` | *Inferred* | associated service provider |
| `UPDATE_MAIL_DOWN_SA` | *Inferred* | associated service provider |
| `UPDATE_NAME_SA` | *Inferred* | associated service provider |
| `UPDATE_TAX_ID_SA` | *Inferred* | associated service provider |
| `UPDATE_TAX_ID_NUMBER` | *Inferred* | associated service provider |
| `UPDATE_TIMES_SA` | *Inferred* | associated service provider |
| `UPDATE_PART_Z10–Z21, Z33` | *Inferred* | associated service provider |

---

### `/HFQ/KOMDATA_RCV_CHECK_METHODS`

**Description:** Check methods for PARTIN reception (`Prüfmethoden für Empfang PARTIN`)
**Visibility:** PUBLIC
**No superclass / no interface implementation** — pure static facade.

This class is a static (class-method-only) wrapper that bundles both the `CHECK_*` and `UPDATE_*` operations. Each method has the same signature as the corresponding BAdI methods, but uses `EXPORTING ET_CHECK_RESULT` (not `CHANGING CT_CHECK_RESULT`), indicating it is called from process-step handler function modules rather than directly as a BAdI call.

Additionally exposes `CHECK_DOCUMENT_STATUS` — a method not present in either BAdI interface — which checks whether the document status field has changed.

**Full method list:**

CHECK methods: `CHECK_NAME_SA`, `CHECK_ADR_SA`, `CHECK_DOCUMENT_STATUS`, `CHECK_BANK_SA`, `CHECK_MAIL_DOWN_SA`, `CHECK_MAIL_COM_SA`, `CHECK_COURT`, `CHECK_TAX_ID_SA`, `CHECK_TIMES_SA`, `CHECK_PART_Z10`, `CHECK_PART_Z11`, `CHECK_PART_Z12`, `CHECK_PART_Z13`, `CHECK_PART_Z14`, `CHECK_PART_Z16`, `CHECK_PART_Z17`, `CHECK_PART_Z18`, `CHECK_PART_Z19`, `CHECK_PART_Z20`, `CHECK_PART_Z21`, `CHECK_PART_Z33`, `CHECK_FAX_COM_SA`, `CHECK_INTERNET_COM_SA`, `CHECK_TAX_ID_NUMBER`, `CHECK_BIKREIS_SA`

UPDATE methods: `UPDATE_NAME_SA`, `UPDATE_ADR_SA`, `UPDATE_BANK_SA`, `UPDATE_MAIL_DOWN_SA`, `UPDATE_MAIL_COM_SA`, `UPDATE_COURT`, `UPDATE_TAX_ID_SA`, `UPDATE_TIMES_SA`, `UPDATE_PART_Z10`, `UPDATE_PART_Z11`, `UPDATE_PART_Z12`, `UPDATE_PART_Z13`, `UPDATE_PART_Z14`, `UPDATE_PART_Z16`, `UPDATE_PART_Z17`, `UPDATE_PART_Z18`, `UPDATE_PART_Z19`, `UPDATE_PART_Z20`, `UPDATE_PART_Z21`, `UPDATE_PART_Z33`, `UPDATE_FAX_COM_SA`, `UPDATE_INTERNET_COM_SA`, `UPDATE_TAX_ID_NUMBER`, `UPDATE_BIKREIS_SA`

---

## Interfaces

### `/HFQ/IF_BADI_KOMDATA_RCV_CHK`

**Description:** Interface for BAdI `/HFQ/BADI_KOMDATA_RCV_CHK` (`Interface für /HFQ/BADI_KOMDATA_RCV_CHK`)
**Also implements:** `IF_BADI_INTERFACE`
**Unicode-enabled:** yes

Defines the contract for validating each data category of a received PARTIN message. All 23 methods share the same signature:

```abap
importing  IS_PROCESS_STEP_KEY type /IDXGC/S_PROC_STEP_KEY
changing   CT_CHECK_RESULT     type /IDXGC/T_CHECK_RESULT
           CR_DATA             type ref to DATA
           CR_DATA_LOG         type ref to DATA
```

Methods: `CHECK_NAME_SA`, `CHECK_ADR_SA`, `CHECK_BANK_SA`, `CHECK_MAIL_DOWN_SA`, `CHECK_MAIL_COM_SA`, `CHECK_COURT`, `CHECK_TAX_ID_SA`, `CHECK_TIMES_SA`, `CHECK_PART_Z10`, `CHECK_PART_Z11`, `CHECK_PART_Z12`, `CHECK_PART_Z13`, `CHECK_PART_Z14`, `CHECK_PART_Z16`, `CHECK_PART_Z17`, `CHECK_PART_Z18`, `CHECK_PART_Z19`, `CHECK_PART_Z20`, `CHECK_PART_Z21`, `CHECK_FAX_COM_SA`, `CHECK_INTERNET_COM_SA`, `CHECK_PART_Z33`, `CHECK_TAX_ID_NUMBER`, `CHECK_BIKREIS_SA`

---

### `/HFQ/IF_BADI_KOMDATA_RCV_UPD`

**Description:** Interface for BAdI `/HFQ/BADI_KOMDATA_RCV_UPDATE` (note: XML description still reads "CHK" — appears to be a copy-paste artefact in the metadata)
**Also implements:** `IF_BADI_INTERFACE`
**Unicode-enabled:** yes

Defines the contract for reacting to detected changes in each data category. All 23 methods share the same signature as `IF_BADI_KOMDATA_RCV_CHK`.

Methods: `UPDATE_ADR_SA`, `UPDATE_BANK_SA`, `UPDATE_FAX_COM_SA`, `UPDATE_INTERNET_COM_SA`, `UPDATE_MAIL_COM_SA`, `UPDATE_COURT`, `UPDATE_MAIL_DOWN_SA`, `UPDATE_NAME_SA`, `UPDATE_PART_Z10`, `UPDATE_PART_Z11`, `UPDATE_PART_Z12`, `UPDATE_PART_Z13`, `UPDATE_PART_Z14`, `UPDATE_PART_Z16`, `UPDATE_PART_Z17`, `UPDATE_PART_Z18`, `UPDATE_PART_Z19`, `UPDATE_PART_Z20`, `UPDATE_PART_Z21`, `UPDATE_TAX_ID_SA`, `UPDATE_TIMES_SA`, `UPDATE_PART_Z33`, `UPDATE_TAX_ID_NUMBER`, `UPDATE_BIKREIS_SA`

---

## Other Objects

### Enhancement Spot `/HFQ/ES_KOMDATA_RCV`

**Type:** ENHS (BAdI definition spot)
**Short text (DE):** Erweiterungsspot für Empfang PARTIN

Contains two BAdI definitions:

| BAdI name | Interface | Default class | Fallback class | Description |
|---|---|---|---|---|
| `/HFQ/BADI_KOMDATA_RCV_CHK` | `/HFQ/IF_BADI_KOMDATA_RCV_CHK` | `/HFQ/CL_BADI_KOMDATA_RCV_CHK` | yes | BAdI for checking received PARTIN data |
| `/HFQ/BADI_KOMDATA_RCV_UPDATE` | `/HFQ/IF_BADI_KOMDATA_RCV_UPD` | `/HFQ/CL_BADI_KOMDATA_RCV_UPD` | yes | BAdI for updates of changed data |

Both BAdIs use `CONTEXT_MODE = N` (no filter context).

---

### IDoc Type `/HFQ/PARTIN_01`

**Type:** IDOC

Custom IDoc type based on the standard IDEXDE PARTIN structure. Defines 21 segment entries organised in 5 hierarchy levels:

| Level | Key segments | Description |
|---|---|---|
| 1 | `E1_UNA_01`, `E1_UNB_01`, `E1_UNZ_01` | EDIFACT interchange envelope |
| 2 | `E1_UNH_01` | Message header (child of UNB) |
| 3 | `E1_BGM_02`, `E1_DTM_01`, `E1_RFF_01`, `E1_NAD_03`, `E1_UNS_01`, `E1_NAD_04`, `E1_UNT_01` | Message body segments |
| 4 | `E1_DTM_02` (under RFF), `E1_CTA_03`, `E1_FII_01`, `E1_FTX_01`, `E1_RFF_02`, `E1_CCI_03`, `E1_CTA_04` | Reference dates, contacts, bank data, free text, reference, characteristic |
| 5 | `E1_COM_01` (under CTA_03), `E1_DTM_03` (under CCI_03), `E1_COM_02` (under CTA_04) | Communication channels, date/time details |

NAD_03 (2 occurrences, mandatory) carries the message sender/receiver identification. NAD_04 (1–99 occurrences) carries the party data payload (contacts, bank details, FTX, characteristics, availability periods).
