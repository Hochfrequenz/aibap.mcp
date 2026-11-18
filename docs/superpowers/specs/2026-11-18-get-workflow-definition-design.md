# `get_workflow_definition` — read SAP Business Workflow definitions (#148)

## Intent

An agent analysing an SAP Business Workflow (a `WS…` template built in the Workflow Builder,
SWDD) currently needs about a dozen hand-written `run_query` calls plus manual joins to answer
"what happens after this step, which branch does value X take, which method is called, where does
the loop exit". Typical trigger: ticket analysis — why a workflow hangs or takes a branch.

`get_workflow_definition` returns one workflow definition as a readable graph in a single call,
with the decoding that today has to be done by hand: active version, branch outcomes (yes/no,
case values), container operations as assignments, step tasks, sub-workflow calls.

It is an **agent tool**: it composes `SELECT` statements through the existing
`adt.QueryClient.RunQuery` and assembles the graph in aibap.mcp. No adtler change, no new ADT
endpoint.

### Why not OData

Checked on both integration systems (SAP ERP 6.0 EHP8 and SAP S/4HANA on-premise):
`API_WORKFLOW_DESIGN_SRV`, `API_WORKFLOW_RUNTIME_SRV`, `API_WORKFLOW_TASK_SRV` and
`SWF_FLEX_WF_MANAGE_SRV` do not exist as backend services (`TADIR` object `IWSV`), so nothing can be
activated in `/IWFND/MAINT_SERVICE`. The 403s reported in #148 are most likely Gateway's
`/IWFND/MED/170` "No service found". The only design-time workflow service, `SWF_FLEX_DEF_SRV`,
covers flexible workflows (`Scenarios`, `Workflows`, …), not SWDD templates.

## Goals

- Read one workflow definition by ID (`WS` + 8 digits), defaulting to the version the runtime uses.
- Return nodes and edges with human-readable labels; helper nodes of the block model collapsed.
- Decode: condition expressions, branch outcomes, multiple-condition values, container operations,
  process-control commands, fork "n of m", step task + agent expression, event names.
- Follow sub-workflow calls (`FUNC` nodes whose task is a `WS…`) up to a requested depth, flagging
  definitions that cannot be found.
- **Constants:** resolve `&<ELEMENT>.<CONSTANT>&` references (conditions, container operations,
  bindings) to their literal value when the container element is typed with an ABAP class or
  interface, e.g. `&CONSTANTS.GC_ABAP_TRUE&` → `'X'`.
- **Container:** return the workflow container (elements with type, description, import / export /
  mandatory flags, initial value) and, per step, the data-flow bindings (workflow → step and back).
- **Client-dependent data** (logon client): read the runtime copy the client actually executes
  (`SWD_*`, `EXETYP = 'E'`), legacy definitions that exist only in `SWD_*`, and customer task
  pointers in `HRP1205`.
- Optional Mermaid rendering.
- Integration test against an SAP-delivered workflow present on both integration systems.

## Non-goals (v1)

- Modifying workflow definitions. ADT offers no write path; writing tables is out of scope.
- Cross-workflow search ("which workflow evaluates field X / calls method Y"). Planned as a separate
  tool (`search_workflow_definitions`) in its own PR once this one has landed.
- Structured constants (`CONSTANTS: BEGIN OF …`), constants typed by reference to other constants,
  and BOR-object attributes. Returned verbatim, marked unresolved.
- Workflow configuration (`EXETYP = 'C'`/`'A'`, client-specific overrides of agents and deadlines):
  two rows on each integration system, not worth a decoder yet. The tool reports that a
  configuration exists.
- Reading another client than the logon client. Data preview runs in the logon client; the result
  says that client-dependent parts reflect the logon client.
- Runtime data: work items (`SWWWIHEAD`…), instances, logs. That is business/runtime data and stays
  outside the scope guardrail.

## Anti-goals

- Output that leaks SAP logon IDs: `SWDSCONDEF` change-info rows carry a user name and timestamp;
  `SWDSHEADER`/`HRS1205` carry `CREATED_BY`/`UNAME`. None of these columns is selected or returned.
- A graph that "looks complete" but silently dropped nodes because a query hit its row limit. Every
  query must detect truncation and fail loudly rather than return a partial graph.
- A tool that becomes a generic table reader. It only ever queries the fixed table list below
  (`SWDS*`, `SWD_*`, `HRS1205`/`HRP1205`, `HRS1000`/`HRP1000`, `SEOCOMPODF`, `SEOMETAREL`) with
  a validated `WS\d{8}` ID or with class/constant names taken from the workflow's own container
  and expressions.

## Data model (verified on both systems, SAP code where noted)

All `SWDS*` tables are client-independent and keyed by `WFD_ID`, `VERSION`, `EXETYP`
(`EXETYP = 'S'`, definition) and — except header/lines — `NODEID`.

| Concern | Source | Notes |
|---|---|---|
| Active version | `HRS1205` (SAP tasks, client-independent; columns `OTYPE='WS'`, `OBJID`, `WFD_ID`, `VERSION`, `ACTIVE`), else `HRP1205` (customer tasks, client-dependent) | This is the pointer the runtime uses (`SWD_CHECK_ACTIVE_VERSION`). `SWDSHEADER.ACTIV` is **not** reliable — `WS00400012` has `X` on `0001` and `9999`, while `HRS1205` points to `0003` on the ECC system and `0004` on the S/4 system. See "Active version rule" below. |
| Fallback version | highest `SWDSHEADER.VERSION` ≠ `9999` | Same rule as `SWD_GET_LATEST_WF_VERSION`: `9999` is the transport version and is discarded. |
| Header | `SWDSHEADER` | `LANGUAGE` (master language), `ACTIVAT_ON`, `CHANGED_ON`. |
| Nodes | `SWDSMNODES` | 4-char `NODETYPE`, `TASK`, `BLOCKID`/`PAR_BLCKID`/`NEST_LEVEL`, `CONT_CMD`, `RETURN`, `RETURNCODE`, `EXC_CLSNAM`, `MIN_TOKENS`/`EXP_TOKENS`, `EVT_NAME`, `EVT_OTYPE`, `CASE_CONTE`. `PARAMETER1–4` are empty for container operations. |
| Edges | `SWDSMLINES` | `PRED_NODE` → `SUCC_NODE`; no outcome code on the edge. |
| Texts | `SWDSTEXT` | `TEXTTYP = 'ND'` node name, `TI` work-item title, `MD` decision outcome text. Contains rows for node IDs that do not exist in the version — ignore them. Languages mixed. |
| Conditions | `SWDSCONDEF` | Line `000` is a header (`LINE_TYP = 'E'`, `001EXP`). Expression lines: `LEFTEXPR COMPOP RIGHTEXPR`, joined by `LOGOP`, `NOTFLAG`. A change-info row (release + user + 14-digit timestamp, empty `COMPOP`) may share a `LINE_ID` with an expression — drop it. `RIGHTEXPR` may carry a `{TYPE=…}` suffix — strip it. `COMPOP` `EX`/`NX` = "is filled"/"is initial". |
| Expressions | `SWDSEXPR` (`EXPRTYPE`, domain `SWD_EXPTYP`) | `031` result, `032` operand 1, `033` operand 2 of a container operation; `035` multiple-condition expression; `013` agent; `003`–`010` deadlines. `037`–`040` appear on almost every node — exclude in SQL. |
| Task texts | `HRS1000` / `HRP1000` (`STEXT`) | Short text of `TS…`/`WS…` tasks for step labels when the node has no `ND` text. |

### Constants

Container element types live in `SWDSCNTIDX` (plain columns, no XML): `OTYPE`/`OBJID` (owner,
`WS` = workflow container), `ELEMENT`, `TYPEID`, `CATID` (`CL` = ABAP class/interface, `BO` = BOR
object), `TYPEKIND` (domain `SWFDTKIND`), `TYPENAME` (DDIC type).

Resolution of `&E.C&`, verified on the SAP-delivered IS-U workflow `WS00288801` on the ECC system
(element `CONSTANTS`, `CATID = 'CL'`, class `/IDXGC/CL_PD_WF_DOCUMENT`; the constant is not on the
class but on the interface `/IDXGC/IF_PD_WF_CONSTANTS` it implements):

1. `SWDSCNTIDX` row for element `E` with `CATID = 'CL'` → class/interface name.
2. `SEOCOMPODF` with `CLSNAME` = that name, `CMPNAME = C`, `VERSION = '1'`, `ATTDECLTYP = '2'`
   (constant) → `ATTVALUE` (ABAP literal, e.g. `'X'`, `'WFSND'`).
3. Not found or `ATTVALUE` empty → follow `SEOMETAREL` (`VERSION = '1'`; `RELTYPE = '1'`
   implemented interface, `'2'` superclass) breadth-first, depth-limited, and repeat step 2.
4. Still nothing → leave the expression verbatim and add it to `unresolved_constants`.

Output keeps both forms: `"condition": "&LV_FLAG& = &CONSTANTS.GC_ABAP_TRUE&"` and
`"condition_resolved": "&LV_FLAG& = 'X'"`, so the agent sees where the value came from. Same for
operation and binding expressions.

No SAP-delivered workflow outside industry solutions uses class constants on either system
(checked: conditions referencing a `CATID = 'CL'` element). The S/4 system has none at all.

### Container and bindings

| Concern | Source | Notes |
|---|---|---|
| Element list + type | `SWDSCNTIDX`, `OTYPE = 'WS'` | Always present. |
| Description, import/export/mandatory, initial value | `SWDSCONT`, `NODEID = 0`, `CNT_TYPE = ''` (workflow container) | `DATA` is LRAW: data preview only returns it when `LENGTH` is selected too; it comes back hex-encoded, 1024 bytes per `TABIX`. Concatenate in `TABIX` order, trim trailing blanks, parse as asXML of `CL_SWF_CNT_CONTAINER`: one `SWFDELMDEFXML` per element with `NAME`, `TYPE` (`<objtype>:<catid>:<ddic>:<kind>:<len>:<dec>`, e.g. `FORMABSENC:BO::u:0:0`, `::WFSYST-RESULT:C:255:0`), `PROPS` (hex flag word), `VALUE`, `LTEXTS`. `CHGDTA` carries the last changer's logon ID — never returned. |
| Step bindings | `SWDSBINDEF`, `BINDTYPE = 'WW'` (workflow → work item) | Plain columns. `DATAFLOW` (domain `SWA_DAFLOW`): `E` export workflow → step, `I` import step → workflow; `ELEMENT` = element in the step/task container, `EXPR` = workflow-side expression. Other `BINDTYPE`s (agent `WR`, event `EW`/`WE`, …) returned with their raw type. |

*Builder call, to verify:* the bit meaning of `PROPS` (import / export / mandatory / multiline).
Determine it from `CL_SWF_CNT_ELEMENT` (or its constants) on the system and pin it in a unit test
with the three real values from `WS48900002`; if it cannot be determined, return `PROPS` raw and no
flags rather than guessing.

### Client-dependent data (`SWD_*`)

Verified on both systems (logon client):

| `SWD_HEADER.EXETYP` | Content | Count (≈, both systems) |
|---|---|---|
| `E` | runtime copy, created/refreshed when a workflow starts | ~620 rows / ~590 tasks |
| `S` | legacy definitions (1995/96); ~20 tasks exist **only** here | ~90 rows |
| `C` | configuration | 2 |

Two traps:

- **Key by `TASK`, not `WFD_ID`.** Legacy definitions use numeric `WFD_ID`s (`0040000064` for task
  `WS00400012`). `SWD_GET_LATEST_WF_VERSION` also selects by `TASK`. Even `SWDSHEADER` has two rows
  where `WFD_ID <> TASK`. All header lookups go through `TASK`; node/line/text queries then use the
  `WFD_ID` found.
- **Different node model.** The runtime copy and legacy definitions fill `SWD_NODES`/`SWD_LINES`
  (1-char `NODETYPE`: `S` step, `F` fork, `J` join, `I` condition, `C` multiple condition,
  `P` container operation, `O` process control, `X`/`Y` block, `R` wait for event; outcome on the
  edge in `SWD_LINES.RETURNCODE`, e.g. `8995`/`8994`), **not** `SWD_MNODES`/`SWD_MLINES`. For
  `WS48900002` version `0000`, the runtime copy on the ECC system has 26 `SWD_NODES`, 31
  `SWD_LINES` and 0 `SWD_MNODES`. So the tool needs a second decoder for this model; it emits the
  same output shape. Conditions, expressions and texts come from `SWD_CONDEF`, `SWD_EXPR`,
  `SWD_TEXT` with the same layout as their `SWDS*` counterparts plus `CLIENT`.

`8995`/`8994` and other `RETURNCODE`s: *builder call, to verify* against the same workflow's
`SWDS*` version, where the outcome is known from `IFRS.RETURN` — map only codes that are confirmed
this way, keep others raw.

Runtime copy vs. pointer: the runtime copy can lag the activation pointer until the next start
(`WS48900002`: pointer `0001`, runtime copy `0000` on the ECC system; `WS20000263`: pointer `0007`,
runtime copy `0003` on the S/4 system). The tool reports both versions whenever both exist.

### Active version rule

Verified on the ECC system: `HRS1205` holds one row per task for most tasks, but ~20 old tasks have
one row per version (`0000`, `0001`, …), with exactly one row flagged `ACTIVE = 'X'`. One task
(`WS60000520`) has its active row on version `9999` — so `9999` is a legitimate result when the
pointer says so.

1. `version` given → use it (`version_source = "requested"`).
2. `HRS1205` rows for `OTYPE = 'WS'`, `OBJID` = the 8 digits, `ACTIVE = 'X'`. One row → use it.
   Several → highest `VERSION`, plus a warning listing all of them.
3. No `HRS1205` row → `HRP1205` with `OTYPE = 'WS'`, `OBJID`, `ACTIVE = 'X'` and
   `BEGDA <= today <= ENDDA`, any `PLVAR`. Several → highest `VERSION` plus a warning.
   *Builder call, unverified:* `HRP1205` is empty in the integration clients of both systems
   (no customer workflow tasks there), so this branch is covered by unit tests only. Reading the
   active plan variant from `T77S0` is deliberately not done to keep the table list fixed.
4. Neither → highest `SWDSHEADER.VERSION` ≠ `9999` for that `TASK`
   (`version_source = "latest_numbered"`) and a warning that no activation pointer was found.
5. The chosen version must exist in `SWDSHEADER`. If the task has **no** `SWDSHEADER` row at all,
   fall back to the legacy definition in `SWD_HEADER` (`EXETYP = 'S'`, by `TASK`, highest version,
   `version_source = "legacy_client"`) and decode it with the `SWD_NODES` decoder. Only if that is
   empty too → "not found", naming every table looked in.
6. Independently, look up the runtime copy (`SWD_HEADER`, `EXETYP = 'E'`, by `TASK`) and report its
   version as `runtime_version` (empty if none).

Sub-workflows are resolved with the same rule, each at its own active version.

### Block model and node types

Every step is a block: `FUNC` → `XOR` → outcome nodes (`EXCP`, `TEVT`, `IFRS`, `CRES`, …) → `ECAS`
(merge). Admin nodes `999xxx` (`EFUN`, `EVTG`, `OFF`, `END`) close the workflow.

Outcome decoding:

| Outcome node | Meaning |
|---|---|
| `IFRS` (condition), `UNRS` (loop "until") | `RETURN = '0001'` → condition true, `'0000'` → false |
| `CRES` | `RETURNCODE` = case value; `COTH` = "others" (`_SAP_WFSYST_RC_OTHERS`) |
| `DRES` | user decision outcome; label from `SWDSTEXT` `MD`/`ND` |
| `EXCP` | `EXC_CLSNAM` set → exception exit, otherwise "step completed" |
| `TEVT` | terminating event (`EVT_NAME`) |
| `CEVT`, `EEVT`, `EVOI`, `WEVT`, `IEVT`, … | single outcome of a non-step node ("done") |

`CONT_CMD` (domain `SWD_CONTRL`): `12` `:=`, `18` append, `11`/`13`/`14`/`15` `+ - * /`; on `CTRL`
nodes `01` cancel work item (node in `PARAMETER1`), `02` cancel workflow, `04` set step obsolete,
`07` complete workflow. Unknown codes are returned as `cmd <nn>` — never guessed.

## Approaches considered

1. **Agent tool in aibap.mcp** (`tools/businessworkflow.go`) on top of `adt.QueryClient`. Fast, no
   release cycle in adtler; logic only usable through the MCP. **Chosen** — it is pure composition
   of `SELECT`s, nothing in the ADT HTTP/XML layer changes. Move to adtler only if a second Go
   consumer appears.
2. Logic in adtler (`adt/workflow`, like `custexport`), thin wrapper here. Reusable, but two PRs and
   an adtler release before anything is usable.
3. Approach 1 plus cross-workflow search in the same PR. Rejected for scope: search is a separate
   tool and PR.

## Tool surface

Name: `get_workflow_definition`. ("Workflow" alone collides with the development-workflow guide
`sap-adt://guides/workflow`; the description states explicitly that this is SAP Business Workflow,
SWDD.) Read-only, idempotent annotations. Registered in the `objects` tool group.

Inputs:

| Param | Type | Default | Notes |
|---|---|---|---|
| `wfd_id` | string, required | — | `WS` + 8 digits; also accepts the 8 digits alone. Validated before any query. |
| `version` | string | active version | 4 digits. `9999` allowed explicitly. |
| `source` | string | `definition` | `definition`: client-independent `SWDS*` (legacy `SWD_*` fallback, see version rule). `runtime`: the runtime copy in the logon client (`SWD_*`, `EXETYP = 'E'`) — what running instances in this client actually execute; error if none exists. |
| `container` | boolean | `true` | Include the workflow container and per-step bindings. |
| `depth` | number | `0` | Sub-workflow levels to resolve. Capped (e.g. 5). Cycles and repeats are resolved once. |
| `language` | string | `E` | Text language; fallback to the workflow's master language, then any language present. |
| `raw` | boolean | `false` | `true` returns all nodes/edges un-collapsed. |
| `mermaid` | boolean | `false` | Adds a Mermaid `flowchart` string. |

Output (`WorkflowDefinitionResult`, named structs in `tools/results.go`, declared via
`WithOutputSchema`):

```
{
  "wfd_id": "WS48900002",
  "version": "0001",
  "version_source": "HRS1205" | "HRP1205" | "latest_numbered" | "legacy_client" | "requested" | "runtime",
  "source_tables": "SWDS" | "SWD",
  "runtime_version": "0000",
  "has_configuration": false,
  "language": "E",
  "title": "...",
  "activated_on": "2002-01-22",
  "nodes": [ { "id": "36", "type": "IF", "kind": "condition",
               "label": "Already Revised Once?",
               "condition": "&COUNTER& > 1",
               "condition_resolved": "&COUNTER& > 1",
               "task": "", "agent": "", "operation": "", "operation_resolved": "", "event": "",
               "bindings": [ { "direction": "export", "step_element": "_WI_OBJECT_ID",
                               "expression": "&ABSENCFORM&", "expression_resolved": "&ABSENCFORM&",
                               "bind_type": "WW" } ],
               "block_id": "36", "parent_block_id": "23" } ],
  "edges": [ { "from": "36", "to": "42", "label": "true" } ],
  "container": [ { "name": "ABSENCFORM", "description": "...", "type": "FORMABSENC",
                   "category": "BO", "type_kind": "u", "ddic_type": "",
                   "import": true, "export": false, "mandatory": false,
                   "initial_value": "" } ],
  "unresolved_constants": [ "&CONSTANTS.GC_STRUCTURED-FIELD&" ],
  "calls": [ { "node": "18", "wfd_id": "WS48900024" } ],
  "warnings": [ "..." ],
  "mermaid": "flowchart TD ...",
  "subworkflows": [ { "wfd_id": "WS48900024", "found": true,
                      "depth": 1, "called_by": ["WS48900002"],
                      "definition": { …same shape as the root, without "subworkflows"… } } ]
}
```

*Builder call:* sub-workflows are a **flat list on the root only**, keyed by `wfd_id`, not nested.
Every definition (root and sub) lists its own outgoing `calls`; the root's `subworkflows` holds
each distinct called workflow once, so repeats and cycles need no special shape — a cycle simply
references a `wfd_id` already in the list. This keeps the `outputSchema` non-recursive (one level:
root definition + list of definitions). Calls beyond `depth` appear in `calls` but not in
`subworkflows`. A called workflow that is not found gets an entry with `found: false`, no
`definition`, and a warning. With `mermaid: true`, each definition carries its own chart.

Node `id`s drop leading zeros. `kind` is a normalised vocabulary (`start`, `end`, `step`,
`subworkflow`, `condition`, `multiple_condition`, `loop_until`, `loop_while`, `fork`, `join`,
`container_operation`, `process_control`, `wait_event`, `trigger_event`, `user_decision`,
`mail`, `other`); `type` keeps the raw `NODETYPE` so nothing is lost.

Collapsing (default): `XOR`, `ECAS`, `ENDI`, `UNJO`, outcome nodes and admin nodes are removed; an
edge from a step to the node after the merge carries the outcome label (`true`/`false`, case value,
`others`, exception class, event name, decision text). Single unlabeled "done" outcomes produce an
unlabeled edge. A fork's `join` node stays, labelled `n of m`.

Loops (*builder call*, shape taken from `WS48900002` and `WS00400012`):

- **Until** (`UNJO` entry … body … `UNTL` → `XOR` → `UNRS`): `UNJO` is collapsed; edges into it
  point to the first body node. The `UNTL` node stays (`kind = loop_until`, `condition` from
  `SWDSCONDEF`) and gets two edges: `true` → the node after the loop (exit), `false` → the first
  body node (back-edge, repeat). The `UNRS` `RETURN` codes decide which is which.
- **While** (`WHJO` entry, `WHIL` → `WHRS`/`WOTH` → body … back to `WHJO`, `EWHI` exit): `WHJO` is
  collapsed into `WHIL` (`kind = loop_while`), which gets `true` → first body node and `false` →
  node after the loop; the body's last node gets an edge back to `WHIL`. The builder verifies the
  exact `WHRS`/`WOTH` → true/false mapping on `WS00400012` before coding it and records it in a
  test fixture.
- `raw: true` keeps every helper node, so the decoded shape can always be checked against the
  original.

## Constraints

- Each query goes through `RunQuery` with an explicit `max_rows` well above any real workflow
  (e.g. 5000). If `TotalRows` exceeds the returned rows **or** the returned row count equals
  `max_rows`, the tool fails with a clear error (anti-goal: no silent partial graphs). The second
  check is needed because it is unsettled whether ADT data preview reports the uncapped total —
  adtler uses the same double check for its `E071` query.
- SQL lines stay under 255 characters (adtler re-wraps, but a long `IN (...)` list of sub-workflow
  IDs must be built with whitespace).
- Errors: `errorResult(err), nil`, never a typed DTO on the error path (#354). Success via
  `mcp.NewToolResultJSON` with an object (#351).
- Unit tests first (TDD) with a fake `QueryClient` that answers per table; fixture rows modelled on
  the real `WS48900002` data.
- README tool table and `TestReadmeToolNamesMatchRegistered`; `structured_content_shape_test.go`
  covers the new tool automatically.
- Versioning: new tool → minor bump at release.
- Public repo: no system aliases, hosts or logon IDs in code, tests, docs or PR text.

## Integration test

`tools/businessworkflow_integration_test.go` (`//go:build integration`), run against every system
in `MCP_INTEGRATION_SYSTEMS`. Target: SAP-delivered `WS48900002` (BPML test workflow "notification
of absence"), identical on both systems, active version `0001` via `HRS1205`. Asserts:

- `version = "0001"`, `version_source = "HRS1205"`.
- Condition node with `&COUNTER& > 1` and outgoing edges labelled `true` and `false`.
- Loop "until" with condition `&FLAG& = X`.
- Multiple condition on `&RESULT&` with branches `A`, `R`, `others`.
- Container operation `counter := &counter& + 1` and `flag := X`.
- Process control "complete workflow".
- Fork with join `2 of 2`.
- With `depth = 1`: sub-workflow `WS48900024` present and `found = true`.
- Container: elements `ABSENCFORM` (BOR `FORMABSENC`), `COUNTER`, `FLAG`, `RESULT`; binding of
  step 18 exports `&ABSENCFORM&` to `FORMABSENC` and imports `RESULT` into `&RESULT&`.
- No SAP user name from the condition change-info rows or the container `CHGDTA` appears anywhere
  in the JSON.
- `source = "runtime"`: if the logon client has a runtime copy, it decodes to a non-empty graph via
  the `SWD_NODES` decoder and `version_source = "runtime"`; if not, the call returns a "no runtime
  copy" error. Both outcomes are asserted explicitly per branch (runtime copies appear and disappear
  with workflow starts, so the test cannot pin which one).
- Legacy fallback: a task that exists only in `SWD_HEADER` (`EXETYP = 'S'`), found at test time by
  query, decodes with `version_source = "legacy_client"`. The test fails loudly — not skips — if no
  such task exists.
- Constants: SAP-delivered IS-U workflow `WS00288801` (active version `0002`; container element
  `CONSTANTS` typed with `/IDXGC/CL_PD_WF_DOCUMENT`, constants defined on the implemented interface
  `/IDXGC/IF_PD_WF_CONSTANTS`). Asserts `&CONSTANTS.GC_ABAP_TRUE&` → `'X'` and
  `&CONSTANTS.GC_PROC_VAL_UTILMD&` → `'UTILMD'` in `condition_resolved`. The test checks for the
  workflow first (`SWDSHEADER` by `TASK`) and calls `t.Skip` with an explicit message
  ("WS00288801 not present — system has no IS-U common layer; constant resolution via workflow not
  verified on <system>") on systems without it. Constant resolution is additionally covered by unit
  tests with fixture rows from this workflow.

## Left to the builder

- Exact SQL per table and whether texts/expressions are fetched per workflow or batched across the
  sub-workflow set.
- Mermaid styling and escaping.
- The precise list of node types per `kind`, beyond those listed above (51 types exist; unknown ones
  map to `other` with the raw `type`).
- Wording of warnings (orphan text rows ignored, unknown `CONT_CMD`, sub-workflow not found, etc.).
- Exact asXML parsing of `SWDSCONT` (namespaces, heap references).
- Mapping of the 1-char `SWD_NODES` types to `kind`.
