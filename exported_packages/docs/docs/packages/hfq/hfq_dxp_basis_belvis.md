# Package: HFQ / DXP_BASIS_BELVIS

**Description**: Ergänzungspaket zur belvis Lösung (Supplementary package for the belvis solution)
**Original language**: English (E)
**Number of objects**: 3 (2 domain appends, 1 package definition)

## Executive Summary

This is a small supplementary package that adds two domain append objects to the shared DXP event source domain `/HFQ/DXP_D_EVENT_SOURCE`. The appends register `CustCreate`, `CustChange`, and `CuAdChange` as valid event source values for the belvis integration. "belvis" is a meter data management and billing system used by German energy utilities; these fixed values indicate the types of customer and customer-address events that can originate from belvis. *Inferred from package name, domain names, and fixed values. Not verified against integration specification.*

---

## Domains and Data Elements

### `/HFQ/DXP_AD_CUSTOMER` (domain append)

**Appends to**: `/HFQ/DXP_D_EVENT_SOURCE`
**Description**: Festwerte für Kundendaten (fixed values for customer data)

| Value | Inferred meaning |
|---|---|
| `CustCreate` | Customer creation event from belvis |
| `CustChange` | Customer change event from belvis |

### `/HFQ/DXP_AD_CUSTOMER_ADDRESS` (domain append)

**Appends to**: `/HFQ/DXP_D_EVENT_SOURCE`
**Description**: Festwerte für Kundenadresse (fixed values for customer address)

| Value | Inferred meaning |
|---|---|
| `CuAdChange` | Customer address change event from belvis |

Both domain appends use `VALEXI=X`, meaning fixed values are enforced (only the listed values are valid). *Inferred from ABAP domain semantics.*
