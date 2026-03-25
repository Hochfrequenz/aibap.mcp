# Package: HFQ / KOMDATA_TOOL

**Description**: Werkzeuge PARTIN (PARTIN tools)
**Original language**: English (E)
**Number of objects**: 5 (3 reports, 2 transactions, 1 package definition)

## Executive Summary

This package provides utility programs for managing PARTIN (market partner communication data) in the German energy market context. Three reports cover distinct tasks: bulk display of contact data from multiple service providers in an ALV grid, a database update tool to populate missing receiver service provider IDs in PARTIN communication tables, and a data inspection report for viewing historical bank and EDI communication data from PARTIN tables. The package complements the main KOMDATA packages and is oriented toward operational administration rather than core business logic.

---

## Reports

### `/HFQ/RP_KOMDATA_DISP_MASS` — Display contact data in bulk

**Transaction**: `/HFQ/KOMDATA_MASS` ("Display von Kontaktdaten in Masse")

**Selection screen:**

| Parameter/Select-option | Type | Description |
|---|---|---|
| `SO_SP` | Select-option for `SERVICE_PROV` | Filter by service provider (sender side) |
| `SO_CT` | Select-option for `/HFQ/DE_KD_CONTACT_TYPE` | Filter by contact type; F4 matchcode object `/HFQ/KOMDATA_SH_CONTACT_TYPE` |
| `SO_OWN` | Select-option for `ESERVPROV-SERVICEID` | Filter by own service providers (those with `OWN_LOG_SYS = 'X'`); custom F4 help provided |

**Behavior (verified from implementation):**
1. At `INITIALIZATION`, pre-selects all service providers that are own (`OWN_LOG_SYS = 'X'`) and appear in `/HFQ/INTCODES` (PARTIN-relevant types).
2. Custom F4 on `SO_OWN-LOW` uses `F4IF_INT_TABLE_VALUE_REQUEST` over the pre-filtered list.
3. At `START-OF-SELECTION`, queries `ESERVPROV` for the requested service providers.
4. If `SO_OWN` is empty, defaults to all own service providers.
5. For each sender/receiver combination, instantiates `/HFQ/CL_PARTIN` and calls `GET_CONT_ALL()` to retrieve all contacts; filters by `SO_CT`.
6. Removes entries where `INFO_CONTACT IS INITIAL`.
7. Displays results in a `CL_SALV_TABLE` ALV grid with optimization, striped pattern, unrestricted layout save, and all function buttons enabled.

**Output fields**: `SERVICE_PROV`, `SERVICEID_RECEIVER`, `CONTACT_TYPE`, `CONTACT_TYPE_TEXT`, `INFO_CONTACT`, `EMAIL`, `TEL`, `FAX`, `CONTACT`, `ADDRESS`, `HOUSE_NUM`, `POST_CODE`, `CITY`, `COUNTRY_CODE`

---

### `/HFQ/RP_KOMDATA_FILL_RECEIVER` — Populate receiver field in PARTIN tables

**Transaction**: `/HFQ/KOMDATA_EMPFANG` ("Füllt leere Felder in PARTIN-Tabelle")

**Selection screen:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `P_RECEIV` | `TEXT100` | `' '` | New receiver service provider ID to set |
| `P_OLD` | `TEXT100` | `' '` | Old receiver value to be replaced |

**Behavior (verified from implementation):**
1. Both parameters are uppercased (`TRANSLATE TO UPPER CASE`).
2. Selects all own service provider IDs from `ESERVPROV` and builds a range table via `/IDXGC/CL_UTILITY_SERVICE`.
3. For each of the three PARTIN communication tables (`/HFQ/PTCOMP`, `/HFQ/PTCOMV`, `/HFQ/PTCOMTIME`):
   - Sets `SERVICEID_RECEIVER = P_RECEIV` where it currently equals `P_OLD`.
   - Then clears `SERVICEID_RECEIVER` (sets to space) where `SERVICEID` is in the own-service-provider range (own records should have an empty receiver field).
4. Error message `E072(/HFQ/MSG_KOMDATA)` on any failed `UPDATE`.

**Note**: This is a direct SQL update tool without intermediate display or simulation mode. Use with care in production.

---

### `/HFQ/RP_READ_OLD_DATA` — View historical PARTIN data

**Selection screen:**

| Parameter | Type | Description |
|---|---|---|
| `P_SA` | `SERVICEID` | Service provider ID (mandatory) |
| `P_VERS` | `/HFQ/DE_KD_VERSION` | Version (optional; if initial, all versions are shown) |
| `P_BANK` | Checkbox | Show bank data from `/HFQ/PTCONTBK` |
| `P_EDI` | Checkbox | Show EDI data (`SERVICEID`, `VERSION`, `EDI_EMAIL`, `DOWNLOAD_LINK`) from `/HFQ/PTCOMV` |

**Behavior (verified from implementation):**
- If `P_SA` is not provided, writes `'Bitte Serviceanbieter angeben'` and returns.
- Queries `/HFQ/PTCONTBK` and/or `/HFQ/PTCOMV` filtered by service provider and optionally by version.
- Each activated checkbox triggers a separate `CL_SALV_TABLE` ALV display with:
  - All functions enabled, optimized column widths, striped pattern
  - Title "Bankdaten" or "EDIFAKT-Daten" respectively
