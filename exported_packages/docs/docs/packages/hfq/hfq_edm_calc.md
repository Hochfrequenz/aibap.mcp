# Package: HFQ / EDM_CALC

**Description**: Entwicklungspaket für Berechnungsformeln (Development package for calculation formulae)
**Original language**: German (D)
**Number of objects**: 5 (1 class, 1 exception class, 1 function group with 1 function module, 1 message class, 1 package definition)

## Executive Summary

This package implements a Standard Load Profile (SLP) calculation formula for gas consumption ("SLP Gas TUM") within the SAP EDM (Energy Data Management) framework. The central class `/HFQ/EDM_CL_CALC_SLP_GAS_TUM` computes hourly gas usage based on outdoor temperature, day-of-week factors, holiday calendar, and a sigmoid + linear demand model sourced from a profile definition (`/US4G/` namespace). The function module `/HFQ/EDM_CALC_SLP_GAS_TUM` acts as the EDM formula entry point, iterating over time-series input and invoking the class. An exception class `/HFQ/EDM_CX_CALC` wraps calculation errors with T100 message support. The implementation integrates with the `/US4G/` (utility SAP4Gas) framework for loading profile parameters.

---

## Classes

### `/HFQ/EDM_CL_CALC_SLP_GAS_TUM`

**Instantiation**: `CREATE PROTECTED` (use `GET_INSTANCE` factory method)

**Public methods:**

| Method | Signature | Description |
|---|---|---|
| `CALCULATE` | `IMPORTING IV_TEMPERATURE TYPE PROFVAL, IV_HOLIDAY_CALENDAR TYPE HIDENT, IV_DATE_UTC TYPE DATS, IV_TIME_UTC TYPE TIMS` / `EXPORTING EV_USAGE TYPE PROFVAL, EV_TO_DATE_UTC TYPE E_EDMDATETO, EV_TO_TIME_UTC TYPE E_EDMTIMETO` / `RAISING /HFQ/EDM_CX_CALC` | Calculates hourly gas usage for a given UTC timestamp and temperature. Converts UTC to local time, determines day type (weekday/holiday/Sunday), applies sigmoid and linear formula components, multiplies by weekday factor, and divides by 24 to produce an hourly value. |
| `GET_INSTANCE` (class method) | `IMPORTING IV_PFDEF_INT_ID TYPE /US4G/DE_PFDEF_INT_ID` / `RETURNING RO_INSTANCE TYPE REF TO /HFQ/EDM_CL_CALC_SLP_GAS_TUM` / `RAISING /HFQ/EDM_CX_CALC` | Singleton factory per profile definition ID. Returns a cached instance or creates a new one. |

**Protected methods:**

| Method | Signature | Description |
|---|---|---|
| `CONSTRUCTOR` | `IMPORTING IV_PFDEF_INT_ID TYPE /US4G/DE_PFDEF_INT_ID` / `RAISING /HFQ/EDM_CX_CALC` | Loads profile definition data from `/US4G/CL_EDM_BA_FACTORY`, sorts by `DATEFROM DESCENDING`. |
| `CONVERT_UTC_TO_LOC` | `IMPORTING IV_DATE_UTC TYPE DATS, IV_TIME_UTC TYPE TIMS` / `EXPORTING EV_TIME_LOC TYPE TIMS, EV_DATE_LOC TYPE DATS` / `RAISING /HFQ/EDM_CX_CALC` | Converts a UTC date/time to local system time using `CL_ABAP_TSTMP`. |

**Calculation formula (verified from `CALCULATE` implementation):**

1. UTC timestamp is converted to local time.
2. If local time is before 06:00, the gas day ends at 06:00 the same day; otherwise it ends at 06:00 the next day.
3. Profile parameters are loaded for the applicable date (`datefrom <= lv_date_loc`).
4. Weekday is determined via `DAY_IN_WEEK`; holidays via `HOLIDAY_CHECK_AND_GET_INFO`.
5. Sigmoid component: `A / (1 + B / (T - V0))^C + D`
6. Linear components: `mH * T + bH` (heating demand) and `mW * T + bW` (warm water demand); the larger of the two is added to the sigmoid.
7. Final hourly usage: `(sigmoid + max(linear)) * weekday_factor / 24`

**Known issue noted in code comment**: Weekday factor assignment had a bug where all days were treated as Monday. This was partially corrected; the `CASE` statement now assigns per-day factors, but there is a redundant dead code block (commented out) indicating the fix was in progress.

**Private state:**

- `GT_INSTANCES` (class-level sorted table, unique key `PFDEF_INT_ID`) — instance cache
- `MT_PFDEF_GA` (`/US4G/T_PFDEF_GA`) — profile definition parameters sorted descending by `DATEFROM`
- `MV_PFDEF_INT_ID` — identity of this instance's profile

---

### `/HFQ/EDM_CX_CALC`

**Inherits from**: `CX_STATIC_CHECK`
**Interfaces**: `IF_T100_MESSAGE`, `IF_T100_DYN_MSG`

A static-check exception class that carries a T100 message key. If no `TEXTID` is supplied at construction, it defaults to `IF_T100_MESSAGE=>DEFAULT_TEXTID`.

---

## Function Groups

### `/HFQ/EDM_CALC_GAS`

Contains one function module:

#### `/HFQ/EDM_CALC_SLP_GAS_TUM`

**Interface (CHANGING):**
- `XY_CNTR TYPE EEDMFORMULACTR` — EDM formula control structure (reference)
- `XY_INP TYPE TEEDMFORMPARLIST_I` — Input parameter list (reference)
- `XY_OUT TYPE TEEDMFORMPARLIST_O` — Output parameter list (reference)

**Exception**: `GENERAL_FAULT`

**Description**: EDM formula entry point for the SLP Gas TUM calculation. Uses EDM macro infrastructure (`edm_formula_init`, `edm_def_par_input`, `edm_def_par_output`, `edm_formula_check`, `edm_reset_index`, `edm_read_input`, `edm_append_output`, `edm_next_index`, `edm_check_all_values`) to iterate over the time-series. On each interval:
- Reads inputs: temperature (param 1), SLP-type/profile ID (param 2), holiday calendar ID (param 3)
- Calls `GET_INSTANCE` / `CALCULATE` on `/HFQ/EDM_CL_CALC_SLP_GAS_TUM` only when temperature, type, or validity period has changed (caches last result)
- Writes computed hourly usage to output param 1

---

## Other Objects

### Message class `/HFQ/EDM_CALC`

The message class exists (master language German) but contains no message texts in the exported source — the XML shows no `T100_TEXTS` entries. Message `e001` referenced in the class implementation (for missing profile definition) is not defined in this export. *Inferred: messages may exist in the system but were not exported, or the class was created as a placeholder.*
