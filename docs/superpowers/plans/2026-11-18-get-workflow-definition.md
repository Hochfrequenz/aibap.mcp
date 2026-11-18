# `get_workflow_definition` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A read-only MCP tool `get_workflow_definition` that returns one SAP Business Workflow
definition (SWDD `WS…`) as a decoded graph — active version, branch outcomes, container operations,
resolved constants, container and bindings, sub-workflows — from `SELECT`s on the workflow
definition tables.

**Architecture:** New package `tools/bwf` holds all decoding; it talks to SAP only through
`adt.QueryClient.RunQuery`. `tools/businessworkflow.go` registers the tool and maps parameters to
`bwf.Options`. Stage 1 (Tasks 1–9) covers the client-independent `SWDS*` model; stage 2
(Tasks 10–11) adds the client-dependent `SWD_*` model (runtime copy, legacy definitions).

**Tech Stack:** Go, `mcp-go`, `github.com/Hochfrequenz/adtler/adt` (`QueryClient`), `encoding/xml`,
`encoding/hex`.

**Spec:** `docs/superpowers/specs/2026-11-18-get-workflow-definition-design.md` — read it first; the
data model tables and decoding rules there are normative and not repeated here.

## Global Constraints

- Tool name `get_workflow_definition`; description must say "SAP Business Workflow (SWDD)" and must
  not be confusable with the `sap-adt://guides/workflow` guide. Registered in the `objects` group.
- Annotations: read-only, idempotent, not destructive, open-world.
- Success via `mcp.NewToolResultJSON(result)` with an object; `mcp.WithOutputSchema[bwf.Result]()`.
  Errors via `errorResult(err), nil`, no `StructuredContent` on errors (#351, #354).
- Every query: `max_rows = 5000`; fail if `TotalRows > len(Rows)` **or** `len(Rows) == 5000`.
- Only tables: `SWDS*`, `SWD_*`, `HRS1205`, `HRP1205`, `HRS1000`, `HRP1000`, `SEOCOMPODF`,
  `SEOMETAREL`. Literals in SQL come only from a validated `WS\d{8}` / `\d{4}` or from names read
  out of the workflow's own rows, validated against `^[A-Z0-9_/]{1,30}$` before use.
- `TASK` must be written table-qualified in ABAP SQL (`swdsheader~task`, `h~task`) — unqualified
  `TASK` is rejected by data preview ("has to be escaped using @ or qualified").
- LRAW `SWDSCONT.DATA` / `SWD_CONT.DATA` can only be selected together with `LENGTH`.
- Never select or return logon IDs: no `CREATED_BY`, `CHANGED_BY`, `ACTIVAT_BY`, `UNAME`; drop
  `SWDSCONDEF` change-info rows; drop `CHGDTA` from the container XML.
- Public repo: no system aliases, hosts, SAP logon IDs in code, fixtures, docs, commits. The real
  `SWDSCONDEF` change-info rows of `WS48900002` contain an SAP logon ID — replace it in fixtures
  with `SAPUSER` (keep the 14-digit timestamp so the filter is exercised).
- Before each commit: `gofmt -w .`, `go vet ./...`, `go test ./...`.
- New tool → minor version bump at release (not part of this plan; note it in the PR).

## Review Focus

1. **Workflow ID typed sloppily** (`ws48900002`, ` 48900002 `, `WS4890000`): normalise case and
   whitespace, accept 8 digits alone, reject anything else locally before any query — Task 1.
2. **Workflow with no texts in the requested language** (customer workflow only in German): labels
   fall back to master language, then any language, never empty when some text exists — Task 3.
3. **Huge workflow near the row limit** (100+ nodes × several text languages): truncation must be
   an error, not a silently shorter graph — Task 1 (fetch) and Task 3 (texts filtered to two
   languages in SQL so real workflows stay far below 5000).
4. **Sub-workflow cycle or self-call** (A → B → A): terminates, each workflow appears once in
   `subworkflows` — Task 7.
5. **Constant reference to an element typed as BOR object or DDIC type** (`&ABSENCFORM.CREATOR&`):
   must stay verbatim and *not* be listed as an unresolved constant — Task 5.

---

## File Structure

| File | Responsibility |
|---|---|
| `tools/bwf/types.go` | Output types (`Result`, `Definition`, `Node`, `Edge`, `Binding`, `ContainerElement`, `Call`, `Subworkflow`), `Options`. |
| `tools/bwf/query.go` | `Row`, `fetch` with truncation check, ID normalisation, name validation. |
| `tools/bwf/version.go` | Active/runtime version resolution. |
| `tools/bwf/load_swds.go` | Loads the `SWDS*` raw model. |
| `tools/bwf/raw.go` | `rawModel`, `rawNode`, `rawLine`. |
| `tools/bwf/decode.go` | Raw nodes → `Node` (labels, conditions, operations, agent, events, bindings); raw lines → `Edge`. |
| `tools/bwf/collapse.go` | Block-model collapse incl. loops. |
| `tools/bwf/constants.go` | Constant resolution. |
| `tools/bwf/container.go` | Container elements from `SWDSCNTIDX` + asXML. |
| `tools/bwf/mermaid.go` | Mermaid rendering. |
| `tools/bwf/get.go` | `Get` orchestrator, sub-workflow traversal. |
| `tools/bwf/load_swd.go` | Stage 2: `SWD_*` loader. |
| `tools/bwf/fake_test.go` | Fake `QueryClient` serving fixture rows by table. |
| `tools/bwf/testdata/*.json` | Fixtures captured from the real systems. |
| `tools/businessworkflow.go` | MCP registration, parameter parsing. |
| `tools/businessworkflow_test.go` | Handler tests via `callTool`. |
| `tools/businessworkflow_integration_test.go` | Live tests. |
| `tools/register.go`, `README.md` | Wiring and tool table. |

Tests inside `tools/bwf` are package-internal (`package bwf`) so unexported decoders can be tested
directly.

---

### Task 1: Package skeleton — types, fetch, ID validation, fake client

**Files:**
- Create: `tools/bwf/types.go`, `tools/bwf/query.go`, `tools/bwf/raw.go`, `tools/bwf/fake_test.go`, `tools/bwf/query_test.go`

**Interfaces:**
- Produces:
  - `type Row map[string]string` — upper-case column name → trimmed value.
  - `func fetch(ctx context.Context, q adt.QueryClient, sql string) ([]Row, error)`; `const maxRows = 5000`.
  - `func NormalizeTask(in string) (string, error)` → `"WS48900002"`.
  - `func validName(s string) bool` (`^[A-Z0-9_/]{1,30}$`).
  - Output types exactly as below (JSON tags are the wire contract):

```go
type Options struct {
	Task      string    // normalised WS…
	Version   string    // "" = active
	Source    string    // "definition" | "runtime"
	Depth     int       // 0..5
	Language  string    // SAP 1-char, default "E"
	Raw       bool
	Container bool
	Mermaid   bool
	Today     time.Time // injected for HRP1205 validity
}

type Result struct {
	Definition
	Subworkflows []Subworkflow `json:"subworkflows"`
}

type Definition struct {
	WfdID               string             `json:"wfd_id"`
	Task                string             `json:"task"`
	Version             string             `json:"version"`
	VersionSource       string             `json:"version_source"` // HRS1205|HRP1205|latest_numbered|legacy_client|requested|runtime
	SourceTables        string             `json:"source_tables"`  // SWDS|SWD
	RuntimeVersion      string             `json:"runtime_version"`
	HasConfiguration    bool               `json:"has_configuration"`
	Language            string             `json:"language"`
	Title               string             `json:"title"`
	ActivatedOn         string             `json:"activated_on"` // YYYY-MM-DD or ""
	Nodes               []Node             `json:"nodes"`
	Edges               []Edge             `json:"edges"`
	Container           []ContainerElement `json:"container"`
	Calls               []Call             `json:"calls"`
	UnresolvedConstants []string           `json:"unresolved_constants"`
	Warnings            []string           `json:"warnings"`
	Mermaid             string             `json:"mermaid,omitempty"`
}

type Node struct {
	ID, Type, Kind, Label                   string // json: id,type,kind,label
	Condition, ConditionResolved            string // condition,condition_resolved
	Task, Agent                             string // task,agent
	Operation, OperationResolved            string // operation,operation_resolved
	Event                                   string // event
	BlockID, ParentBlockID                  string // block_id,parent_block_id
	Bindings                                []Binding // bindings
}

type Edge struct{ From, To, Label string } // from,to,label

type Binding struct {
	Direction, StepElement, Expression, ExpressionResolved, BindType string
	// direction ("export"|"import"|raw DATAFLOW), step_element, expression, expression_resolved, bind_type
}

type ContainerElement struct {
	Name, Description, Type, Category, TypeKind, DDICType string
	Import, Export, Mandatory                             bool
	InitialValue, Props                                   string
	// name,description,type,category,type_kind,ddic_type,import,export,mandatory,initial_value,props
}

type Call struct{ Node, WfdID string } // node,wfd_id

type Subworkflow struct {
	WfdID      string      `json:"wfd_id"`
	Found      bool        `json:"found"`
	Depth      int         `json:"depth"`
	CalledBy   []string    `json:"called_by"`
	Definition *Definition `json:"definition,omitempty"`
}
```

  (Write the field-per-line form with the JSON tags given in the comments; slices are never `nil`
  in output — initialise to empty so the schema's arrays serialise as `[]`.)
  - Fake: `type fakeQuery struct{ tables map[string][]Row; calls []string }` implementing
    `adt.QueryClient`. It extracts the first table after `FROM` (case-insensitive, also
    `FROM x AS a`), returns that table's rows filtered to those whose `WFD_ID`, `TASK`, `OBJID`
    or `CLSNAME` value (whichever columns exist) appears as a quoted literal in the SQL, and
    projects nothing (returns full rows). `func loadFixture(t *testing.T, name string) map[string][]Row`
    reads `testdata/<name>.json` (`{"SWDSMNODES": [{"NODEID": "0000000001", …}], …}`).

- [ ] **Step 1: Write failing tests** in `query_test.go`:
  - `TestNormalizeTask`: `"WS48900002"`, `"ws48900002"`, `" 48900002 "` → `"WS48900002"`; `"WS4890000"`, `"TS30000018"`, `"WS4890000X"`, `""` → error.
  - `TestFetch_MapsRowsByColumn`: fake returns `Columns [{Name:"NODEID"},{Name:"NODETYPE"}]`, `Rows [["0000000001","STRT"]]` → `[]Row{{"NODEID":"0000000001","NODETYPE":"STRT"}}`.
  - `TestFetch_TruncationIsError`: `TotalRows 6000` with 2 rows → error containing `"truncated"`; exactly 5000 rows with `TotalRows 5000` → error.
  - `TestFetch_PassesMaxRows`: fake records `maxRows == 5000`.
  - `TestValidName`: `/IDXGC/CL_PD_WF_DOCUMENT`, `GC_ABAP_TRUE` ok; `X' OR '1`, 31 chars, `""` rejected.
- [ ] **Step 2:** `go test ./tools/bwf/ -run 'TestNormalizeTask|TestFetch|TestValidName'` → FAIL (undefined).
- [ ] **Step 3:** Implement `types.go`, `query.go`, `raw.go` (empty `rawModel` struct placeholder filled in Task 3), `fake_test.go`.
- [ ] **Step 4:** Same command → PASS.
- [ ] **Step 5:** Commit `feat(#148): bwf package skeleton with guarded fetch`.

---

### Task 2: Version resolution

**Files:**
- Create: `tools/bwf/version.go`, `tools/bwf/version_test.go`

**Interfaces:**
- Consumes: `fetch`, `Row`, `Options`.
- Produces:

```go
type versionInfo struct {
	WfdID, Version, Source, Tables string // Tables: "SWDS" | "SWD"
	RuntimeWfdID, RuntimeVersion   string
	HasConfiguration               bool
	Warnings                       []string
}
func resolveVersion(ctx context.Context, q adt.QueryClient, task, requested string, today time.Time) (versionInfo, error)
func resolveRuntime(ctx context.Context, q adt.QueryClient, task string) (wfdID, version string, hasConfig bool, err error)
```

Rules: spec "Active version rule" steps 1–6 verbatim. `ErrNotFound` (exported sentinel) when
neither `SWDSHEADER` nor legacy `SWD_HEADER` (`EXETYP='S'`) has the task; message names
`SWDSHEADER, SWD_HEADER`. `resolveRuntime` reads `SWD_HEADER` by task: highest `EXETYP='E'` row →
runtime; any `EXETYP IN ('C','A')` row → `hasConfig`.

- [ ] **Step 1: Failing tests** (fake tables built inline):
  - `TestResolveVersion_HRS1205Active`: HRS1205 `{OBJID 00400012, VERSION 0003, ACTIVE X}`, SWDSHEADER versions `0000,0001,0002,0003,9999` → `Version "0003"`, `Source "HRS1205"`, `Tables "SWDS"`, no warnings.
  - `TestResolveVersion_HRS1205MultipleRowsOneActive`: rows `0000 ""`, `0001 ""`, `0003 X` → `"0003"`.
  - `TestResolveVersion_HRS1205TwoActive`: `0001 X`, `0002 X` → `"0002"` + 1 warning mentioning both.
  - `TestResolveVersion_Active9999`: `9999 X` and SWDSHEADER has `9999` → `"9999"`.
  - `TestResolveVersion_HRP1205`: no HRS1205; HRP1205 `{VERSION 0002, ACTIVE X, BEGDA 20200101, ENDDA 99991231}` plus expired `{0001, X, ENDDA 20191231}`, today 2026-11-18 → `"0002"`, `Source "HRP1205"`.
  - `TestResolveVersion_LatestNumbered`: no pointer; SWDSHEADER `0000,0004,9999` → `"0004"`, `Source "latest_numbered"`, 1 warning.
  - `TestResolveVersion_Requested`: requested `"0001"` exists → `Source "requested"`; requested `"0007"` missing → error naming `0007`.
  - `TestResolveVersion_LegacyClient`: no SWDSHEADER row; SWD_HEADER `{WFD_ID 0040000064, TASK WS00400012, EXETYP S, VERSION 0001}` → `WfdID "0040000064"`, `Tables "SWD"`, `Source "legacy_client"`.
  - `TestResolveVersion_NotFound`: all empty → `errors.Is(err, ErrNotFound)`.
  - `TestResolveRuntime`: SWD_HEADER `{WS48900002, 0000, E}` → `("WS48900002","0000",false)`; with an extra `C` row → `hasConfig true`.
  - `TestResolveVersion_WfdIDDiffersFromTask`: SWDSHEADER row `{WFD_ID WS00000001, TASK WS99999999}` is found when asking for task `WS99999999`.
- [ ] **Step 2:** `go test ./tools/bwf/ -run 'TestResolve'` → FAIL.
- [ ] **Step 3:** Implement. SQL selects only `WFD_ID, VERSION, ACTIVE, BEGDA, ENDDA` (HRP), `OBJID, WFD_ID, VERSION, ACTIVE` (HRS), `WFD_ID, VERSION, LANGUAGE, ACTIVAT_ON` + qualified `task` (headers).
- [ ] **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(#148): resolve active and runtime workflow version`.

---

### Task 3: `SWDS*` loader and node decoding (un-collapsed)

**Files:**
- Create: `tools/bwf/load_swds.go`, `tools/bwf/decode.go`, `tools/bwf/decode_test.go`, `tools/bwf/testdata/ws48900002.json`
- Modify: `tools/bwf/raw.go`

**Interfaces:**
- Consumes: `fetch`, `versionInfo`.
- Produces:

```go
type rawNode struct {
	ID, Type, Task, BlockID, ParentBlockID, ContCmd, Return, ReturnCode,
	ExcClass, MinTokens, ExpTokens, EvtName, EvtOType, CaseConte string
}
type rawLine struct{ From, To, ReturnCode string }
type rawModel struct {
	Model                                string // "SWDS" | "SWD"
	WfdID, Version, MasterLanguage       string
	ActivatedOn                          string
	Nodes                                []rawNode
	Lines                                []rawLine
	Texts, Conds, Exprs, Bindings        []Row
	CntIdx, ContXML, TaskTexts           []Row
}
func loadSWDS(ctx context.Context, q adt.QueryClient, wfdID, version, language string, withContainer bool) (*rawModel, error)
func decodeNodes(m *rawModel, language string) (nodes []Node, warnings []string)
func rawEdges(m *rawModel) []Edge
func taskOf(nodeTask string) string // trims, "" for none
```

Decoding rules come from the spec ("Data model", "Block model and node types"): ids without leading
zeros; `kind` per node type (spec list; unknown → `other`); label priority `ND` text in
`language` → master language → any → task short text (`HRS1000`/`HRP1000` `STEXT`) → raw type;
conditions from `SWDSCONDEF` (drop `LINE_ID 000` header and change-info rows, strip `{TYPE=…}`,
`EX`/`NX` → `is filled`/`is initial`, join with `LOGOP`, `NOT` prefix when `NOTFLAG`);
operations from `SWDSEXPR` 031/032/033 + `CONT_CMD` (`12 :=`, `18 append`, `11 + 13 - 14 * 15 /`,
unknown → `cmd <nn>` + warning); `CTRL` `CONT_CMD` `01/02/04/07` → text; agent from `013`;
multiple condition expression `035` into `condition`; `event` from `EVT_NAME` (+`EVT_OTYPE`);
bindings from `SWDSBINDEF` (`DATAFLOW E`→`export`, `I`→`import`, other raw); `SWDSTEXT` rows for
node ids absent from `Nodes` are ignored (one aggregated warning). Text SQL restricts
`LANGUAGE IN (<language>, <master>)`; if that yields no `ND` at all, one more query without the
filter.

Fixture: capture `WS48900002` version `0001` from a real system with the column lists used by
`loadSWDS` (`SWDSHEADER`, `SWDSMNODES`, `SWDSMLINES`, `SWDSTEXT` E+D, `SWDSCONDEF`, `SWDSEXPR`
without 037–040, `SWDSBINDEF`, `SWDSCNTIDX`, `SWDSCONT` incl. `LENGTH`/`DATA`, `HRS1000` for its
tasks, `HRS1205` for `48900002`/`48900024`), plus `WS48900024` version `0001` for Task 7. Replace
the logon ID in change-info rows and in the container XML `CHGDTA` with `SAPUSER` (re-hex the
`DATA` after editing; keep chunk boundaries at 1024 bytes).

- [ ] **Step 1: Capture fixture** via `run_query` (purpose `development_metadata`) into `testdata/ws48900002.json`; check `grep -ri` on the file finds no logon ID other than `SAPUSER`.
- [ ] **Step 2: Failing tests** in `decode_test.go` (load fixture through `loadSWDS` with the fake):
  - `TestDecode_NodeCountRaw`: 53 nodes, 56 raw edges; node `"36"` has `Type "IF"`, `Kind "condition"`, `Label "Already Revised Once?"`, `Condition "&COUNTER& > 1"`, `BlockID "36"`, `ParentBlockID "23"`.
  - `TestDecode_LoopCondition`: node `"10"` `Kind "loop_until"`, `Condition "&FLAG& = X"`.
  - `TestDecode_Operations`: node `"33"` `Operation "counter := &counter& + 1"`; node `"30"` `"flag := X"`; node `"15"` `"flag := "` (empty operand kept as empty).
  - `TestDecode_ProcessControl`: node `"47"` `Kind "process_control"`, `Operation "complete workflow"`.
  - `TestDecode_MultipleCondition`: node `"23"` `Kind "multiple_condition"`, `Condition "&RESULT&"`.
  - `TestDecode_StepAndSubworkflow`: node `"4"` `Kind "step"`, `Task "TS30200106"`, `Agent "&_WF_INITIATOR&"`; node `"18"` `Kind "subworkflow"`, `Task "WS48900024"`.
  - `TestDecode_Bindings`: node `"18"` has bindings `{export, FORMABSENC, &ABSENCFORM&, WW}` and `{import, RESULT, &RESULT&, WW}`.
  - `TestDecode_LanguageFallback`: `language "D"` → node `"36"` label `"Bereits einmal überarbeitet ?"`; fixture copy with all `D` rows removed and `language "D"` → falls back to English label.
  - `TestDecode_NoLogonIDInOutput`: `json.Marshal` of nodes contains neither `SAPUSER` nor a 14-digit timestamp.
  - `TestDecode_OrphanTextsIgnored`: no node with id `"70"`; exactly one warning mentioning ignored text rows.
  - `TestDecode_UnknownContCmd`: fixture copy with node 33 `CONT_CMD 99` → `Operation` starts with `"cmd 99"` and a warning.
  - `TestDecode_ChangeInfoVariants`: hand-built `Conds` where the change-info row is at `LINE_ID 000` in one node and shares `LINE_ID 001` in another, plus `RIGHTEXPR "'312'{TYPE=C}"` and `COMPOP "NX"` → conditions `"&A& = '312'"` and `"&B& is initial"`.
- [ ] **Step 3:** `go test ./tools/bwf/ -run TestDecode` → FAIL.
- [ ] **Step 4:** Implement loader + decoder.
- [ ] **Step 5:** PASS.
- [ ] **Step 6:** Commit `feat(#148): load and decode SWDS workflow nodes`.

---

### Task 4: Collapse block model (incl. loops)

**Files:**
- Create: `tools/bwf/collapse.go`, `tools/bwf/collapse_test.go`, `tools/bwf/testdata/ws00400012_while.json`

**Interfaces:**
- Consumes: `[]Node`, `[]Edge` from `decodeNodes`/`rawEdges`, `*rawModel` (for `Return`, `ReturnCode`, `ExcClass`, `MinTokens`, `ExpTokens`).
- Produces: `func collapse(m *rawModel, nodes []Node, edges []Edge) ([]Node, []Edge)`

Rules: spec "Collapsing" and "Loops". Outcome labels: `IFRS`/`UNRS` `RETURN 0001 → "true"`,
`0000 → "false"`; `CRES` → `RETURNCODE`; `COTH` → `"others"`; `DRES` → node label; `EXCP` with
`ExcClass` → `"exception " + class`, without → no label; `TEVT` → `"event " + EVT_NAME` when
`EVT_NAME` is set, else the `TEVT` node's own label (in `WS48900002` `EVT_NAME` is empty and only
`EVT_OTYPE = FORMABSENC` is set). Join
label `"<MinTokens> of <ExpTokens>"` (leading zeros dropped). Admin nodes (`999xxx`, `EFUN`,
`EVTG`, `OFF`) removed; `END` kept as `kind "end"`.

- [ ] **Step 1: Verify the while-loop mapping live** before coding: dump `SWDSMNODES`
  (`NODEID, NODETYPE, RETURN, RETURNCODE`), `SWDSMLINES`, `SWDSTEXT` (E) for `WS00400012` at its
  `HRS1205` version on a real system; record in a comment at the top of `collapse.go` which of
  `WHRS`/`WOTH` leads into the body (= `"true"`) and save the rows as `testdata/ws00400012_while.json`
  (no logon IDs).
- [ ] **Step 2: Failing tests** (`WS48900002` fixture through `loadSWDS` → `decodeNodes` → `collapse`):
  - `TestCollapse_NoHelperNodes`: no node of type `XOR`, `ECAS`, `ENDI`, `UNJO`, `IFRS`, `UNRS`, `CRES`, `COTH`, `EXCP`, `TEVT`, `CEVT`, `EEVT`, `EFUN`, `EVTG`, `OFF` remains.
  - `TestCollapse_ConditionEdges`: edges `36→42 "true"`, `36→50 "false"`.
  - `TestCollapse_CaseEdges`: edges from `23` labelled `"A"` (→30), `"R"` (→33), `"others"` (→ node after the case merge, i.e. `10`).
  - `TestCollapse_UntilLoop`: edge `10→55 "true"` (exit to fork) and `10→15 "false"` (back to first body node); no edge points to `9`.
  - `TestCollapse_ForkJoin`: node `56` `Kind "join"`, label `"2 of 2"`; edges `55→60`, `55→64`.
  - `TestCollapse_TerminatingEvent`: edge `42→47` labelled `"Form deleted"` (fixture `TEVT` node 45 has empty `EVT_NAME`); a fixture copy with `EVT_NAME "DELETED"` on node 45 gives `"event DELETED"`.
  - `TestCollapse_WhileLoop` (while fixture): `WHIL` node kept, `Kind "loop_while"`, `"true"` edge into the body per Step 1, `"false"` edge to the node after the loop, body's last node has an edge back to the `WHIL` node; no `WHJO`, `WHRS`, `WOTH`, `EWHI` remain.
  - `TestCollapse_RawModeUntouched`: `Options.Raw` path (call site in Task 7) — here assert `collapse` is pure: input slices unchanged after the call.
- [ ] **Step 3:** `go test ./tools/bwf/ -run TestCollapse` → FAIL.
- [ ] **Step 4:** Implement.
- [ ] **Step 5:** PASS.
- [ ] **Step 6:** Commit `feat(#148): collapse workflow block model into labelled edges`.

---

### Task 5: Constant resolution

**Files:**
- Create: `tools/bwf/constants.go`, `tools/bwf/constants_test.go`, `tools/bwf/testdata/constants_idxgc.json`

**Interfaces:**
- Consumes: `fetch`, `rawModel.CntIdx`, decoded `[]Node`.
- Produces: `func resolveConstants(ctx context.Context, q adt.QueryClient, m *rawModel, nodes []Node) (resolved []Node, unresolved []string, err error)` — fills `ConditionResolved`, `OperationResolved`, `Bindings[i].ExpressionResolved` for every node (copy of the original when nothing to resolve).

Rules: spec "Constants". Reference regex `&([A-Z0-9_]+)\.([A-Z0-9_]+)&`. Only elements whose
`SWDSCNTIDX` row (`OTYPE='WS'`) has `CATID='CL'` are candidates; references to other elements are
left verbatim and **not** reported. Lookup batched: one `SEOCOMPODF` query per round
(`CLSNAME IN (…) AND CMPNAME IN (…) AND VERSION = '1' AND ATTDECLTYP = '2'`), one `SEOMETAREL`
query per hierarchy level (`VERSION = '1'`, `RELTYPE IN ('1','2')`), max 5 levels, visited set.
Replacement value is `ATTVALUE` verbatim (ABAP literal incl. quotes). Unresolvable candidates →
`unresolved` (deduplicated, sorted).

Fixture: rows from the ECC system for `WS00288801` version `0002`: `SWDSCNTIDX` element
`CONSTANTS`, `SEOCOMPODF` for `/IDXGC/CL_PD_WF_DOCUMENT` (`GC_ABAP_TRUE`, `GC_PROC_VAL_UTILMD`,
`ATTDECLTYP 0`, empty `ATTVALUE`) and `/IDXGC/IF_PD_WF_CONSTANTS` (`ATTDECLTYP 2`, `'X'`,
`'UTILMD'`), `SEOMETAREL` for the class.

- [ ] **Step 1: Capture fixture** (live, as above).
- [ ] **Step 2: Failing tests:**
  - `TestResolveConstants_ViaInterface`: node condition `"&LV_END_DCR& = &CONSTANTS.GC_ABAP_TRUE&"` → `ConditionResolved "&LV_END_DCR& = 'X'"`; `"&LV_FIRST_MSG_TYPE& = &CONSTANTS.GC_PROC_VAL_UTILMD&"` → `"… = 'UTILMD'"`; `unresolved` empty.
  - `TestResolveConstants_Unresolved`: `&CONSTANTS.GC_DOES_NOT_EXIST&` → stays verbatim, `unresolved == ["&CONSTANTS.GC_DOES_NOT_EXIST&"]`.
  - `TestResolveConstants_NonClassElementIgnored`: `&ABSENCFORM.CREATOR&` with `CATID 'BO'` → verbatim, `unresolved` empty, no `SEOCOMPODF` query issued (fake `calls` contains no `SEOCOMPODF`).
  - `TestResolveConstants_BindingsAndOperations`: operation `"lv := &CONSTANTS.GC_ABAP_TRUE&"` and binding expression with the same reference both resolved.
  - `TestResolveConstants_CycleInHierarchy`: `SEOMETAREL` A→B, B→A, constant nowhere → terminates, reported unresolved.
  - `TestResolveConstants_InvalidNameNotQueried`: element typed with class name failing `validName` → not queried, reported unresolved.
- [ ] **Step 3:** FAIL. **Step 4:** Implement. **Step 5:** PASS.
- [ ] **Step 6:** Commit `feat(#148): resolve class constants in workflow expressions`.

---

### Task 6: Container elements

**Files:**
- Create: `tools/bwf/container.go`, `tools/bwf/container_test.go`

**Interfaces:**
- Consumes: `rawModel.CntIdx`, `rawModel.ContXML` (rows with `TABIX`, `LENGTH`, `DATA` hex).
- Produces: `func decodeContainer(m *rawModel) ([]ContainerElement, []string, error)`

Rules: spec "Container and bindings". Elements = `SWDSCNTIDX` rows with `OTYPE='WS'` (name, type
`TYPEID`, `CATID`, `TYPEKIND`, `TYPENAME`), enriched from the XML element of the same `NAME`
(description from `LTEXTS` in the requested/any language, `VALUE` → `InitialValue`, `PROPS` raw).
XML: hex-decode each `DATA` (first `LENGTH` bytes), concatenate by `TABIX`, trim trailing spaces,
`encoding/xml` decode of the `SWFDELMDEFXML` elements (namespace-agnostic). `CHGDTA` is never
read into the output. Missing XML → elements from `SWDSCNTIDX` only + warning. Elements in the XML
but not in `SWDSCNTIDX` are added.

- [ ] **Step 1: Determine the `PROPS` flag bits** on a real system: read the source of
  `CL_SWF_CNT_ELEMENT` / the container element definition class (`get_source`), find where `PROPS`
  is built/parsed, and note the bit for import, export, mandatory. Cross-check against SWDD for
  `WS48900002` (expected: `ABSENCFORM` is import; `COUNTER`/`FLAG`/`RESULT` are not). If the bits
  cannot be determined, set `Import/Export/Mandatory` false, keep `Props` raw, and add the warning
  `"container flags not decoded"` — and make the flag test below assert that instead.
- [ ] **Step 2: Failing tests** (WS48900002 fixture):
  - `TestContainer_Elements`: names `ABSENCFORM, COUNTER, FLAG, RESULT`; `ABSENCFORM` `Type "FORMABSENC"`, `Category "BO"`; `RESULT` `DDICType "WFSYST"`, `TypeKind "C"`.
  - `TestContainer_Flags`: `ABSENCFORM.Import == true`, others false (per Step 1 outcome).
  - `TestContainer_NoChangerInOutput`: marshalled elements do not contain `SAPUSER`.
  - `TestContainer_MissingXML`: `ContXML` empty → 4 elements from index, 1 warning, no error.
  - `TestContainer_CorruptHex`: invalid hex → error mentioning `SWDSCONT`.
- [ ] **Step 3:** FAIL. **Step 4:** Implement. **Step 5:** PASS.
- [ ] **Step 6:** Commit `feat(#148): decode workflow container`.

---

### Task 7: `Get` orchestrator, sub-workflows, Mermaid

**Files:**
- Create: `tools/bwf/get.go`, `tools/bwf/mermaid.go`, `tools/bwf/get_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `func Get(ctx context.Context, q adt.QueryClient, opts Options) (*Result, error)` and `func renderMermaid(d *Definition) string`.

Flow per definition: `resolveVersion` (or runtime branch — stage 2; in stage 1 `Source "runtime"`
returns error `"source=runtime not implemented yet"`, replaced in Task 10) → `loadSWDS` →
`decodeNodes` → `collapse` unless `Raw` → `resolveConstants` → `decodeContainer` if `Container`
→ `Calls` = nodes with `Kind "subworkflow"` → Mermaid if requested. Sub-workflows: BFS from the
root up to `Depth` (clamp 0..5), visited set, each distinct `WS` once in `Result.Subworkflows`
with `Depth` of first discovery and all callers in `CalledBy`; not found (`ErrNotFound`) →
`Found false`, warning on the root. `Result.Subworkflows` definitions carry no own
`subworkflows` (type guarantees it). Mermaid: `flowchart TD`, node ids `n<id>`, labels quoted with
`"` escaped as `#quot;`, edge labels `-->|label|`.

- [ ] **Step 1: Failing tests:**
  - `TestGet_Root`: `Get(Task "WS48900002")` → `Version "0001"`, `VersionSource "HRS1205"`, `Title` non-empty, `len(Calls) == 1` (`{Node "18", WfdID "WS48900024"}`), `Subworkflows` empty (depth 0).
  - `TestGet_Depth1`: `Depth 1` → one subworkflow `WS48900024`, `Found true`, `Depth 1`, `CalledBy ["WS48900002"]`, `Definition.Nodes` non-empty.
  - `TestGet_Cycle`: fixture copy where `WS48900024` calls `WS48900002` back, `Depth 5` → terminates, `Subworkflows` has exactly one entry.
  - `TestGet_MissingSub`: sub fixture removed → `Found false`, root warning names `WS48900024`.
  - `TestGet_Raw`: `Raw true` → 53 nodes.
  - `TestGet_ContainerOff`: `Container false` → `Container` empty slice, no `SWDSCONT` query in `calls`.
  - `TestGet_Mermaid`: starts with `"flowchart TD"`, contains `-->|true|`; `Mermaid false` → field empty.
  - `TestGet_DepthClamped`: `Depth 99` behaves as 5 (no error).
  - `TestGet_EmptySlicesSerialiseAsArrays`: marshalled JSON of a result with no calls contains `"calls":[]`, `"subworkflows":[]`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat(#148): assemble workflow definitions with sub-workflows`.

---

### Task 8: MCP tool registration

**Files:**
- Create: `tools/businessworkflow.go`, `tools/businessworkflow_test.go`
- Modify: `tools/register.go` (`objects` group), `README.md` (objects table, group count `11` → `12`)

**Interfaces:**
- Consumes: `bwf.Get`, `bwf.Options`, `bwf.Result`, `bwf.NormalizeTask`.
- Produces: `func registerBusinessWorkflowTools(s toolAdder, client adt.QueryClient)`.

Parameters exactly as the spec "Tool surface" table: `wfd_id` (required), `version`, `source`
(`definition`|`runtime`, default `definition`), `depth` (default 0), `language` (default `E`;
accept `EN`/`DE` and map to `E`/`D`, else first char upper-cased), `raw`, `container` (default
true), `mermaid`. `version` validated `^\d{4}$`; bad `source` rejected locally. `Today` =
`time.Now()`. README row: ``| `get_workflow_definition` | Read an SAP Business Workflow definition (SWDD) as a decoded graph: active version, branches, container operations, resolved constants, container, sub-workflows |``.

- [ ] **Step 1: Failing tests** (`mockClient.runQueryFn` backed by the bwf fixture — load the JSON in the test with a small helper; the bwf fake is package-internal):
  - `TestGetWorkflowDefinition_Success`: `callTool(… {"wfd_id":"WS48900002"})` → not `IsError`, `StructuredContent` is a map with `"version" == "0001"`.
  - `TestGetWorkflowDefinition_InvalidID`: `"wfd_id":"TS30000018"` → `IsError`, no `RunQuery` call.
  - `TestGetWorkflowDefinition_InvalidVersion`: `"version":"1"` → `IsError`, no call.
  - `TestGetWorkflowDefinition_InvalidSource`: `"source":"db"` → `IsError`, no call.
  - `TestGetWorkflowDefinition_NotFoundIsToolError`: empty tables → `IsError`, text names `SWDSHEADER`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement + wire + README.
- [ ] **Step 4:** `go test ./...` → PASS, including `TestReadmeToolNamesMatchRegistered` and `structured_content_shape_test.go` (the reflective call with only `wfd_id` must hit the error path cleanly or succeed — no panic).
- [ ] **Step 5:** Commit `feat(#148): get_workflow_definition tool`.

---

### Task 9: Stage-1 integration tests

**Files:**
- Create: `tools/businessworkflow_integration_test.go` (`//go:build integration`)

Per system in `integrationSystems` (`requireReachable`, `mustSelectSystem` on `sharedServer`):

- [ ] **Step 1:** `TestIntegration_WorkflowDefinition_WS48900002` asserting every bullet of the spec
  "Integration test" for `WS48900002` (version/source, condition + true/false edges, until loop,
  case A/R/others, operations, process control, join `2 of 2`, `depth 1` sub-workflow found,
  container elements + step 18 bindings, no `SAPUSER`-like logon ID: assert the JSON contains no
  14-digit timestamp substring and none of the header user columns' names).
- [ ] **Step 2:** `TestIntegration_WorkflowDefinition_Constants_ISU`: first `run_query`
  `SELECT swdsheader~task FROM swdsheader WHERE swdsheader~task = 'WS00288801'`; no row →
  `t.Skipf("WS00288801 not present — system has no IS-U common layer; constant resolution via workflow not verified on %s", sys)`.
  Otherwise assert `condition_resolved` values `'X'` and `'UTILMD'` per spec, `version "0002"`.
- [ ] **Step 3:** Run `go test -tags integration -v -count=1 -run 'WorkflowDefinition' ./tools/...`
  → PASS on both systems, IS-U test PASS on the ECC system and SKIP (with message) on the S/4
  system. Paste the output into the PR.
- [ ] **Step 4:** Commit `test(#148): integration tests for get_workflow_definition`.

---

### Task 10: Stage 2 — `SWD_*` model (runtime copy, legacy definitions)

**Files:**
- Create: `tools/bwf/load_swd.go`, `tools/bwf/load_swd_test.go`, `tools/bwf/testdata/ws48900002_runtime.json`, `tools/bwf/testdata/legacy.json`
- Modify: `tools/bwf/decode.go` (1-char kinds), `tools/bwf/get.go` (runtime + legacy branches)

**Interfaces:**
- Consumes: `versionInfo` (`Tables "SWD"`), `resolveRuntime`.
- Produces: `func loadSWD(ctx context.Context, q adt.QueryClient, wfdID, version, exetyp, language string, withContainer bool) (*rawModel, error)` — `Model "SWD"`, nodes from `SWD_NODES`, lines from `SWD_LINES` (with `RETURNCODE`), texts/conds/exprs/bindings/cntidx/cont from `SWD_TEXT`/`SWD_CONDEF`/`SWD_EXPR`/`SWD_BINDEF`/`SWD_CNTIDX`/`SWD_CONT`; tasks from `SWD_STEPS` (`NODEID → TASK`).

Rules: spec "Client-dependent data". 1-char kinds: `S` step (→ `subworkflow` when task is `WS…`),
`F` fork, `J` join, `I` condition, `C` multiple condition, `P` container operation, `O` process
control, `R` wait event, `X`/`Y` block markers (dropped unless `Raw`), others `other`. Operands of
`P` nodes from `SWD_NODES.PARAMETER1–3` (spec #148 comment: `PARAMETER1` result, `2`/`3`
operands) when `SWD_EXPR` 031–033 are absent. `collapse` is skipped for `Model "SWD"`; edge
labels come from `SWD_LINES.RETURNCODE` via a confirmed-codes map.
`Get`: `Source "runtime"` → `resolveRuntime` → error `"no runtime copy of <task> in the logon client"`
when empty, else `loadSWD(..., "E", …)`, `VersionSource "runtime"`. `Tables "SWD"` from
`resolveVersion` (legacy) → `loadSWD(..., "S", …)`. `RuntimeVersion`/`HasConfiguration` set on
every root definition.

- [ ] **Step 1: Verify `RETURNCODE` meanings live**: for `WS48900002`, dump the runtime copy
  (`SWD_NODES`, `SWD_LINES`, `SWD_TEXT` E, `EXETYP 'E'`, version `0000`) on the ECC system and
  match each condition/loop outgoing `RETURNCODE` against the `SWDS*` outcome (`IFRS.RETURN`) of the
  same node id/text. Record confirmed codes (expected `8995`/`8994`, direction to be confirmed) in
  the map with a comment; unconfirmed codes stay raw labels. Save rows as
  `testdata/ws48900002_runtime.json`; also capture one legacy-only task (query: `SWD_HEADER`
  `EXETYP 'S'` whose task has no `SWDSHEADER` row) as `testdata/legacy.json`. No logon IDs.
- [ ] **Step 2: Failing tests:**
  - `TestLoadSWD_RuntimeCopy`: 26 nodes raw; condition node with `&COUNTER& > 1` and edges labelled `"true"`/`"false"` per Step 1.
  - `TestLoadSWD_ContainerOperationFromParameters`: `P` node with only `PARAMETER1/2` set → `Operation "<p1> := <p2>"`.
  - `TestGet_SourceRuntime`: `Get(Source "runtime")` → `VersionSource "runtime"`, `SourceTables "SWD"`, `Version "0000"`.
  - `TestGet_SourceRuntimeMissing`: no `E` row → error text `"no runtime copy"`.
  - `TestGet_LegacyClient`: legacy fixture → `VersionSource "legacy_client"`, `SourceTables "SWD"`, nodes non-empty, `WfdID` numeric, `Task` = the `WS` id.
  - `TestGet_RuntimeVersionReported`: definition call on `WS48900002` with runtime row `0000` → `RuntimeVersion "0000"`.
  - `TestGet_HasConfiguration`: `C` row → `HasConfiguration true`.
- [ ] **Step 3:** FAIL. **Step 4:** Implement. **Step 5:** `go test ./...` PASS.
- [ ] **Step 6:** Commit `feat(#148): read client-dependent SWD workflow copies`.

---

### Task 11: Stage-2 integration tests and wrap-up

**Files:**
- Modify: `tools/businessworkflow_integration_test.go`

- [ ] **Step 1:** `TestIntegration_WorkflowDefinition_Runtime`: `source "runtime"` for
  `WS48900002`; branch explicitly: success → `version_source "runtime"`, nodes non-empty; tool
  error → text contains `"no runtime copy"`. Any other outcome fails.
- [ ] **Step 2:** `TestIntegration_WorkflowDefinition_Legacy`: find a legacy-only task by
  `run_query` (`SWD_HEADER` `EXETYP 'S'` not in `SWDSHEADER`); none found → `t.Fatalf` (spec:
  fail loudly, not skip); else assert `version_source "legacy_client"`, nodes non-empty.
- [ ] **Step 3:** Run `go test -tags integration -v -count=1 -run 'WorkflowDefinition' ./tools/...`
  on both systems → PASS (IS-U test SKIP on S/4 with message).
- [ ] **Step 4:** `gofmt -l .` empty, `go vet ./...`, `go test ./...`, `make lint` clean.
- [ ] **Step 5:** Diff scan for internal data: grep the branch diff for an internal domain suffix,
  `K9[0-9]{5}` transport numbers, and logon-ID shapes; must be empty.
- [ ] **Step 6:** Commit `test(#148): integration tests for runtime and legacy workflow copies`.
- [ ] **Step 7:** Ask the user before pushing/opening the PR. PR body (cold-reader, independent
  review subagent first per CLAUDE.md): `Closes #148`, minor bump note (new tool), integration
  output, follow-up issue suggestion for `search_workflow_definitions`.
