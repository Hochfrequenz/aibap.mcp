# Package: HFQ / EDM

**Description**: Hauptpaket für EDM Entwicklungen (Main package for EDM developments)
**Original language**: German (D)
**Number of objects**: 2 (the top-level package itself, and a sub-package `HFQ/EDM/calc`)

## Executive Summary

This is a structural parent package for EDM (Energy Data Management / Energiedatenmanagement) developments. All actual source objects live in the child package `HFQ/EDM/calc` (sub-directory `src/calc/`). The top-level package contains no directly assigned objects; it serves only as a namespace grouping container. See the `HFQ/EDM_CALC` documentation for the full content description, as this package and `HFQ/EDM_CALC` contain identical source objects.

---

## Sub-packages

| Sub-package | Description |
|---|---|
| HFQ/EDM/calc | Entwicklungspaket für Berechnungsformeln (Development package for calculation formulae) |

The `calc` sub-package contains:
- One function group: `/HFQ/EDM_CALC_GAS`
- Two classes: `/HFQ/EDM_CL_CALC_SLP_GAS_TUM`, `/HFQ/EDM_CX_CALC`
- One message class: `/HFQ/EDM_CALC`

Refer to the [HFQ/EDM_CALC documentation](hfq_edm_calc.md) for the complete description of all objects in this sub-package, as the source files are identical.
