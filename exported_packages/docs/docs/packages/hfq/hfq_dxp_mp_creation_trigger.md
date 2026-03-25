# Package: HFQ / DXP_MP_CREATION_TRIGGER

**Description**: Lösung zur auto. synchronen Marktpartner Anlage (Solution for automatic synchronous market partner creation)
**Original language**: German (D)
**Number of objects**: 5 (1 class, 1 internal structure, 2 domain appends, 1 package definition)

## Executive Summary

This package implements a DXP (Data Exchange Platform) data provider class for triggering automatic synchronous market partner (Marktpartner) creation in the German energy market. The class `/HFQ/DXP_CL_DP_MP_CREATION` reads a DXP event's object key (a DUNS number), hardcodes the external code list ID to `'293'` (a known energy-market code list), and constructs the own external ID by prepending `'7'` to the key (replacing the first character). Two domain append objects extend the shared DXP event source and object type domains with the `MarParSync` and `extMP` values respectively.

---

## Classes

### `/HFQ/DXP_CL_DP_MP_CREATION`

**Inherits from**: `/HFQ/DXP_CL_DP_SERIALIZED`
**Instantiation**: `CREATE PUBLIC`

**Public types:**

```abap
TYPES: BEGIN OF ty_s_data.
  INCLUDE TYPE /hfq/dxp_s_dp_mp_creation.
TYPES END OF ty_s_data.
```

**Public methods:**

| Method | Signature | Description |
|---|---|---|
| `CONSTRUCTOR` | `RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `SUPER->CONSTRUCTOR()`. Sets `MV_STRUCT_NAME` to `'<CLASSNAME>=>TY_S_DATA'` for serialization. |

**Protected methods:**

| Method | Signature | Description |
|---|---|---|
| `DETERMINE_DATA_CHILD` | Redefinition | Retrieves event data via `/HFQ/DXP_CL_DB_EVE=>GET_DATA(IV_TRANSACTION_ID)`. Assigns `EXTERNAL_ID` = `LS_EVENT_DATA-OBJECT_KEY`, `EXT_CODELISTID` = `'293'` (hardcoded), `OWN_EXTERNAL_ID` = `'7' && EXTERNAL_ID+1` (prepends '7', skips first character of the external ID). |

**Hardcoded values (verified from implementation):**
- `EXT_CODELISTID = '293'` — This is the BDEW code list ID 293, which corresponds to external market partner identification in the German energy market. *Inferred from context; not commented in the code.*
- The own external ID construction `'7' && <ls_data>-external_id+1` prefixes `7` and skips the first character of the DUNS number, which in German energy market practice creates a Marktpartner number (MP-ID starts with `'7'` for market partner type). *Inferred from domain knowledge. Not verified against business rule specification.*

---

## Tables / Data Definitions

### `/HFQ/DXP_S_DP_MP_CREATION` (internal structure, `TABCLASS=INTTAB`)

**Description** (German): Marktpartneranlagestruktur (market partner creation structure)

| Field | Type/Role name | Description |
|---|---|---|
| `EXTERNAL_ID` | `DUNSNR` | DUNS number (external market partner ID) |
| `EXT_CODELISTID` | `E_EDMIDEEXTCODELISTID` | External code list ID (set to '293') |
| `OWN_EXTERNAL_ID` | `DUNSNR` | Own/internal market partner ID (derived from EXTERNAL_ID) |

---

## Domains and Data Elements

### `/HFQ/DXP_AD_ES_MAR_PAR_CREATE` (domain append)

**Appends to**: `/HFQ/DXP_D_EVENT_SOURCE`
**Description**: Festwert für Marktpartnererstellung (fixed value for market partner creation)

| Value | Description |
|---|---|
| `MarParSync` | Event source identifier for synchronous market partner creation |

### `/HFQ/DXP_AD_OBJ_TYPE_MAR_PAR` (domain append)

**Appends to**: `/HFQ/DXP_D_OBJECT_TYPE`
**Description**: Append für Marktpartner Objekt Typ (append for market partner object type)

| Value | Description |
|---|---|
| `extMP` | Object type for external market partner |
