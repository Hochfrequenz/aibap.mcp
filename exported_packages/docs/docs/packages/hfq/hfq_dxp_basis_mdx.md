# Package: HFQ / DXP_BASIS_MDX

**Description**: Paket für MDX Adapter (Package for MDX adapter)
**Original language**: German (D)
**Number of objects**: 4 (3 classes, 1 package definition)

## Executive Summary

This package contains stub implementations of three MDX (Master Data Exchange) adapter classes for the `/UCOM/` framework. All three classes implement framework interfaces but their method bodies are entirely empty — the implementation was started but not completed. The classes serve as integration points for change tracking, process handling, and trigger management within the MDX (Master Data Exchange) scenario of the DXP (Data Exchange Platform). *Inferred from interface names and class hierarchy. All method implementations verified as empty from source.*

---

## Classes

### `/HFQ/DXP_CL_MDX_CHGTRK_GEN`

**Implements**: `/UCOM/IF_MDX_TRACK_CHANGES`
**Instantiation**: `CREATE PUBLIC`

**Purpose**: Generic change tracker for MDX. Stores old and new data references plus change event metadata.

**Constructor:**

| Parameter | Type | Description |
|---|---|---|
| `IV_CHANGE_EVENT` | `/UCOM/DE_MDX_CHANGE_EVENT` | Change event identifier |
| `IV_CHANGE_DATE` | `DATUM` | Date of the change |
| `IR_OLD_DATA` | `REF TO DATA` | Reference to old data (deep copied internally) |
| `IR_NEW_DATA` | `REF TO DATA` | Reference to new data (deep copied internally) |
| `IV_RULE_SET` | `/UCOM/DE_MDX_RULE_SET` | MDX rule set |
| `IV_OBJECT_KEY` | `CHAR200` | Optional object key |
| `IV_PDOC_NR` | `/UCOM/DE_PROCESS_REF` | Optional process document number |
| `IV_PDOC_UUID` | `/UCOM/DE_APPL_DB_KEY` | Optional process document UUID |

`RAISING /UCOM/CX_MDX_ERROR`

The constructor deep-copies old and new data into instance attributes `MR_OLD_DATA` and `MR_NEW_DATA`.

**Interface methods (all empty stubs):**

| Method | Behavior |
|---|---|
| `/UCOM/IF_MDX_TRACK_CHANGES~CHECK_TRACKING` | Always returns `RV_ACTIVE = ABAP_TRUE` (hardcoded — only non-empty method) |
| `/UCOM/IF_MDX_TRACK_CHANGES~GET_TIME_SLICE_FIELDS` | Empty |
| `/UCOM/IF_MDX_TRACK_CHANGES~PREPARE_CHANGED_DATA` | Empty |
| `/UCOM/IF_MDX_TRACK_CHANGES~TRACK_CHANGES` | Empty |

---

### `/HFQ/DXP_CL_MDX_PROCESS`

**Implements**: `/UCOM/IF_MDX_PROCESS`
**Instantiation**: `CREATE PUBLIC`

**Interface methods (all empty stubs):**

| Method | Behavior |
|---|---|
| `/UCOM/IF_MDX_PROCESS~START` | Empty |
| `/UCOM/IF_MDX_PROCESS~START_WITHOUT_TRIGGER` | Empty |
| `/UCOM/IF_MDX_PROCESS~START_WITH_TRIGGER` | Empty |

---

### `/HFQ/DXP_CL_MDX_TRIGGER`

**Implements**: `/UCOM/IF_MDX_TRIGGER`
**Instantiation**: `CREATE PUBLIC`

**Interface methods (all empty stubs):**

| Method | Behavior |
|---|---|
| `/UCOM/IF_MDX_TRIGGER~CREATE_OUTBOUND_ENTRIES` | Empty |
| `/UCOM/IF_MDX_TRIGGER~FILL_BUFFER` | Empty |
| `/UCOM/IF_MDX_TRIGGER~GET_TRIGGERS` | Empty |
| `/UCOM/IF_MDX_TRIGGER~PROCESS_OUTBOUND_ENTRIES` | Empty |

---

All three classes are non-functional stubs. No business logic is implemented beyond `CHECK_TRACKING` returning `ABAP_TRUE`. *This package appears to be work-in-progress or a placeholder for a planned MDX integration.*
