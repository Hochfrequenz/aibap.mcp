# Package: HFQ / BO4E2

**Description**: BO4E Schnittstelle zweiter Versuch
**Original language**: English (E)
**Number of objects**: 57 files across 30 distinct ABAP objects (2 behavior implementation classes, 3 CDS root view entities, 3 behavior definitions, 4 service definitions, 7 OData service bindings, 3 legacy OData V2 service artifacts, 6 authorization check stubs, 1 namespace object, 1 package definition, plus supporting metadata files)

## Executive Summary

BO4E2 is Hochfrequenz's second iteration of a BO4E (Business Objects for Energy) interface layer on SAP IS-U. It exposes three domain entities — **Adresse** (postal address), **Geschäftspartner** (business partner), and **Marktteilnehmer** (market participant) — plus a read-only **Geräte** (device/meter) view, all modelled using ABAP RAP (RESTful Application Programming). Each entity is backed by a CDS root view entity, an unmanaged behavior definition (BDEF), and a local behavior handler class. Service definitions (SRVD) wrap the CDS views; OData V4 service bindings (SRVB/G4BA) publish them as released or unreleased OData endpoints. A legacy OData V2 service for devices (`/HFQ/UI_GERAETE_R_V2`) and a legacy V2 service for business partners (`/HFQ/DD_GPARTNER_CDS`) are also present, indicating a migration from the classic ICF/SAP Gateway stack to the modern RAP/OData V4 stack. The `Adresse` behavior handler is the only one with a substantive implementation: it reads from and writes to the SAP central address table `ADRC`, using number range object `ADRNR` for key assignment.

---

## Classes

### `/HFQ/BP_DD_ADRESSE`

**Description**: Behavior Implementation for `/HFQ/DD_ADRESSE`
**Category**: ABAP class (behavior pool, category 06)
**Source files**:
- `#hfq#bp_dd_adresse.clas.abap` — empty shell class declaration `FOR BEHAVIOR OF /hfq/dd_adresse`
- `#hfq#bp_dd_adresse.clas.locals_imp.abap` — contains the real implementation in two local classes

**Local class `lhc_DD_ADRESSE`** (inherits `cl_abap_behavior_handler`):

| Method | Status | Notes |
|---|---|---|
| `create` | Implemented | Gets next address number via `cl_numberrange_runtime=>number_get` (number range object `ADRNR`, interval `01`). Maps BO4E fields to `ty_adrc_buffer` and appends to class-level static buffer `gt_create_buffer`. Returns mapped key `AdressNummer`. PLZ is written to `POST_CODE1` for street addresses and `POST_CODE2` for PO-box addresses. |
| `update` | Stub (empty body) | |
| `delete` | Stub (empty body) | |
| `read` | Implemented | Selects a single row from `ADRC` by `addrnumber`. Concatenates `house_num1` + `house_num2`. Uses a `CASE` expression to pick `post_code1` vs. `post_code2` based on whether `po_box` is initial. |
| `lock` | Implemented | Calls function module `ENQUEUE_EADRC`. Reports `'Adresse ist gesperrt'` on lock failure. |
| `get_instance_authorizations` | Commented out | A permissive placeholder is commented out; no authorization check is active. |

**Transactional buffer** (`gt_create_buffer`, type `ty_adrc_buffer`): holds staged `ADRC` rows between `create` and `save`.

**Local class `lsc_DD_ADRESSE`** (inherits `cl_abap_behavior_saver`):

| Method | Notes |
|---|---|
| `save` | Iterates `gt_create_buffer`, maps each entry to `ADRC` structure (sets `date_from = sy-datum`, `nation = space`), and executes `INSERT adrc`. Raises `cx_sy_open_sql_db` on failure. |
| `cleanup` | Clears `gt_create_buffer`. |
| `finalize`, `check_before_save`, `cleanup_finalize` | Empty stubs. |

---

### `/HFQ/BP_DD_MARKTTEILNEHMER`

**Description**: Behavior Implementation for `/HFQ/DD_MARKTTEILNEHMER`
**Category**: ABAP class (behavior pool, category 06)
**Source files**:
- `#hfq#bp_dd_marktteilnehmer.clas.abap` — empty shell class declaration `FOR BEHAVIOR OF /hfq/dd_marktteilnehmer`
- `#hfq#bp_dd_marktteilnehmer.clas.locals_imp.abap` — local handler and saver classes

**Local class `lhc_DD_Marktteilnehmer`** (inherits `cl_abap_behavior_handler`):

All methods (`get_instance_authorizations`, `create`, `update`, `delete`, `read`, `lock`) are declared but have empty implementations. This class is a scaffold; no actual data access is coded.

**Local class `lsc_DD_MARKTTEILNEHMER`** (inherits `cl_abap_behavior_saver`):

All methods (`finalize`, `check_before_save`, `save`, `cleanup`, `cleanup_finalize`) are empty stubs.

---

## CDS Views

### `/HFQ/DD_ADRESSE`

**Type**: Root view entity (RAP, unmanaged)
**Description**: CDS view für Adressen
**Source**: `#hfq#dd_adresse.ddls.asddls`
**Authorization check**: `#NOT_REQUIRED`
**Search enabled**: yes

**Base tables and joins**:

| Table | Alias | Join type | Purpose |
|---|---|---|---|
| `ADRC` | `adrc` | primary | Central SAP address table |
| `ILOA` | `iloa` | left outer | Functional location address link |
| `IFLOT` | `iflot` | left outer | Functional location master (for key access) |
| `EVBS` | `evbs` | left outer | IS-U connection object (Verbrauchsstelle) |
| `EANL` | `eanl` | left outer | IS-U installation (Anlage) |
| `BUT020` | `but020` | left outer | Business partner address assignment |

**Exposed fields**:

| CDS alias | Source expression | BO4E meaning |
|---|---|---|
| `AdressNummer` (key) | `adrc.addrnumber` | Address identifier |
| `postleitzahl` | `CASE po_box WHEN '' THEN post_code1 ELSE post_code2 END` | Postal code (street or PO box) |
| `ort` | `adrc.city1` | City name |
| `strasse` | `adrc.street` | Street name |
| `hausnummer` | `concat(house_num1, house_num2)` | House number including suffix |
| `postfach` | `adrc.po_box` | PO box |
| `adresszusatz` | `evbs.lgzusatz` | Location supplement (e.g., "3rd floor left") |
| `coErgaenzung` | `adrc.name_co` | c/o supplement |
| `landescode` | `adrc.country` | ISO country code |
| `vstelle` | `evbs.vstelle` | Connection object (Verbrauchsstelle) |
| `anlage` | `eanl.anlage` | Installation number |
| `partner` | `but020.partner` | Business partner number |
| `is_default_address` | `but020.xdfadr` | Default address flag |

**Note**: `adresszusatz`, `vstelle`, `anlage`, `partner`, and `is_default_address` are declared `readonly` in the behavior definition.

---

### `/HFQ/DD_GERAETE`

**Type**: Root view entity (read-only, no behavior definition)
**Description**: CDS view für Geräte
**Source**: `#hfq#dd_geraete.ddls.asddls`
**Authorization check**: `#NOT_REQUIRED`
**Search enabled**: yes

**Structure**: A `UNION` of two sub-queries, both rooted in `EUIINSTLN` (meter point / Zählpunkt):

**First branch** — historical meters (via `EGERH`):

| Table | Join type | Purpose |
|---|---|---|
| `EUIINSTLN` | primary | Meter point installation |
| `EUITRANS` | inner join | Meter point structure type |
| `EASTL` | left outer | Device installation link |
| `EGERH` | left outer | Historical device master |

`geraetetyp` and `bezeichnung` are hardcoded to `''` in this branch because CDS does not allow annotations in UNION branches and the enum mapping via `_enum` is commented out.

**Second branch** — current meters (via `EGERR`, with enum lookup):

| Table | Join type | Purpose |
|---|---|---|
| `EUIINSTLN` | primary | Meter point installation |
| `EUITRANS` | inner join | Meter point structure type |
| `EASTL` | left outer | Device installation link |
| `EGERR` | left outer | Current device master |
| `/HFQ/CDS_HLP_ENUM` as `_enum` | left outer | Enum mapping for `Geraetetyp` |

**Exposed fields**:

| CDS alias | Notes |
|---|---|
| `internZP` (key) | Internal meter point number (`int_ui`) |
| `anlagennummer` | Installation number (not in BO4E spec; used for cross-reference) |
| `zaehlernummer` | Device/counter number (not in BO4E spec) |
| `logNr` | Logic number (`logiknr`) — not in BO4E |
| `abDatum` | Valid-from date — not in BO4E |
| `bisDatum` | Valid-to date — not in BO4E |
| `geraetetyp` | BO4E device type (from enum mapping; empty in first branch) |
| `bezeichnung` | SAP material number / designation |

**Filter**: `euiinstln.euirole_dereg = 'X'` (deregulated meter points only). The `uistrutyp = 'ME'` filter is commented out in both branches.

---

### `/HFQ/DD_GPARTNER`

**Type**: Root view entity (RAP, unmanaged)
**Description**: Geschäftspartner
**Source**: `#hfq#dd_gpartner.ddls.asddls`
**Authorization check**: `#CHECK`
**Search enabled**: yes
**OData publish**: `@OData.publish: true` (legacy annotation still present)
**Composition root / create/update/delete**: all set to `false` in annotations (read-only intent)

**Base tables and joins**:

| Table | Join / Association | Purpose |
|---|---|---|
| `BUT000` | primary (`DISTINCT`) | Business partner master |
| `FKKVKP` | left outer | FI-CA contract account link |
| `EVER` | left outer | IS-U contract (`auszdat = '99991231'` filter for active contracts) |
| `EANL` | left outer | IS-U installation |
| `TSAD3T` | left outer | Title text (German, `langu = 'D'`) |
| `BUT001` | left outer | Business partner company data (Handelsregister) |
| `DFKKBPTAXNUM` | left outer | Tax number (`taxtype = 'DE0'`, German VAT ID) |
| `BUT020` + `ADR6` | left outer inline join | Email address |
| `BUT020` + `ADR12` | left outer inline join | Website URL |
| `SEPA_MANDATE` | left outer | SEPA mandate (creditor ID) |

**Associations**:

| Name | Target CDS | Cardinality | Purpose |
|---|---|---|---|
| `_kw` | `/HFQ/CDS_Kontaktweg` | 0..* | Contact channels |
| `_rolle` | `/HFQ/CDS_GPRolle` | 1..* | Business partner roles |
| `_adresse` | `/HFQ/CDS_COM_ADRESSE` | 0..1 | Default partner address |

**Exposed fields**:

| CDS alias | Notes |
|---|---|
| `gpnummer` (key) | Business partner number |
| `anlage` (key) | IS-U installation |
| `vertragsbeginn` | Contract start date |
| `vertrag` | Contract number |
| `anrede` | Title (German text) |
| `name1` | Last name / org name 1 / group name 1 (type-dependent CASE) |
| `name2` | First name / org name 2 / group name 2 |
| `name3` | Middle name / org name 3 |
| `gewerbekennzeichnung` | `'true'` if `type = '2'` (company), else `'false'` |
| `hrnummer` | Commercial register number |
| `amtsgericht` | Court of registration |
| `kontaktweg` | Association `_kw` |
| `umsatzsteuerId` | German VAT ID |
| `glaeubigerId` | SEPA creditor ID |
| `eMailAdresse` | Email (`adr6.smtp_addr`) |
| `website` | Website URL (`adr12.uri_srch`) |
| `geschaeftspartnerrolle` | Association `_rolle` |
| `addrnumber` | Address number |
| `partneradresse` | Association `_adresse` (default address only) |

**Filter**: `ever.auszdat = '99991231'` — workaround because `sy-datum` is not usable in CDS at the time of writing.

---

### `/HFQ/DD_MARKTTEILNEHMER`

**Type**: Root view entity (RAP, unmanaged)
**Description**: CDS view für Marktteilnehmer
**Source**: `#hfq#dd_marktteilnehmer.ddls.asddls`
**Authorization check**: `#NOT_REQUIRED`

**Base tables and joins**:

| Table / alias | Join type | Purpose |
|---|---|---|
| `ESERVPROVP` | primary | Service provider - business partner assignment |
| `ESERVPROV` | inner join | IS-U service provider master |
| `TECDE` | inner join | Technology definition (maps to service) |
| `/HFQ/CDS_HLP_ENUM` as `marktrolle` | left outer | Enum mapping for `Marktrolle` |
| `/HFQ/CDS_HLP_ENUM` as `codetyp` | left outer | Enum mapping for `RollenCodeTyp` |
| `EDEXDEFSERVPROV` + `EDEXCOMMFORMMAIL` + `EDEXCOMMMAILADDR` as `servprov_fremd` / `mailform_fremd` / `mailaddr_fremd` | left outer inline join chain | MaKo email address for external provider |
| `EDEXDEFSERVPROV` + `EDEXCOMMFORMMAIL` + `EDEXCOMMMAILADDR` as `servprov_eigen` / `mailform_eigen` / `mailaddr_eigen` | left outer inline join chain | MaKo email address for self-reference |

**Exposed fields**:

| CDS alias | Key | Notes |
|---|---|---|
| `rollencodenummer` | yes | External ID (e.g., BDEW market role code) |
| `rollencodetyp` | yes | Code type (mapped via enum) |
| `GPNummer` | | Business partner number |
| `marktrolle` | | BO4E market role (mapped via enum) |
| `makoadresse` | | MaKo email: prefers `mailaddr_fremd`, falls back to `mailaddr_eigen` |

---

## Behavior Definitions (BDEF)

### `/HFQ/DD_ADRESSE`

**Source**: `#hfq#dd_adresse.bdef.asbdef`
**Implementation**: unmanaged, class `/hfq/bp_dd_adresse`, `strict(2)`
**Operations**: `create`, `update`, `delete`
**Field constraints**:
- `readonly`: `AdressNummer`, `adresszusatz`, `vstelle`, `anlage`, `partner`, `is_default_address`
- `mandatory`: `postleitzahl`, `ort`, `strasse`, `hausnummer`, `landescode`

---

### `/HFQ/DD_GPARTNER`

**Source**: `#hfq#dd_gpartner.bdef.asbdef`
**Description**: Read and change Business Partners
**Implementation**: unmanaged, class `/hfq/bp_dd_gpartner`, `strict(2)`
**Operations**: `create`, `update`, `delete`

*Note*: There is no `/HFQ/BP_DD_GPARTNER` class exported in this package. The behavior implementation class is absent from the export. *Inferred from [bdef source]: the class is referenced but not present in this package snapshot.*

---

### `/HFQ/DD_MARKTTEILNEHMER`

**Source**: `#hfq#dd_marktteilnehmer.bdef.asbdef`
**Description**: Markteilnehmer lesen
**Implementation**: unmanaged, class `/hfq/bp_dd_marktteilnehmer`, `strict(2)`
**Operations**: `create`, `update`, `delete`
**Field constraints**: `readonly`: `GPNummer`

---

## OData Service Layer

### Service Definitions (SRVD)

| Name | Exposes | Description |
|---|---|---|
| `/HFQ/ADRESSEN_R` | `/HFQ/DD_ADRESSE` | Read-Only: Adressen |
| `/HFQ/GERAETE_R` | `/HFQ/DD_GERAETE` | Lesen von Geräten |
| `/HFQ/GPARTNER_R` | `/HFQ/DD_GPartner` | Lesen von Geschäftspartnern |
| `/HFQ/MARKTTEILNEHMER_R` | `/HFQ/DD_Marktteilnehmer` | Read Marktteilnehmer |

All four are of type `SRVD/SRV` and were created via ABAP Development Tools.

---

### OData V4 Service Bindings (SRVB / G4BA)

| Binding name | Bound service (SRVD) | OData version | Release state | Description |
|---|---|---|---|---|
| `/HFQ/UI_ADRESSEN_R_V4` | `/HFQ/ADRESSEN_R` | V4 | RELEASED | Lesen von Adressen |
| `/HFQ/MAKRKTEILNEHMER_R_V4` | `/HFQ/MARKTTEILNEHMER_R` | V4 | RELEASED | Lesen von Marktteilnehmern |
| `/HFQ/UI_GERAETE_R_V4` | `/HFQ/GERAETE_R` | V4 | NOT_RELEASED | Lesen von Geräten |
| `/HFQ/UI_GPARTNER_R_V4` | `/HFQ/GPARTNER_R` | V4 | NOT_RELEASED | OData Service V4 |

*Note*: The name `/HFQ/MAKRKTEILNEHMER_R_V4` contains a typo (`MAKRK` instead of `MARKT`); this is as found in the source.

All four bindings have `CONTRACT=C2`, `PUBLISHED=true`, `BINDING_CREATED=true`.

---

### Legacy OData V2 Artifacts

#### `/HFQ/UI_GERAETE_R_V2` (IWSV + IWMO + SRVB)

| Artifact type | Technical name | Description |
|---|---|---|
| IWSV (service registration) | `/HFQ/UI_GERAETE_R_V2` v0001 | Lesen von Geräten |
| IWMO (model provider) | `/HFQ/UI_GERAETE_R_V2` v0001 | uses `CL_SADL_GW_RAP_EXPOSURE_MPC` |
| SRVB | `/HFQ/UI_GERAETE_R_V2` | Binds `/HFQ/GERAETE_R` as OData **V2**, RELEASED |

The model provider class `CL_SADL_GW_RAP_EXPOSURE_MPC` indicates this V2 service exposes the RAP CDS view via the SAP Gateway SADL bridge.

#### `/HFQ/DD_GPARTNER_CDS` (IWSV + IWMO + IWVB)

| Artifact type | Technical name | Description |
|---|---|---|
| IWSV (service registration) | `/HFQ/DD_GPARTNER_CDS` v0001 | Geschäftspartner |
| IWMO (model provider) | `/HFQ/DD_GPARTNER_CDS` v0001 | uses `CL_SADL_GW_AUTO_EXPOSURE_MPC` |
| IWVB (vocabulary/annotation) | `/HFQ/DD_GPARTNER_CDS_VAN` v0001 | Generic Annotation Provider (`CL_SADL_GW_CDS_EXPOSURE_APC`) |

This is the classic CDS-published OData V2 service generated via `@OData.publish: true` on the `/HFQ/DD_GPARTNER` CDS view.

---

## Other Objects

### Authorization Check Stubs (SUSH)

Six `SUSH` (authorization check) hash-named objects are present. They record which authorization objects were checked when activating the service bindings and IW services. None define restrictive authorization logic — they register `S_START` (TADIR object start check) or `S_SERVICE` (ICF service check) as "OK" (permissive).

| SUSH hash | Associated object | Auth object checked |
|---|---|---|
| `05DD49703F428BA20DC12D9821D3A1` | `G4BA /HFQ/UI_ADRESSEN_R_V4` | `S_START` (OK) |
| `9F6AD34C4E5EEA87F4E72D739D7169` | `G4BA /HFQ/UI_GERAETE_R_V4` | `S_START` (OK) |
| `EA50074EDE14A1EC59293CC424AC14` | `G4BA /HFQ/MAKRKTEILNEHMER_R_V4` | `S_START` (OK) |
| `3590969CB2BBE1E7365D8DD522BBFE` | `G4BA /HFQ/UI_GPARTNER_R_V4` | none defined |
| `CE34D3FCDF14727D36095DE7F1DC8F` | `IWSV /HFQ/UI_GERAETE_R_V2 0001` | `S_SERVICE` (OK) |
| `ABC49DB8F475175BD1F3BC1777A39F` | `IWSV /HFQ/DD_GPARTNER_CDS 0001` | none defined |

### Published API Registrations (APIS)

Three `APIS` objects register service bindings in the API framework:

| Object ID | Type | Registered name |
|---|---|---|
| `/HFQ/UI_ADRESSEN_R_V4` | G4BA | `/HFQ/UI_ADRESSEN_R_V4` |
| `/HFQ/MAKRKTEILNEHMER_R_V4` | G4BA | `/HFQ/MAKRKTEILNEHMER_R_V4` |
| `/HFQ/UI_GELYI7XHMANMZTSI6W5LD67UTQ3Y` | IWSV | `/HFQ/UI_GERAETE_R_V2 0001` |

The third entry uses an obfuscated/hashed object ID for the legacy IWSV registration.

### Namespace Object

`/HFQ/` namespace descriptor (`#hfq#.nspc.xml`): records the `/HFQ/` namespace with German description "Hochfrequenz".
