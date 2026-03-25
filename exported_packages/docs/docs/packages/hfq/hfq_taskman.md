# Package: HFQ / TASKMAN

**Description**: Taskmanager
**Original language**: German (D)
**Number of objects**: 39 files — 6 transparent tables, 5 CDS root view entities, 5 domains, 5 data elements, 1 namespace descriptor, 1 package descriptor, 1 abapgit config

---

## Executive Summary

TASKMAN is a pure data-layer package within the `/HFQ/` namespace. It defines the complete relational schema for a task management application: projects, tasks, comments, assignments, project members, and a change-history log. There are no classes, function groups, or reports — the package consists exclusively of transparent database tables, their corresponding CDS basic interface views, and the domain/data-element vocabulary used to type those tables.

All six tables are client-dependent (`CLIDEP = X`), transparent (`TABCLASS = TRANSP`), application-data category (`TABART = APPL1`), and have buffering disabled (`BUFALLOW = N`). Every table uses a GUID (`ROLLNAME GUID`) as its primary key; all GUIDs are represented as string types but carry no declared foreign key constraints within the ABAP dictionary — referential integrity between tables is *Inferred from field naming conventions. Not verified against implementation.*

---

## CDS Views

All five views are **root view entities** (`define root view entity`) with `@AccessControl.authorizationCheck: #NOT_REQUIRED` and `@Metadata.ignorePropagatedAnnotations: true`. Each is a thin, non-joining projection over exactly one transparent table — there are no associations, compositions, or scalar functions.

### `/HFQ/I_PROJECTS`

| Property | Value |
|---|---|
| Label | Projects |
| Base table | `/HFQ/TM_PROJECTS` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `Name`, `Description`, `Status`

Note: `CREATED_AT`, `UPDATED_AT`, and `CREATED_BY` exist on the underlying table but are **not projected** into this view. *Inferred from field omission. Not verified against implementation.*

---

### `/HFQ/I_TASKS`

| Property | Value |
|---|---|
| Label | Tasks |
| Base table | `/HFQ/TM_TASKS` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `ProjectGuid`, `Title`, `Description`, `Status`, `Priority`, `AssigendTo`

Note: `CREATED_BY`, `DUE_DATE`, `CREATED_AT`, and `UPDATED_AT` exist on the underlying table but are **not projected** into this view. The field name `AssigendTo` (missing second 'n') mirrors the spelling in the table column `ASSIGEND_TO`.

---

### `/HFQ/I_COMMENT`

| Property | Value |
|---|---|
| Label | Comment |
| Base table | `/HFQ/TM_COMMENTS` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `TaskGuid`, `Userid`, `Content`, `CreatedAt`, `UpdatedAt`

---

### `/HFQ/I_ASSIGMENT`

| Property | Value |
|---|---|
| Label | Assigment (sic) |
| Base table | `/HFQ/TMASSIGMENT` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `TaskGuid`, `Userid`, `AssigendAt`

Note: both the view name and the label contain the consistent misspelling "Assigment" (missing 'n'). The field `AssigendAt` reflects the column name `ASSIGEND_AT` on the table.

---

### `/HFQ/I_MEMBERS`

| Property | Value |
|---|---|
| Label | Members |
| Base table | `/HFQ/TM_MEMBERS` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `ProjectGuid`, `Userid`, `Role`

Note: `ASSIGEND_AT` (timestamp) exists on the underlying table but is **not projected** into this view.

---

### `/HFQ/I_HISTORY`

| Property | Value |
|---|---|
| Label | History |
| Base table | `/HFQ/TM_HISTORY` |
| Key field | `Guid` |

**Exposed fields**: `Guid`, `ProjectGuid`, `Userid`, `Actiontype`, `Entitytype`, `EntityGuid`, `OldValue`, `NewValue`, `CreatedAt`

---

## Tables / Data Definitions

All tables use `CONTFLAG = A` (application data) and `EXCLASS = 1` (smallest enhancement category — no modification). Text descriptions are in German only (`LANGU = D`).

### `/HFQ/TM_PROJECTS` — TM_Projects

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | Project identifier |
| `NAME` | `CHAR(20)` | | Project name |
| `DESCRIPTION` | `CHAR(200)` | | Project description |
| `STATUS` | `/HFQ/TM_PR_STAT` | | Project status (domain: 01/02/03) |
| `CREATED_AT` | `TIMESTAMP` | | |
| `UPDATED_AT` | `TIMESTAMP` | | |
| `CREATED_BY` | `UNAME` | | |

---

### `/HFQ/TM_TASKS` — TM_Tasks

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | Task identifier |
| `PROJECT_GUID` | `GUID` | | Foreign reference to project |
| `TITLE` | `CHAR(200)` | | |
| `DESCRIPTION` | `STRG` | | Unbounded string |
| `STATUS` | `/HFQ/TM_TK_STAT` | | Task status (domain: 01/02/03) |
| `PRIORITY` | `/HFQ/TM_TK_PRIO` | | Priority (domain: H/M/L) |
| `ASSIGEND_TO` | `UNAME` | | Assigned user (typo in column name) |
| `CREATED_BY` | `UNAME` | | |
| `DUE_DATE` | `TIMESTAMP` | | |
| `CREATED_AT` | `TIMESTAMP` | | |
| `UPDATED_AT` | `TIMESTAMP` | | |

---

### `/HFQ/TM_COMMENTS` — Task Comments

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | Comment identifier |
| `TASK_GUID` | `GUID` | | Foreign reference to task |
| `USERID` | `UNAME` | | Author |
| `CONTENT` | `STRG` | | Unbounded string |
| `CREATED_AT` | `TIMESTAMP` | | |
| `UPDATED_AT` | `TIMESTAMP` | | |

---

### `/HFQ/TMASSIGMENT` — Task Assigment (sic)

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | Assignment identifier |
| `TASK_GUID` | `GUID` | | Foreign reference to task |
| `USERID` | `UNAME` | | Assigned user |
| `ASSIGEND_AT` | `TIMESTAMP` | | Assignment timestamp (typo in column name) |

Note: Table name `/HFQ/TMASSIGMENT` is missing the namespace separator `TM_` seen in all other tables, and continues the "Assigment" misspelling.

---

### `/HFQ/TM_MEMBERS` — Projects Members

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | Membership record identifier |
| `PROJECT_GUID` | `GUID` | | Foreign reference to project |
| `USERID` | `UNAME` | | Member's user ID |
| `ROLE` | `/HFQ/TM_ROLES` | | Member role (domain: A/M) |
| `ASSIGEND_AT` | `TIMESTAMP` | | Membership timestamp (typo in column name) |

---

### `/HFQ/TM_HISTORY` — Tsak History (sic)

| Field | Type / Roll | Key | Notes |
|---|---|---|---|
| `MANDT` | `MANDT` | PK | Client |
| `GUID` | `GUID` | PK | History entry identifier |
| `PROJECT_GUID` | `GUID` | | Owning project |
| `USERID` | `UNAME` | | Acting user |
| `ACTIONTYPE` | `/HFQ/TM_ACTIONTYPE` | | Action performed (domain: C/U/D/M) |
| `ENTITYTYPE` | `/HFQ/TM_ENTITYTYPE` | | Type of entity changed (domain: P/T/M) |
| `ENTITY_GUID` | `GUID` | | GUID of the changed entity |
| `OLD_VALUE` | `STRG` | | Previous value, unbounded string |
| `NEW_VALUE` | `STRG` | | New value, unbounded string |
| `CREATED_AT` | `TIMESTAMP` | | |

Note: the German short text in the XML is "Tsak History" — a transposition typo for "Task History".

---

## Domains and Data Elements

All domain/data-element pairs are mastered in German (`DTELMASTER = D`). Fixed-value enforcement is active on all domains (`VALEXI = X`).

### `/HFQ/TM_ACTIONTYPE`

| Property | Value |
|---|---|
| Domain base type | `CHAR(1)` |
| Domain label (DE) | ACTIONTYPE |
| Data element label (DE) | ACTIONTYPE |
| Allowed values | `C`, `U`, `D`, `M` |

*Value semantics inferred from naming convention: C = Create, U = Update, D = Delete, M = Member (or Move). Not verified against implementation.*

---

### `/HFQ/TM_ENTITYTYPE`

| Property | Value |
|---|---|
| Domain base type | `CHAR(1)` |
| Domain label (DE) | EntityTYPE |
| Data element label (DE) | EntityTYPE |
| Allowed values | `P`, `T`, `M` |

*Value semantics inferred from naming convention: P = Project, T = Task, M = Member. Not verified against implementation.*

---

### `/HFQ/TM_PR_STAT`

| Property | Value |
|---|---|
| Domain base type | `CHAR(2)` |
| Domain label (DE) | TM Prj. Status |
| Data element label (DE) | TM_Prj. Status |
| Allowed values | `01`, `02`, `03` |

*Value semantics (e.g., Active / Completed / Archived) are not defined in the domain XML — no descriptive texts per value are present. Not verified against implementation.*

---

### `/HFQ/TM_TK_STAT`

| Property | Value |
|---|---|
| Domain base type | `CHAR(2)` |
| Domain label (DE) | Task Status |
| Data element label (DE) | Task Status |
| Allowed values | `01`, `02`, `03` |

*Value semantics (e.g., Open / In Progress / Done) are not defined in the domain XML. Not verified against implementation.*

---

### `/HFQ/TM_TK_PRIO`

| Property | Value |
|---|---|
| Domain base type | `CHAR(1)` |
| Domain label (DE) | TK Prio |
| Data element label (DE) | Priority |
| Allowed values | `H`, `M`, `L` |

*Value semantics inferred from naming convention: H = High, M = Medium, L = Low. Not verified against implementation.*

---

### `/HFQ/TM_ROLES`

| Property | Value |
|---|---|
| Domain base type | `CHAR(1)` |
| Domain label (DE) | Roles |
| Data element label (DE) | Roles |
| Allowed values | `A`, `M` |

*Value semantics inferred from naming convention: A = Administrator (or Admin), M = Member. Not verified against implementation.*

---

## Other Objects

### `/HFQ/` Namespace (`#hfq#.nspc.xml`)

Namespace `/HFQ/` is registered with owner "Hochfrequenz". Two language slots are present (`<LANGU/>` empty entry and `D`); only the German text is populated.

---

## Known Data Quality Issues

The following spelling errors are present in the source code and must be preserved when referencing object names:

| Object | Issue |
|---|---|
| `/HFQ/TMASSIGMENT`, `/HFQ/I_ASSIGMENT` | "Assigment" — missing second 'n' (should be "Assignment") |
| Column `ASSIGEND_AT` (in `TMASSIGMENT`, `TM_MEMBERS`) | "Assigend" — should be "Assigned" |
| Column `ASSIGEND_TO` (in `TM_TASKS`) | Same typo |
| `/HFQ/TM_HISTORY` German short text | "Tsak History" — transposition of "Task" |
| `/HFQ/I_TASKS` — field `AssigendTo` | CDS alias inherits the column-name typo |
