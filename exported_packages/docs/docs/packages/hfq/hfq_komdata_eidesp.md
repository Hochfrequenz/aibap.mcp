# Package: HFQ / KOMDATA_EIDESP

**Description**: Erweiterungsprojekt für Serviceanbieter-Dynpro (Enhancement project for service provider screen)
**Original language**: German (D)
**Number of objects**: 3 (1 function group with 1 screen/dynpro, 1 customer modification project, 1 package definition)

## Executive Summary

This package implements a SAP screen enhancement (via a Customer Modification / CMOD project named `/HFQ/KOM`) that adds a sub-screen (Dynpro 1110) for displaying PARTIN communication data at a service provider. The sub-screen shows version and validity information, URL, fax, tax ID, court of registration, commercial register number, and embeds three ALV custom controls for availability data, contacts, and bank data. The package was authored by Hochfrequenz Unternehmensberatung GmbH in December 2021. The function group `/HFQ/FG_KOMDATA_EIDESP` hosts the screen and its PBO/PAI flow logic.

---

## Function Groups

### `/HFQ/FG_KOMDATA_EIDESP`

Contains no exported function modules — it serves exclusively as a container for Dynpro 1110 and its associated PBO/PAI modules.

**Includes referenced in top include:**
- `/HFQ/LFG_KOMDATA_EIDESPTOP` — Global declarations (not in export, exists on system)
- `/HFQ/LFG_KOMDATA_EIDESPUXX` — Function module include
- `/HFQ/LFG_KOMDATA_EIDESPO01` — PBO module include (explicitly included twice in the top include, with one commented out — likely a merge artifact)

**Dynpro 1110** ("Subscreen für Kontaktdaten Servicepartner"):

- **Type**: Subscreen (`TYPE=I`)
- **Size**: 50 lines × 120 columns
- **Language**: German

**Screen fields (verified from dynpro XML):**

| Field name | Type | Screen label | Notes |
|---|---|---|---|
| `GS_HEADER-VERSION` | NUMC (output) | Version | Right-justified, 9 characters |
| `GV_VERSION_VALIDITY` | CHAR (output only) | — | 45-char validity text next to version |
| `GS_HEADER-URL` | CHAR (scrollable output) | Website | 255 chars, visible 52 |
| `GS_HEADER-FAX` | CHAR (scrollable output) | Fax-Nummer | 70 chars, visible 52 |
| `GS_HEADER-TAX_ID` | NUMC (scrollable output) | Umsatzsteuer-ID | 70 chars, visible 52 |
| `GS_HEADER-TAX_NUM` | CHAR (scrollable output) | Steuernummer | 70 chars, visible 52 |
| `GS_HEADER-COURT` | CHAR (scrollable output) | Gericht | 100 chars, visible 52 |
| `GS_HEADER-COMMERCIAL_REG_NUM` | CHAR (scrollable output) | Handelsregister-Nr. | 100 chars, visible 52 |
| `GS_HEADER-EDI_EMAIL` | CHAR (scrollable output, invisible) | Datenaustausch | Hidden |
| `GS_HEADER-DOWNLOAD_LINK` | CHAR (scrollable output, invisible) | E-Mail-Zertifikat | Hidden |

**Custom controls (ALV containers):**

| Control name | Position | Size | Purpose |
|---|---|---|---|
| `CONT_ALV_AVAILABILITY` | Row 1, Col 78 | 32×8 | Availability display |
| `CONT_ALV_CONTACTS` | Row 10, Col 2 | 108×16 | Contact data ALV |
| `CONT_ALV_BANK_DATA` | Row 27, Col 2 | 108×16 | Bank data ALV |

**Flow logic (verified from `screen_1110.abap`):**
```
PROCESS BEFORE OUTPUT.
  MODULE create_gui_objects.
  MODULE fill_data.
PROCESS AFTER INPUT.
  MODULE read_version.
```

The actual module implementations (`create_gui_objects`, `fill_data`, `read_version`) are in the PBO/PAI include files which are not part of this abapGit export.

---

## Other Objects

### Customer Modification `/HFQ/KOM` (CMOD)

A SAP customer enhancement project named `/HFQ/KOM`. The XML shows the project is defined but contains no enhancement assignments (no `MODATR`/`MODINC` entries). *Inferred: the CMOD project wraps the exit/BAdI that calls dynpro 1110, but the actual enhancement assignments are not included in this abapGit export.*
