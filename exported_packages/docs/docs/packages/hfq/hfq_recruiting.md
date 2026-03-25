# Package: HFQ / RECRUITING

**Description**: Bewerbercases (Applicant cases)
**Original language**: English (E) — `SRV_CHECK=X` flag is set, indicating transport-layer server checks are active
**Number of objects**: 6 (1 generic template report + 4 per-applicant copies + 1 package definition + 1 namespace)

## Executive Summary

This package contains ABAP exercise programs used in Hochfrequenz's recruiting process. There is one generic template report (`/HFQ/REC_CASE_00_GENERIC`) and four identical copies named after specific applicants: Hackspacher, Maehrlein, Neuland, and Proelss. Each copy is given to a candidate as their individual workspace to implement solutions to four coding exercises. The `SRV_CHECK=X` flag in the package definition indicates the package was set up with transport server checks enabled. *Inferred from the pattern of identical programs with candidate-specific names, the header comment "Übungen zum Recruiting", and the placeholder text (XXX, YYY, AAA, CCC) that candidates must fill in.*

---

## Reports

All five programs (`/HFQ/REC_CASE_00_GENERIC`, `/HFQ/REC_CASE_HACKSPACHER`, `/HFQ/REC_CASE_MAEHRLEIN`, `/HFQ/REC_CASE_NEULAND`, `/HFQ/REC_CASE_PROELSS`) are byte-for-byte identical in logic. They differ only in the `REPORT` statement name.

### Selection screen

| Parameter | Type | Description |
|---|---|---|
| `P_GEBDAT` | `DATS` | Birthdate input |
| `P_ZAHL` | `I` | Integer input |
| `P_U1`–`P_U4` | Radio buttons (group `G1`) | Exercise selector |

A constant `C_ZUSE TYPE DATS VALUE '19100622'` represents the birth date of Konrad Zuse (22 June 1910), founder of modern computing. *Inferred from the date; not stated explicitly in the code.*

### Exercises (to be completed by the applicant)

| Exercise | Radio button | Description |
|---|---|---|
| Übung 1 | `P_U1` | Compares the birth year from `P_GEBDAT` to Konrad Zuse's birth year. Prints `'XXX'` or `'YYY'` (placeholders to be replaced). |
| Übung 2 | `P_U2` | Calculates the applicant's age in years from `P_GEBDAT`, accounting for whether the birthday has passed this year. Prints `'AAA'` and the computed age (placeholder to be replaced). |
| Übung 3 | `P_U3` | Counts the number of days from `P_GEBDAT` to today using a `DO` loop that decrements a date by 1 each iteration. Prints `'CCC'` and the count (placeholder to be replaced). |
| Übung 4 | `P_U4` | Calls a recursive subroutine `MAIN` that computes `P_ZAHL!` (factorial). Prints `'XXX'` (number) and `'YYY'` (result) (placeholders to be replaced). |

The programs are intentionally left incomplete — applicants are expected to add comments, replace placeholder strings with meaningful output, and demonstrate understanding of ABAP date arithmetic and recursion.
