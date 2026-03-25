# Package: HFQ / DXP_DATA_UBD

**Description**: UBD Logiken (UBD logic)
**Original language**: German (D)
**Number of objects**: 5 (1 class, 2 internal table/structure definitions, 2 data elements, 1 package definition)

## Executive Summary

This package implements a data provider class for fetching customer name and address data from the UBD (Unbundling Data / Kundendaten) layer of the `/US4G/` (utility SAP4Gas) framework, as part of the DXP (Data Exchange Platform) architecture. The class `/HFQ/DXP_CL_DP_UBD_CUSTOMER` extends the base class `/HFQ/DXP_CL_DP_SERIALIZED` and, given a transaction ID from the DXP event store, queries the UBD address interface for a delivery point (POD), then maps the result into a flat customer structure. Two data elements and two structure definitions provide the type model for customer identity and address data.

---

## Classes

### `/HFQ/DXP_CL_DP_UBD_CUSTOMER`

**Inherits from**: `/HFQ/DXP_CL_DP_SERIALIZED`
**Instantiation**: `CREATE PUBLIC`

**Public types:**

```abap
TYPES: BEGIN OF ty_s_data.
  INCLUDE TYPE /hfq/dxp_s_dp_customer.
TYPES END OF ty_s_data.
```

**Public methods:**

| Method | Signature | Description |
|---|---|---|
| `CONSTRUCTOR` | `RAISING /HFQ/DXP_CX_GENERAL_ERROR` | Calls `SUPER->CONSTRUCTOR()`. Sets `MV_STRUCT_NAME` to `'<CLASSNAME>=>TY_S_DATA'` for serialization purposes. |

**Protected methods:**

| Method | Signature | Description |
|---|---|---|
| `MAPPING` | `IMPORTING IS_UBD_DATA TYPE /US4G/S_UBD_DATA` / `RETURNING RS_DXP_DATA TYPE TY_S_DATA` | Maps UBD name/address data to the DXP customer structure. Sets `CUSTOMER_ID` from `BU_PARTNER_ID`, constructs `CUSTOMER_NAME` as `FIRST_NAME_COMP_NAME2 || ' ' || LAST_NAME_COMP_NAME1`, maps full address (street, house number, postal code, city, district, country), and preserves the original UBD request structure. |
| `DETERMINE_DATA_CHILD` | Redefinition | Retrieves event data by `IV_TRANSACTION_ID` from `/HFQ/DXP_CL_DB_EVE=>GET_DATA`. Uses `/APE/CL_BASIS_FACTORY` to get the UBD data object factory, then queries the UBD name/address interface (`/US4G/IF_UBD_NM_ADDR`) for the POD derived from `LS_EVENT_DATA-EXTERNAL_ID` with category `'NaAdrPoD'`, using `LS_EVENT_DATA-KEY_DATE` as the validity date. Assigns result via `MAPPING()`. |

---

## Tables / Data Definitions

### `/HFQ/DXP_S_DP_CUSTOMER` (internal structure, `TABCLASS=INTTAB`)

**Description** (German): Datenfelder zur Datenbereitstellung von Kundendaten

| Field | Type/Role name | Description |
|---|---|---|
| `EXTERNAL_CUSTOMER_ID` | `/HFQ/DXP_E_EXT_CUSTOMER_ID` | External customer ID (dummy, CHAR 50) |
| `CUSTOMER_ID` | `BU_PARTNER` | SAP Business Partner number |
| `CUSTOMER_NAME` | `/HFQ/DXP_E_CUSTOMER_NAME` | Combined first and last name (CHAR 140) |
| `ACADEMIC_TITLE` | `/US4G/DE_ACADEMIC_TITLE` | Academic title |
| `FIRST_NAME` | `/US4G/DE_FIRSTNAME_COMPANY_NAM` | First name / company name 2 |
| `LAST_NAME` | `/US4G/DE_LASTNAME_COMPANY_NAME` | Last name / company name 1 |
| `CUSTOMER_ADDRESS` | `/HFQ/DXP_S_DP_CUSTOMER_ADDRESS` | Nested address structure |
| `REQUEST` | `/US4G/S_UBD_ADR_REQ` | Original UBD address request (for traceability) |

### `/HFQ/DXP_S_DP_CUSTOMER_ADDRESS` (internal structure, `TABCLASS=INTTAB`)

**Description** (German): Datenfelder zur Datenbereitstellung von Kundenadressdaten

| Field | Type/Role name | Description |
|---|---|---|
| `STREET` | `/US4G/DE_STREET` | Street name |
| `HOUSE_NUMBER` | `/US4G/DE_HOUSE_NUMBER` | House number |
| `POSTAL_CODE` | `/US4G/DE_POSTALCODE` | Postal code |
| `CITY` | `/US4G/DE_CITY1` | City |
| `DISTRICT` | `/US4G/DE_DISTRICT` | District |
| `COUNTRY_CODE` | `/US4G/DE_COUNTRYCODE_EXT` | Country code (external) |

---

## Domains and Data Elements

### `/HFQ/DXP_E_CUSTOMER_NAME`
- Data type: `CHAR`, length 140
- Label: "Vor- und Nachname des Kunden" (customer first and last name)

### `/HFQ/DXP_E_EXT_CUSTOMER_ID`
- Data type: `CHAR`, length 50
- Label: "Externe Kundennummer des Endpunkts (Dummy)" — explicitly marked as a dummy/placeholder external customer ID
