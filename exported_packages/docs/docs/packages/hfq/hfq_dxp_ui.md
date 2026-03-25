# Package: HFQ / DXP_UI

**Package description:** UI Frontend
**Namespace:** `/HFQ/`
**Master language:** English
**abapGit folder logic:** PREFIX

## Executive Summary

`HFQ/DXP_UI` is the Fiori/OData UI layer for the DXP (Data Exchange Platform) event monitoring use case. It exposes a single RAP Business Object — the DXP Event — through a full ABAP RESTful Application Programming Model (RAP) stack: a root CDS view entity backed by table `/HFQ/DXP_T_EVE`, a projection view with search and metadata-extension annotations, a managed behavior definition with create/update/delete, a service definition, and an OData V2 service binding published for Fiori consumption.

The package contains no custom business logic. The behavior handler class `/HFQ/BP_DXP_R_EVENT` is a skeleton with an empty `get_instance_authorizations` handler, and the projection behavior simply delegates all standard operations (`create`, `update`, `delete`) to the root.

The resulting OData V2 service `DXP_UI_EVENT_O2` (version 0001) is what a Fiori app or SAP Gateway consumer would call. Authorization for the service is checked via the `S_SERVICE` authorization object (check confirmed active, exception state "Okay").

---

## CDS Views

### `/HFQ/DXP_R_EVENT` — DXP Event (Data Model)

| Property | Value |
|---|---|
| Kind | `root view entity` |
| Source | `/HFQ/DXP_T_EVE` (database table) |
| Authorization check | `#NOT_REQUIRED` |
| File | `#hfq#dxp_r_event.ddls.asddls` |

Root view entity that reads directly from the persistence table. Exposes all columns with CameLCase aliases and decorates administrative fields with standard RAP semantics annotations.

**Fields:**

| CDS Field | DB Column | Semantics Annotation |
|---|---|---|
| `TransactionId` | `transaction_id` | Key; managed numbering (readonly) |
| `ObjectType` | `object_type` | — |
| `ObjectKey` | `object_key` | — |
| `Scenario` | `scenario` | — |
| `Timestamp` | `timestamp` | — |
| `Status` | `status` | — |
| `ReturnCode` | `return_code` | — |
| `ReturnMessage` | `return_message` | — |
| `ChangedAt` | `changed_at` | `@Semantics.systemDate.lastChangedAt` |
| `ChangedBy` | `changed_by` | `@Semantics.user.lastChangedBy` |
| `CreatedAt` | `created_at` | `@Semantics.systemDate.createdAt` |
| `CreatedBy` | `created_by` | `@Semantics.user.createdBy` |

---

### `/HFQ/DXP_C_EVENT` — DXP Event (Projection)

| Property | Value |
|---|---|
| Kind | `root view entity` (projection) |
| Provider contract | `transactional_query` |
| Base view | `/HFQ/DXP_R_EVENT` |
| Search | `@Search.searchable: true`; `ReturnMessage` is `@Search.defaultSearchElement` |
| Metadata extensions | allowed (`@Metadata.allowExtensions: true`) |
| Propagated annotations | ignored (`@Metadata.ignorePropagatedAnnotations: true`) |
| File | `#hfq#dxp_c_event.ddls.asddls` |

Projects all fields from the root view without transformation. Enables full-text search on `ReturnMessage`.

---

### `/HFQ/DXP_C_EVENT` — Metadata Extension (DDLX)

| Property | Value |
|---|---|
| Metadata layer | `#CORE` |
| Description | DXP Event Metadata |
| File | `#hfq#dxp_c_event.ddlx.asddlxs` |

Defines UI layout for the Fiori list report and object page:

- **Header:** type name "Transaction" / "Transactions"; title and description both bound to `TransactionId`.
- **Facet:** single `#IDENTIFICATION_REFERENCE` facet labelled "Transaction" at position 10.
- **Line item / selection field / identification** columns (all importance `#MEDIUM`):

| Position | Field |
|---|---|
| 10 | `TransactionId` |
| 15 | `Status` |
| 20 | `Timestamp` |
| 30 | `ObjectType` |
| 35 | `ObjectKey` |
| 40 | `Scenario` |

Administrative fields (`ReturnCode`, `ReturnMessage`, `ChangedAt`, `ChangedBy`, `CreatedAt`, `CreatedBy`) are present on the projection but carry no explicit UI annotations in this extension.

---

## Behavior Definitions

### `/HFQ/DXP_R_EVENT` — Root BDEF

| Property | Value |
|---|---|
| Kind | Managed, `strict(2)` |
| Implementation class | `/HFQ/BP_DXP_R_EVENT` |
| Persistent table | `/HFQ/DXP_T_EVE` |
| Lock | master |
| Authorization | master (instance) |
| File | `#hfq#dxp_r_event.bdef.asbdef` |

Supported standard operations: `create`, `update`, `delete`.
`TransactionId` is `readonly` and uses `numbering : managed` (system-generated key).

Field mapping is an explicit 1-to-1 mapping between CDS field names and database column names (snake_case → CamelCase).

---

### `/HFQ/DXP_C_EVENT` — Projection BDEF

| Property | Value |
|---|---|
| Kind | Projection, `strict(2)` |
| Entity alias | `Event` |
| File | `#hfq#dxp_c_event.bdef.asbdef` |

Delegates `use create`, `use update`, and `use delete` to the root BDEF. No additional actions or validations are defined.

---

## Classes

### `/HFQ/BP_DXP_R_EVENT` — Behavior Implementation

| Property | Value |
|---|---|
| Superclass | `cl_abap_behavior_handler` (via local class `lhc_DXP_R_EVENT`) |
| Visibility | `PUBLIC ABSTRACT FINAL` |
| Behavior definition | `/HFQ/DXP_R_EVENT` |
| Description | Behavior Implementation for /HFQ/DXP_R_EVENT |
| Category | `06` (*Inferred: behavior pool*) |
| Files | `#hfq#bp_dxp_r_event.clas.abap`, `#hfq#bp_dxp_r_event.clas.locals_imp.abap` |

The global class body (`clas.abap`) is empty — definition and implementation are stubs.

The local handler class `lhc_DXP_R_EVENT` (in `clas.locals_imp.abap`) inherits from `cl_abap_behavior_handler` and implements one method:

- **`get_instance_authorizations`** — registered handler for `FOR INSTANCE AUTHORIZATION` on entity `/HFQ/DXP_R_EVENT`. The method body is empty; no authorization restrictions are enforced at runtime.

---

## Service Definition and Binding

### `/HFQ/DXP_EVENT` — Service Definition (SRVD)

| Property | Value |
|---|---|
| Description | DXP Event Service Definition |
| Exposed entity | `/HFQ/DXP_C_EVENT` |
| File | `#hfq#dxp_event.srvd.srvdsrv` |

Single-entity service definition exposing the projection view. No aliasing applied.

---

### `/HFQ/DXP_UI_EVENT_O2` — OData V2 Service Binding (SRVB)

| Property | Value |
|---|---|
| Description | DXP Event OData V2 UI Service |
| Binding type | OData V2 |
| Service definition reference | `/HFQ/DXP_EVENT` |
| Service version | 0001 |
| Release state | `NOT_RELEASED` |
| Published | `true` |
| Contract | `C1` |
| File | `#hfq#dxp_ui_event_o2.srvb.xml` |

The service binding publishes the OData V2 endpoint. Release state is `NOT_RELEASED`, meaning it is active and consumable but not formally API-released.

---

### `/HFQ/DXP_UI_EVENT_O2` — OData V2 ICM Service (IWSV / IWMO)

| Property | Value |
|---|---|
| External name | `DXP_UI_EVENT_O2` |
| Version | 0001 |
| Description | DXP Event OData V2 UI Service |
| DPC class | `CL_SADL_RAP_EXPOSURE_DPC` (SAP standard RAP gateway data provider) |
| MPC class | `CL_SADL_GW_RAP_EXPOSURE_MPC` (SAP standard RAP gateway metadata provider) |
| Is SAP service | No |
| Files | `#hfq#dxp_ui_event_o2               0001.iwsv.xml`, `#hfq#dxp_ui_event_o2            0001.iwmo.xml` |

Both DPC and MPC are SAP-standard SADL/RAP exposure classes; there is no custom gateway implementation.

---

### `/HFQ/DXP_UI_EVENT_O2_VAN` — OData V2 Vocabulary Annotation (IWVB)

| Property | Value |
|---|---|
| Technical name | `/HFQ/DXP_UI_EVENT_O2_VAN` |
| Version | 0001 |
| Description | Generic Annotation Provider |
| Annotation provider class | `CL_SADL_GW_CDS_EXPOSURE_APC` (SAP standard) |
| Schema namespace | `DXP_UI_EVENT_O2` |
| Is main service | Yes |
| Service alias | SAP |
| File | `#hfq#dxp_ui_event_o2_van        0001.iwvb.xml` |

Vocabulary annotation bundle generated automatically from the CDS metadata extension.

---

## Other Objects

### `6A1DF44239B4CE93CB3166CEBA8F58` — Service Usage Stub (SUSH)

| Property | Value |
|---|---|
| Type | `HT` (hash-keyed usage stub) |
| Display name | `R3TR IWSV /HFQ/DXP_UI_EVENT_O2               0001` |
| Authorization object checked | `S_SERVICE` |
| Check active | Yes (`OKFLAG: X`) |
| Exception state | 3 = Okay |
| File | `6a1df44239b4ce93cb3166ceba8f58ht.sush.xml` |

Authorization usage stub linking the published OData service to `S_SERVICE` check. This ensures that the service start is subject to a standard SAP service-level authorization check. No custom authorization fields are defined beyond the standard object.

---

## Object Inventory

| File | Object type | Technical name |
|---|---|---|
| `package.devc.xml` | Package | `HFQ/DXP_UI` |
| `#hfq#.nspc.xml` | Namespace | `/HFQ/` |
| `#hfq#dxp_r_event.ddls.*` | CDS root view entity | `/HFQ/DXP_R_EVENT` |
| `#hfq#dxp_r_event.bdef.*` | Behavior definition (managed) | `/HFQ/DXP_R_EVENT` |
| `#hfq#bp_dxp_r_event.clas.*` | Behavior pool class | `/HFQ/BP_DXP_R_EVENT` |
| `#hfq#dxp_c_event.ddls.*` | CDS projection view | `/HFQ/DXP_C_EVENT` |
| `#hfq#dxp_c_event.ddlx.*` | CDS metadata extension | `/HFQ/DXP_C_EVENT` |
| `#hfq#dxp_c_event.bdef.*` | Behavior definition (projection) | `/HFQ/DXP_C_EVENT` |
| `#hfq#dxp_event.srvd.*` | Service definition | `/HFQ/DXP_EVENT` |
| `#hfq#dxp_ui_event_o2.srvb.xml` | Service binding (OData V2) | `/HFQ/DXP_UI_EVENT_O2` |
| `#hfq#dxp_ui_event_o2 … 0001.iwsv.xml` | ICM OData service registration | `/HFQ/DXP_UI_EVENT_O2` v0001 |
| `#hfq#dxp_ui_event_o2 … 0001.iwmo.xml` | OData model provider registration | `/HFQ/DXP_UI_EVENT_O2` v0001 |
| `#hfq#dxp_ui_event_o2_van … 0001.iwvb.xml` | Vocabulary annotation | `/HFQ/DXP_UI_EVENT_O2_VAN` v0001 |
| `6a1df44239b4ce93cb3166ceba8f58ht.sush.xml` | Service usage stub | `6A1DF44239B4CE93CB3166CEBA8F58` |
