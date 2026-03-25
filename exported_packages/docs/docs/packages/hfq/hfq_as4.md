# Package: HFQ / AS4

**Description**: Versand der AS4 Daten (Sending of AS4 data)
**Original language**: German (D)
**Number of objects**: 7 (1 report, 1 function group with 2 function modules, 1 message class, 1 table type, 1 transaction, 1 package definition)

## Executive Summary

This package implements AS4 (ebMS 3.0 / AS4 profile) communication for the German energy market PARTIN message format. It provides both mass-send (Massenversand) and single-send (Einzelversand) dispatch of AS4 communication data to market partners via the SAP GC/IDX framework (`/IDXGC/`). The main report `/HFQ/RP_AS4_SND` (accessible via transaction `/HFQ/AS4`) presents a selection screen for certificate issuer, certificate subject, version information, sender service provider, reference date, and optional receiver filtering. Two function modules perform the actual process triggering by building process data structures and calling `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS`. The package was authored by Hochfrequenz Unternehmensberatung GmbH in July 2023.

---

## Function Groups

### `/HFQ/FG_AS4`

Contains two function modules.

#### `/HFQ/TRIG_AS4_MAS` — Mass send trigger

**Importing:**

| Parameter | Type | Description |
|---|---|---|
| `IV_ISSUER` | `TEXT100` | Certificate issuer (Zertifikatsaussteller) — placed in FTX segment with qualifier Z24 |
| `IV_SUBJECT` | `TEXT100` | Certificate subject/user (Zertifikatsnutzer) — placed in FTX segment with qualifier Z23 |
| `IV_VERSION_OLD` | `/HFQ/DE_KD_VERSION` | Predecessor version (optional; validated to be strictly less than `IV_VERSION`) |
| `IV_VERSION` | `/HFQ/DE_KD_VERSION` | Current version |
| `IV_SERVICE_SEN` | `SERVICE_PROV` | Sender service provider ID |
| `IV_DATE` | `DATUM` | Reference date (used as REF message date with qualifier AGK = current version) |
| `IT_RECEIVER` | `/HFQ/TT_SERVICEID` | Table of receiver service provider IDs (each becomes a MARPA entry with `party_func_qual = 'REC'`) |

**Exporting:**
- `ES_PROC_STEP_ALL TYPE /IDXGC/S_PROC_STEP_DATA_ALL`

**Exception**: `/HFQ/CX_PARTIN_ERROR`

**Behavior (verified from implementation):**
1. Reads process configuration for process ID `GC_PROCID_KOMDAT_MAS` via `/IDXGC/CL_CUST_ACCESS`.
2. Looks up the BMID from `/IDXGC/AMID_CONF` for AMID `GC_AMID_37007` (valid at `SY-DATUM`).
3. Builds outbound process step data with direction `GC_IDOC_DIRECTION_OUTBOUND`.
4. Attaches FTX comment segments for issuer (Z24) and subject (Z23).
5. Adds version reference with qualifier AGK; if `IV_VERSION_OLD` is supplied, adds a second reference with qualifier ACW and validates `IV_VERSION_OLD < IV_VERSION`.
6. Adds receiver entries as MARPA partners with `party_func_qual = 'REC'`.
7. Starts the process via `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS` (no PDOC display).

#### `/HFQ/FB_AS4_SND` — Single-send step function

**Importing:**

| Parameter | Type | Description |
|---|---|---|
| `IS_PROCESS_STEP_ALL` | `/IDXGC/S_PROC_STEP_DATA_ALL` | Existing process step data (reference) |
| `IV_SERVICE_REC` | `SERVICE_PROV` | Receiver service provider |
| `IV_SERVICE_SEN` | `SERVICE_PROV` | Sender service provider |

**Exporting:**
- `ES_PROC_STEP_ALL TYPE /IDXGC/S_PROC_STEP_DATA_ALL`

**Behavior (verified from implementation):**
1. Reads process configuration for `GC_PROCID_KOMDAT_SND`.
2. Looks up BMID for AMID `GC_AMID_37007`.
3. Builds a new outbound process step (direction = 2), setting `OWN_SERVPROV = IV_SERVICE_SEN` and `ASSOC_SERVPROV = IV_SERVICE_REC`.
4. Clears step-specific fields (step number, ref, timestamp, status, MARPA).
5. Triggers via `/IDXGC/CL_PROCESS_TRIGGER=>START_PROCESS` followed by `COMMIT WORK AND WAIT`.
6. Returns the resulting process step data.

---

## Reports

### `/HFQ/RP_AS4_SND`

**Transaction**: `/HFQ/AS4` ("Senden der AS4-Kommunikationsdaten")

**Selection screen blocks:**

- Block `ZER` (Zertifikate / Certificates): `P_ZERAUS` (issuer), `P_ZERNU` (subject), `P_VRSN` (version, mandatory), `P_VRVRSN` (predecessor version, optional), `P_VRSNDR` (sender service provider, mandatory), `P_DATUM` (date, mandatory)
- Block `REC` (Empfänger / Receivers): `SO_RECVR` (select-options for `ESERVPROV-SERVICEID`, single values only — `NO INTERVALS`)

**Behavior (verified from implementation):**
1. Validates that `P_VRSNDR` exists in `ESERVPROV` with `OWN_LOG_SYS = 'X'`. If not found, issues message `S000(/HFQ/AS4)` and returns.
2. If receivers are specified, validates all listed service provider IDs exist in `ESERVPROV`. Lists missing ones and returns if any are not found.
3. Calls `/HFQ/TRIG_AS4_MAS` with the selection screen values.
4. On success, issues message `S007(/HFQ/AS4)` ("Versenden abgeschlossen.").

---

## Other Objects

### Message class `/HFQ/AS4`

**Description**: Nachrichtenklasse für AS4 Versand

| Message number | Text |
|---|---|
| 000 | Serviceanbieter &1 nicht gefunden |
| 001 | Unerwartete BMID &1 |
| 002 | Die Daten &1 sind unvollständig |
| 003 | &1 ist kein gültiger Z-Code |
| 004 | Die neue Version &1 muss mindestens 1 größer sein, als die Alte &2 |
| 005 | Folgende Serviceanbieter konnten nicht gefunden werden: |
| 006 | Bitte warten. Versandprozess wird verarbeitet. |
| 007 | Versenden abgeschlossen. |
| 008 | Aktuelle Versionsnummer konnte nicht gefunden werden |

### Table type `/HFQ/TT_SERVICEID`

Standard table of `SERVICEID` (CHAR 10), default key, no uniqueness constraint. Used to pass receiver lists.
