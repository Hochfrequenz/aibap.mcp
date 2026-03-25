# Package: HFQ / KOMDATA_SND

**Description**: Versand PARTIN (PARTIN Send)
**Original language**: German (D)
**Number of objects**: 27 files across 4 classes, 1 interface, 1 function group, 1 enhancement spot, 2 transparent tables, 3 data elements, 1 namespace object

---

## Executive Summary

`HFQ/KOMDATA_SND` implements the outbound PARTIN process for the German energy market contact data sheet (Kontaktdatenblatt). PARTIN is an EDIFACT message type used between market participants (grid operators, suppliers, metering point operators, etc.) to exchange master contact data.

The package covers the full outbound pipeline:

1. **Process triggering** — two function modules (`/HFQ/TRIG_PARTIN_MAS` for mass send, `/HFQ/TRIG_PARTIN_SND` for single send) set up the `/IDXGC/` process framework context, derive the correct AMID from the sender's service category, and start the process.
2. **Data provision** — `/HFQ/CL_DP_PARTIN` (inheriting `/IDXGL/CL_DP`) collects all PARTIN payload fields (address, contacts, bank data, availability windows, settlement units, certificates, etc.) from the customizing-driven source and populates the process step data structure.
3. **Validation** — two dedicated check classes validate data before and after enrichment. `/HFQ/CL_PARTIN_CHECK` is a stateful, version-aware validator that performs format and completeness checks on individual data domains. `/HFQ/CL_KOMDATA_SND_CHECK` acts as the process framework check class (static methods conforming to `/IDXGC/` check callback signatures) and also determines the list of receivers and triggers single-send sub-processes.
4. **IDoc mapping** — `/HFQ/CL_PARTIN_MAPPING` translates process step data into/from EDIFACT IDoc segments (NAD, CTA, COM, FII, FTX, CCI, CAV, DTM, RFF).
5. **Extensibility** — the BAdI `/HFQ/BADI_KOMDATA_SND_FILT` (defined in enhancement spot `/HFQ/ES_KOMDATA_SND`, interface `/HFQ/IF_BADI_KOMDATA_SND_FILT`) allows custom filtering of the determined receiver list before sending.
6. **Customizing tables** — `/HFQ/MAND_BANK` and `/HFQ/MAND_CONT` store which bank account types and contact types are mandatory per sender, driving the validation and data provision logic.

The package integrates deeply with the SAP IS-U add-on namespace `/IDXGC/` (process framework, check framework, IDoc/IDE layer) and the `/HFQ/` PARTIN base packages that hold constants, exception classes, and DB access helpers.

---

## Classes

### `/HFQ/CL_DP_PARTIN` — Data Provision Class PARTIN

**File**: `#hfq#cl_dp_partin.clas.abap` / `#hfq#cl_dp_partin.clas.xml`
**Superclass**: `/IDXGL/CL_DP`
**Visibility**: public, instantiable

Responsible for gathering all PARTIN payload data and populating the process step data structure (`/IDXGC/S_PROC_STEP_DATA_ALL`). Each logical data domain is encapsulated in its own public instance method. All methods raise `/IDXGC/CX_PROCESS_ERROR`.

The class holds protected internal tables and scalars as state buffers; these are populated before calling individual methods, or derived from customizing/source data depending on the data processing mode (`gc_data_from_source`, `gc_data_from_add_source`, `gc_default_processing`).

**Key protected attributes**

| Attribute | Type | Description |
|---|---|---|
| `SIV_NAD_QUAL_SENDER` | `/IDXGC/DE_PARTY_FUNC_QUAL` | Service type qualifier of sender |
| `SIT_CONTACTS` | `/HFQ/T_KD_CONTACT` | Contact address records |
| `SIT_AVAILABILITY` | `/HFQ/T_KD_AVAIL` | Availability time windows |
| `SIT_BILANZKREIS` | `/HFQ/T_KD_BIKREIS` | Settlement unit records |
| `SIT_BANK_DATA` | `/HFQ/T_KD_BANK` | Bank account records |
| `SIS_PARTIN_HEADER` | `/HFQ/S_KD_HDR` | PARTIN header data |
| `SIV_VERSION` | `/HFQ/DE_KD_VERSION` | Current PARTIN version |

**Public methods**

| Method | Description |
|---|---|
| `FILL_PARTIN_DATA` | Helper to populate all PARTIN data without IDoc metadata; orchestrates the individual domain methods. Changing parameter: `CS_PROCESS_STEP_DATA` of type `/IDXGC/S_PROC_STEP_DATA_ALL`. |
| `SENDER_ADDRESS` | Fills sender address (NAD segment source data). |
| `CONTACTS` | Fills contact address records; maps `contact_type` → `party_func_qual`, `contact` → `fam_comp_name1`, address fields. Skips if document status is code 11. |
| `CONTACT_COMM` | Fills contact communication channels. |
| `INFO_CONTACTS` | Fills contact name records. |
| `AVAILABILITY` | Fills availability time windows; validates mandatory time types from `/HFQ/CL_PARTIN_DB=>GET_CUST_MAND_AVAIL`; raises error if required types are missing or duplicated. Skips if document status is code 11. |
| `BANK_DATA` | Fills bank account data; validates mandatory bank types from `/HFQ/CL_PARTIN_DB=>GET_CUST_MAND_BANK`; raises error if required types are missing or duplicated. Skips if document status is code 11. |
| `BILANZKREIS` | Fills settlement unit (Bilanzkreis) data; only relevant when sender/receiver service category combination matches customizing in `/HFQ/MAND_BIKR`. Validates mandatory Bilanzkreis types. |
| `INACTIVITY` | Sets the invalidity flag in process step data. |
| `DOCUMENT_VERSION` | Sets document version reference. |
| `DOCUMENT_VERSION_AS4` | Sets document version reference for AS4 communication. |
| `EDI_COMM` | Fills EDIFACT communication data. |
| `SUBJECT` | Fills certificate subject/user field. |
| `ISSUER` | Fills certificate issuer field. |
| `URI` | Fills URI (issuer) field. |
| `FAX_NUMBER` | Fills general fax number of sender. |
| `URL` | Fills sender website URL. |
| `TAX` | Fills tax ID and/or tax number. |
| `TAX_ID` | OLD / DO NOT USE — fills sender taxation ID (deprecated). |
| `COURT` | Fills court and commercial registry information. |
| `INSTANTIATE` | Redefinition of superclass method. |
| `MESSAGE_CATEGORY` | Redefinition of superclass method. |

---

### `/HFQ/CL_KOMDATA_SND_CHECK` — PARTIN Send Check Class

**File**: `#hfq#cl_komdata_snd_check.clas.abap` / `#hfq#cl_komdata_snd_check.clas.xml`
**Visibility**: public, not inheriting
**Design**: all methods are static class-methods; conforms to the `/IDXGC/` check framework callback signature (`IS_PROCESS_STEP_KEY`, `ET_CHECK_RESULT`, `CR_DATA`, `CR_DATA_LOG`).

Performs pre-send completeness and business-rule checks for PARTIN outbound processes, and handles receiver determination and single-send process triggering.

**Check result constants**

| Constant | Value | Meaning |
|---|---|---|
| `GC_CR_INCONSISTENT` | `'INCONSISTENT'` | Data is logically inconsistent |
| `GC_CR_MISSING` | `'MISSING'` | Required data is absent |
| `GC_CR_VERSION_ERROR` | `'VERSION_ERROR'` | Version number rule violated |
| `GC_CR_WRONG_SP` | `'WRONG_SP'` | Wrong service provider |
| `GC_CR_FORMAT_ERROR` | `'FORMAT_ERROR'` | Format validation failed |
| `GC_CR_TOO_MANY` | `'TOO_MANY'` | Too many entries for a type |

**Static methods**

| Method | Description | Raises |
|---|---|---|
| `CHECK_AS4` | Validates AS4 communication data: checks that version qualifier AGK exists and is strictly greater than previous version ACW; checks exactly 2 FTX comment rows with qualifiers Z23/Z24 (certificate issuer/user) are present and non-empty; checks sender is own service provider and receiver is filled. Returns OK or INCOMPLETE/ERROR. | `/IDXGC/CX_UTILITY_ERROR` |
| `CHECK_API` | Validates API communication data: same version checks as CHECK_AS4; checks exactly 3 FTX comment rows with qualifiers Z17/Z23/Z24 (URI + certificate issuer/user). | `/IDXGC/CX_UTILITY_ERROR` |
| `CHECK_AS4_OR_PARTIN` | Determines whether AS4 or classic PARTIN communication is in use and delegates accordingly. | `/IDXGC/CX_UTILITY_ERROR` |
| `CHECK_AVAILABILITY` | Validates availability time window completeness. | — |
| `CHECK_BILANZKREIS` | Validates settlement unit data. | — |
| `CHECK_BANK_DATA` | Validates bank account data completeness. | — |
| `CHECK_CONTACTS` | Validates contact data. | `/IDXGC/CX_UTILITY_ERROR` |
| `CHECK_SENDER_ROLE` | Determines and validates the sender's market role. | — |
| `CHECK_SP_HEADER` | Validates company header data (Kopfdaten des Unternehmens). | `/IDXGC/CX_UTILITY_ERROR` |
| `FETCH_RECEIVERS` | Determines the list of service providers to whom a PARTIN must be sent. | `/IDXGC/CX_UTILITY_ERROR` |
| `FETCH_RECEIVERS_API` | Determines receivers for API-based PARTIN communication. | `/IDXGC/CX_UTILITY_ERROR` |
| `FETCH_RECEIVERS_BILANZKREIS` | Determines receivers specifically for Bilanzkreis-relevant PARTIN. | `/IDXGC/CX_UTILITY_ERROR` |
| `TRIGGER_SINGLE_SND` | Triggers the individual single-send sub-processes after receivers have been determined. | `/IDXGC/CX_UTILITY_ERROR` |

---

### `/HFQ/CL_PARTIN_CHECK` — PARTIN Message Validator

**File**: `#hfq#cl_partin_check.clas.abap` / `#hfq#cl_partin_check.clas.xml`
**Visibility**: public, final (not inheritable)

A stateful, version-aware validator. An instance is created for a specific key date (determining the applicable PARTIN message version via `DET_PARTIN_VERSION`). Validation is performed per data domain; each check method accumulates errors into a changing table of type `/HFQ/T_KD_ERROR`.

**Constructor** — `IV_KEYDATE type DATS` — raises `/HFQ/CX_PARTIN_ERROR`.

**Static methods**

| Method | Returns |
|---|---|
| `DET_PARTIN_VERSION( IV_KEYDATE )` | `RV_MESCOD type EDI_MESCOD` — the applicable PARTIN version code for the given date. |

**Instance check methods** — all have `CT_ERROR type /HFQ/T_KD_ERROR` as changing parameter and optional `IV_SENDER`, `IV_RECEIVER` (`SERVICE_PROV`), `IV_STRICT` (`BOOLEAN`).

| Method | Additional Inputs | Description |
|---|---|---|
| `CHECK_HEAD` | `IS_HEADER type /HFQ/S_KD_HDR` | Validates PARTIN header data (company master data). |
| `CHECK_CONTACTS` | `IT_CONT type /HFQ/T_KD_CONTACT` | Validates contact records: format checks on address, email, phone, fax, court registry, URL, tax. Strict mode also checks for superfluous contact types. |
| `CHECK_BANK_DATA` | `IT_BANK type /HFQ/T_KD_BANK` | Validates bank records: IBAN, BIC, account holder name, bank name. |
| `CHECK_AVAILABILITY` | `IT_AVAIL type /HFQ/T_KD_AVAIL` | Validates availability windows: checks required time types per customizing, detects duplicates (`TOO_MANY`), validates from-time ≤ to-time (`INCONSISTENT`), validates that mandatory types are present (`MISSING`). |
| `CHECK_BILANZKREIS` | `IT_BIKREIS type /HFQ/T_KD_BIKREIS`, `IV_VALID_FROM type /HFQ/DE_KD_VON` | Validates settlement unit data. |

**Private format-check helpers** — return `RT_ERROR type /HFQ/T_KD_ERROR`

| Helper | Validates |
|---|---|
| `FORMAT_CHECK_ADDRESS` | Street, city, postal code, country code, house number |
| `FORMAT_CHECK_BANK` | IBAN, account holder name, BIC, bank name |
| `FORMAT_CHECK_CONTACT` | Contact name |
| `FORMAT_CHECK_EMAIL` | Email address format |
| `FORMAT_CHECK_INFO_CONTACT` | Info contact name, email, fax, phone |
| `FORMAT_CHECK_COURT` | Court name, commercial register number |
| `FORMAT_CHECK_TEL` | Phone number format |
| `FORMAT_CHECK_URL` | URL format |
| `FORMAT_CHECK_TAX` | Tax ID, tax number, country code |

**Private state**

| Attribute | Description |
|---|---|
| `MV_MESCOD type EDI_MESCOD` | PARTIN message version code determined at construction |
| `MR_PREVIOUS` (class-data) | Last caught `/HFQ/CX_PARTIN_ERROR` |

---

### `/HFQ/CL_PARTIN_MAPPING` — PARTIN IDoc Mapping

**File**: `#hfq#cl_partin_mapping.clas.abap` / `#hfq#cl_partin_mapping.clas.xml`
**Visibility**: public, not inheriting

Translates between the process step data structure (`/IDXGC/S_PROC_STEP_DATA`) and EDIFACT IDoc segment structures (`EDEX_IDOCDATA`). Handles both inbound parsing and outbound generation of IDoc data.

**Constructor** — `IS_IDOC_DATA`, `IS_PROC_STEP_DATA`, `IV_MSG_VERSION type CHAR4` — stores context and builds the set of relevant NAD qualifier codes based on message version (versions `1.0`, `1.0a`, `1.0b`, `1.0c`, `1.0e` supported; version `1.0a`–`1.0e` additionally include qualifier Z33).

**Message version constants**

| Constant | Value |
|---|---|
| `GC_MSG_VERSION_10` | `'1.0'` |
| `GC_MSG_VERSION_10A` | `'1.0a'` |
| `GC_MSG_VERSION_10B` | `'1.0b'` |
| `GC_MSG_VERSION_10C` | `'1.0c'` |
| `GC_MSG_VERSION_10E` | `'1.0e'` |

**Segment name constants** — reference the `/IDXGC/` IDoc segment types

| Constant | Value |
|---|---|
| `GC_SEGNAM_NAD_04` | `/IDXGC/E1_NAD_04` |
| `GC_SEGNAM_CTA_04` | `/IDXGC/E1_CTA_04` |
| `GC_SEGNAM_COM_02` | `/IDXGC/E1_COM_02` |
| `GC_SEGNAM_FII_01` | `/IDXGC/E1_FII_01` |
| `GC_SEGNAM_FTX_01` | `/IDXGC/E1_FTX_01` |
| `GC_SEGNAM_CCI_01` | `/IDXGC/E1_CCI_01` |
| `GC_SEGNAM_CAV_01` | `/IDXGC/E1_CAV_01` |
| `GC_SEGNAM_DTM_03` | `/IDXGC/E1_DTM_03` |
| `GC_SEGNAM_RFF_02` | `/IDXGL/E1_RFF_02` |
| `GL_SEGNAM_NAD_01` | `/IDXGL/E1_NAD_01` |
| `GC_REF2MSG_QUAL_AGK` | `'AGK'` |

**Public methods**

| Method | Direction | Description |
|---|---|---|
| `FILL_SG4_NAD04_SEGMENTS_OUT` | Outbound | Main method: populates all SG4/NAD04 IDoc segments from process step data. Changing: `CS_IDOC_DATA`. Raises `/IDXGC/CX_IDE_ERROR`. |
| `PROC_SG4_NAD04_SEGMENTS_IN` | Inbound | Main method: reads SG4/NAD04 segments from IDoc data and populates process step data. Changing: `CS_PROC_STEP_DATA`. Raises `/IDXGC/CX_IDE_ERROR`. |

**Key private fill methods** (called by `FILL_SG4_NAD04_SEGMENTS_OUT`)

| Method | Segment | Content |
|---|---|---|
| `FILL_NAD_01` / `FILL_NAD_01_API` / `FILL_NAD_01_AS4` | NAD_01 | Sender address (variant per communication mode) |
| `FILL_NAD_04_GC` | NAD_04 | Contact address |
| `FILL_CTA_04` | CTA_04 | Contact name |
| `FILL_COM_02` | COM_02 | Communication channels (phone TE, email EM, fax FX) |
| `FILL_FII_01` | FII_01 | Bank data: IBAN, BIC, account holder, bank name |
| `FILL_FTX_01` / `FILL_FTX_01_AS4` / `FILL_FTX_01_API` | FTX_01 | Free text (certificate/URI data, variant per mode) |
| `FILL_CCI_01_CAV_01` | CCI_01 + CAV_01 | Settlement unit (Bilanzkreis): class type Z19 + value |
| `FILL_CCI_01_DTM_03` | CCI_01 + DTM_03 | Availability windows: class type Z40 + time periods (format 501, HHMM concatenated) |
| `FILL_RFF_02` | RFF_02 | Reference qualifier AGK + version number |

---

## Interfaces

### `/HFQ/IF_BADI_KOMDATA_SND_FILT` — BAdI Interface for PARTIN Receiver Filtering

**File**: `#hfq#if_badi_komdata_snd_filt.intf.abap` / `#hfq#if_badi_komdata_snd_filt.intf.xml`
**Extends**: `IF_BADI_INTERFACE`
**Description**: Interface for the BAdI that allows custom filtering of the PARTIN receiver list.

**Method**

| Method | Signature | Description |
|---|---|---|
| `FILTER_SERV_PROVS` | `CHANGING CT_SERVPROV type T_ESERVPROV` | Called after receiver determination; implementations can remove entries from the receiver table to suppress sending to specific service providers. |

The corresponding BAdI definition is `/HFQ/BADI_KOMDATA_SND_FILT`, defined in enhancement spot `/HFQ/ES_KOMDATA_SND` with context mode `N` (no filter). The BAdI short text is "BAdI für Filterung von Empfänger-SAs".

---

## Function Groups

### `/HFQ/FG_TRIG_PROC` — Process Trigger Function Group

**Files**: `#hfq#fg_trig_proc.fugr.xml`, `#hfq#fg_trig_proc.fugr.#hfq#lfg_trig_proctop.abap`, `#hfq#fg_trig_proc.fugr.#hfq#saplfg_trig_proc.abap`, `#hfq#fg_trig_proc.fugr.#hfq#trig_partin_mas.abap`, `#hfq#fg_trig_proc.fugr.#hfq#trig_partin_snd.abap`

Contains two function modules that serve as the main entry points for triggering PARTIN outbound processes. Both modules raise `/HFQ/CX_PARTIN_ERROR` and export `ES_PROC_STEP_ALL type /IDXGC/S_PROC_STEP_DATA_ALL`.

---

#### `/HFQ/TRIG_PARTIN_MAS` — Trigger PARTIN Mass Send

Triggers the PARTIN mass-send process (`gc_procid_komdat_mas`). Does not perform data provision itself — the process framework handles that separately.

**Import parameters**

| Parameter | Type | Optional | Description |
|---|---|---|---|
| `IV_VERSION` | `/HFQ/DE_KD_VERSION` | No | PARTIN version number (AGK reference) |
| `IV_SERVICE_SEN` | `SERVICE_PROV` | No | Sending service provider ID |
| `IV_CODE11_STATUS` | `ABAP_BOOL` | Yes | If true, sets document status to code 11 (inactivity) |

**Logic**:
1. Reads process config for `gc_procid_komdat_mas`.
2. Derives intcode from sender's service ID via `/HFQ/CL_PARTIN_HELPER=>GET_INTCODE`.
3. Maps intcode to AMID: Distributor→37001, Supplier→37000, MOS→37002, BKV→37003, SETLCO→37004, TSO→37005, EP→37006.
4. Reads BMID from `/IDXGC/AMID_CONF` by AMID and current date.
5. Assembles `ls_proc_data` with direction=outbound, own_servprov=sender, document_status=code11 if applicable, version in `ref_to_msg` with qualifier AGK.
6. Calls `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS`.
7. Returns the resulting `ES_PROC_STEP_ALL`.

---

#### `/HFQ/TRIG_PARTIN_SND` — Trigger PARTIN Single Send

Triggers a single PARTIN outbound process (`gc_procid_komdat_snd`) with full data provision.

**Import parameters**

| Parameter | Type | Optional | Description |
|---|---|---|---|
| `IV_VERSION` | `/HFQ/DE_KD_VERSION` | No | PARTIN version number |
| `IV_SERVICE_SEN` | `SERVICE_PROV` | No | Sending service provider ID |
| `IV_SERVICE_REC` | `SERVICE_PROV` | No | Receiving service provider ID |
| `IV_OBJECT` | `SWO_OBJTYP` | Yes | Business object type (for mass-send context) |
| `IV_OBJECTKEY` | `EIDEGENERICKEY` | Yes | Business object key (for mass-send context) |
| `IV_DOCUSTATUS` | `/HFQ/DE_KD_DOCU_STATUS` | Yes | Document status (e.g., code 11 for inactivity) |

**Logic**:
1. Reads process config for `gc_procid_komdat_snd`.
2. Derives intcode and maps to AMID (same logic as `TRIG_PARTIN_MAS`).
3. Reads BMID from `/IDXGC/AMID_CONF`.
4. Assembles process header with `IV_OBJECT`/`IV_OBJECTKEY` for mass-send parent tracking.
5. Assembles step data with sender, receiver, document status, and AGK version reference.
6. Instantiates `/HFQ/CL_DP_PARTIN` and calls `FILL_PARTIN_DATA` to enrich step data.
7. Calls `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS`.
8. Issues `COMMIT WORK AND WAIT`.
9. Returns `ES_PROC_STEP_ALL`.

---

## Tables / Data Definitions

### `/HFQ/MAND_BANK` — Required Bank Account Types Customizing

**File**: `#hfq#mand_bank.tabl.xml`
**Table class**: TRANSP (transparent), client-dependent, customizing (`CONTFLAG=C`)
**Description**: "Notwendige Felder der Bankverbindungen" (Required fields of bank accounts)

| Field | Key | Data Element | Description |
|---|---|---|---|
| `MANDT` | X | `MANDT` | Client |
| `OWN_SERVICE` | X | `/HFQ/DE_KD_OWN_SERV` | Own service provider ID |
| `BANK_TYPE` | X | `/HFQ/DE_KD_BANK_TYPE` | Bank account type |

*Inferred:* Rows in this table define which bank account types (identified by `BANK_TYPE`) are mandatory for a given sending service provider. The data provision and check logic reads this table via `/HFQ/CL_PARTIN_DB=>GET_CUST_MAND_BANK`.

---

### `/HFQ/MAND_CONT` — Required Contact Types Customizing

**File**: `#hfq#mand_cont.tabl.xml`
**Table class**: TRANSP (transparent), client-dependent, customizing (`CONTFLAG=C`)
**Description**: "Customizingtabelle für notwendige Kontakttypen" (Customizing table for required contact types)

| Field | Key | Data Element | Description |
|---|---|---|---|
| `MANDT` | X | `MANDT` | Client |
| `CONTACT_TYPE` | X | `/HFQ/DE_KD_CONTACT_TYPE` | Contact type code |
| `OWN_SERVICE` | X | `/HFQ/DE_KD_OWN_SERV` | Own service provider ID |
| `MANDATORY_LF` | — | `/HFQ/DE_KD_MANDATORY_LF` | Required for sending to LF (Lieferant/supplier) |
| `MANDATORY_NB` | — | `/HFQ/DE_KD_MANDATORY_NB` | Required for sending to NB (Netzbetreiber/grid operator) |
| `MANDATORY_MSB` | — | `/HFQ/DE_KD_MANDATORY_MSB` | Required for sending to MSB (Messstellenbetreiber/metering point operator) |

The three mandatory flags use domain `EBA_FLAG` (boolean flag). This table drives per-receiver-type contact type requirements in validation and data provision.

---

## Domains and Data Elements

### `/HFQ/DE_KD_MANDATORY_LF`

**File**: `#hfq#de_kd_mandatory_lf.dtel.xml`
**Domain**: `EBA_FLAG`
**Description**: "Notwendig für Senden an LF" (Required for sending to LF/Supplier)
Screen texts: "Notw. LF" (short), "Notwendig für LF" (medium), "Notwendig für Senden an LF" (long).

---

### `/HFQ/DE_KD_MANDATORY_MSB`

**File**: `#hfq#de_kd_mandatory_msb.dtel.xml`
**Domain**: `EBA_FLAG`
**Description**: "Notwendig für Senden an MSB" (Required for sending to MSB/Metering point operator)
Screen texts: "Notw. MSB" (short), "Notwendig für MSB" (medium), "Notwendig für Senden an MSB" (long).

---

### `/HFQ/DE_KD_MANDATORY_NB`

**File**: `#hfq#de_kd_mandatory_nb.dtel.xml`
**Domain**: `EBA_FLAG`
**Description**: "Notwendig für Senden an NB" (Required for sending to NB/Grid operator)
Screen texts: "Notw- NB" (short), "Notwendig für NB" (medium), "Notwendig für Senden an NB" (long).

---

## Other Objects

### `/HFQ/ES_KOMDATA_SND` — Enhancement Spot (BAdI Definition)

**File**: `#hfq#es_komdata_snd.enhs.xml`
**Tool**: `BADI_DEF`

Defines the BAdI `/HFQ/BADI_KOMDATA_SND_FILT`:

| Property | Value |
|---|---|
| BAdI name | `/HFQ/BADI_KOMDATA_SND_FILT` |
| Interface | `/HFQ/IF_BADI_KOMDATA_SND_FILT` |
| Context mode | `N` (no filter/context) |
| Short text | "BAdI für Filterung von Empfänger-SAs" |

---

### `/HFQ/` Namespace Object

**File**: `#hfq#.nspc.xml`
**Namespace**: `/HFQ/`
**Owner**: Hochfrequenz
Languages: German (D).
