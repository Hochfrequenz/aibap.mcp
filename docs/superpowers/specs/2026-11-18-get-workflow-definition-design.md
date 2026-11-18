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
- Optional Mermaid rendering.
- Integration test against an SAP-delivered workflow present on both integration systems.

## Non-goals (v1)

- Modifying workflow definitions. ADT offers no write path; writing tables is out of scope.
- Cross-workflow search ("which workflow evaluates field X / calls method Y"). Planned as a separate
  tool (`search_workflow_definitions`) in its own PR once this one has landed.
- Container definitions and bindings (`SWDSCONT`, `SWDSBINDEF`; `DATA` is LRAW/XML).
- Resolving constants such as `&CONSTANTS.GC_…&`. Returned verbatim.
- Runtime data: work items (`SWWWIHEAD`…), instances, logs. That is business/runtime data and stays
  outside the scope guardrail.
- Reading the client-dependent definition/runtime copies in `SWD_*`. v1 reads the
  client-independent `SWDS*` family only; if `SWDSHEADER` has no row, the tool reports
  "not found" and says which table it looked in.

## Anti-goals

- Output that leaks SAP logon IDs: `SWDSCONDEF` change-info rows carry a user name and timestamp;
  `SWDSHEADER`/`HRS1205` carry `CREATED_BY`/`UNAME`. None of these columns is selected or returned.
- A graph that "looks complete" but silently dropped nodes because a query hit its row limit. Every
  query must detect truncation and fail loudly rather than return a partial graph.
- A tool that becomes a generic table reader. It only ever queries the fixed table list below with
  a validated `WS\d{8}` ID.

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
4. Neither → highest `SWDSHEADER.VERSION` ≠ `9999` (`version_source = "latest_numbered"`) and a
   warning that no activation pointer was found.
5. The chosen version must exist in `SWDSHEADER`; otherwise "not found", naming the version and
   where it came from.

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
  "version_source": "HRS1205" | "HRP1205" | "latest_numbered" | "requested",
  "language": "E",
  "title": "...",
  "activated_on": "2002-01-22",
  "nodes": [ { "id": "36", "type": "IF", "kind": "condition",
               "label": "Already Revised Once?",
               "condition": "&COUNTER& > 1",
               "task": "", "agent": "", "operation": "", "event": "",
               "block_id": "36", "parent_block_id": "23" } ],
  "edges": [ { "from": "36", "to": "42", "label": "true" } ],
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
- No SAP user name from the condition change-info rows appears anywhere in the JSON.

## Left to the builder

- Exact SQL per table and whether texts/expressions are fetched per workflow or batched across the
  sub-workflow set.
- Mermaid styling and escaping.
- The precise list of node types per `kind`, beyond those listed above (51 types exist; unknown ones
  map to `other` with the raw `type`).
- Wording of warnings (orphan text rows ignored, unknown `CONT_CMD`, sub-workflow not found, etc.).
