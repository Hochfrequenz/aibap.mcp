# Package: HFQ / NATUVION

**Description**: /HFQ/NATUVION (package short text equals the package name; no separate description in metadata)
**Original language**: German (D)
**Number of objects**: 14 (1 namespace, 11 transparent tables, 1 package definition, 1 `.abapgit.xml`)

## Executive Summary

This package contains a complete replica of the classic SAP ABAP flight-data model (SFLIGHT, SCARR, SPFLI, SCUSTOM, SBOOK, etc.) under the `/HFQ/` namespace prefix. All tables carry names beginning with `/HFQ/S…`, mirroring the well-known SAP training data model structure. The purpose is almost certainly to support Natuvion migration projects or training exercises that require a customer-namespace copy of the standard flight demo data. *Inferred from file names, table structures, and the package name referencing the consultancy "Natuvion". Not verified against implementation.*

---

## Tables / Data Definitions

All tables are transparent (`TABCLASS=TRANSP`) and client-dependent (`CLIDEP=X`).

| Table name | Short description | Key fields |
|---|---|---|
| `/HFQ/SAIRPORT` | Airports (Flughäfen) | `MANDT`, `ID` |
| `/HFQ/SAPLANE` | Aircraft types (Flugzeug) | `MANDT`, `PLANETYPE` |
| `/HFQ/SBOOK` | Flight bookings | `MANDT`, `UUID_ROOT` — uses UUID as primary key instead of the classic composite key |
| `/HFQ/SBUSPART` | Airline business partners (Fluggeschäftspartner) | `MANDANT`, `BUSPARTNUM` |
| `/HFQ/SCARPLAN` | Plane-airline assignment (Fluggesellschaft-Flugzeug-Zuordnung) | `MANDT`, `CARRID`, `PLANETYPE` |
| `/HFQ/SCARR` | Airlines | `MANDT`, `CARRID` |
| `/HFQ/SCITAIRP` | City-airport assignment (Stadt-Flughafen-Zuordnung) | `MANDANT`, `CITY`, `COUNTRY`, `AIRPORT` |
| `/HFQ/SCUSTOM` | Flight customers (Flugkunden) | `MANDT`, `ID` — includes a fuzzy-search index (`FUZ`) on `NAME` |
| `/HFQ/SFLIGHT` | Flight table | `CLIENT`, `UUID_ROOT` — also uses UUID as primary key |
| `/HFQ/SNVOICE` | Invoices (Flugrechnung) | `MANDT`, `CARRID`, `CONNID`, `FLDATE`, `BOOKID`, `CUSTOMID`, `INSTNO` |
| `/HFQ/SPFLI` | Flight schedule (Flugplan) | `MANDT`, `CARRID`, `CONNID` — has several secondary indices and full-text indices |
| `/HFQ/STICKET` | Flight ticket (Flugticket) | `MANDT`, `CARRID`, `CONNID`, `FLDATE`, `BOOKID`, `CUSTOMID`, `TICKET` |
| `/HFQ/STRAVELAG` | Travel agencies | `MANDT`, `AGENCYNUM`, `BUKRS` |

**Notable differences from standard SAP SFLIGHT model:**
- `/HFQ/SBOOK` and `/HFQ/SFLIGHT` use `SYSUUID_X16` (`UUID_ROOT`) as their primary key instead of the classic `CARRID`/`CONNID`/`FLDATE`/`BOOKID` composite. This is a modern design deviation.
- `/HFQ/STICKET` checks against `/HFQ/SCARR` and `/HFQ/SPFLI` (namespace copies), but `/HFQ/SNVOICE` still references the standard `SCARR`, `SPFLI`, `SFLIGHT`, and `SCUSTOM` check tables (not the `/HFQ/` namespace copies).
- `/HFQ/SCUSTOM` has a fuzzy full-text search index defined on `NAME`.
- `/HFQ/SPFLI` has five indices (001, A, ARP, FT1, FT2), including two full-text indices for departure town and destination searches.
