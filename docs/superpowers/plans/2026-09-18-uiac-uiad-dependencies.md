# UIAC/UIAD Dependency Resolution — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `get_object_dependencies` resolve UIAC (Fiori catalog) and UIAD (catalog app entry) objects instead of rejecting them, so a transport completeness check can list a catalog's app entries and each entry's launch target without falling back to hand-written SQL.

**Architecture:** The dependency engine lives in adtler (`adt/dependencies.go`), not in aibap.mcp — the MCP handler only forwards `object_type`. The change is therefore two PRs in two repositories: adtler gains two cases in the existing `switch objectType`, aibap.mcp updates the tool surface and consumes the new adtler release. Both new cases follow the established pattern of the PROG/FUGR arms: one `RunQuery` against a SAP repository table, mapped to `[]ObjectDependency`.

**Tech Stack:** Go, adtler `adt` package, SAP ADT data-preview endpoint (`RunQuery`), `net/http/httptest` for unit tests, `eachSystem(t)` for adtler integration tests.

**Spec:** aibap.mcp issue #409, plus the live findings in "Verified Facts" below. The issue's own implementation proposal is superseded — see "Deviations from the issue".

## Global Constraints

- **Public repositories, both of them.** No hostnames, no internal or partner SAP namespaces (a leading slash-namespace), no internal catalog or object names, no system aliases, no transport numbers, no client numbers tied to a named system, no support-package levels, no local paths carrying a user name — not in code, comments, commit messages, issue text or PR text. Name a system by type and release level instead. Integration tests must discover their test objects at runtime rather than hardcoding real object names.
- **Column access by name, never by position.** Data-preview results are addressed through `queryColumnIndexes` / `queryCell` (`adt/transport.go`), because the endpoint may reorder or omit columns. This is the single most important technical constraint in this plan.
- Go: `gofmt -w .`, `go vet ./...`, `go test ./...` green before every commit. adtler additionally: `go build -tags integration ./adt/...` and `go vet -tags integration ./adt/...`.
- All SQL values go through `EscapeValue`.
- adtler's fix/review/test/merge cycle (adtler `CLAUDE.md`) is binding: unit tests + `eachSystem(t)` integration test, PR linking both issues, `needs:integration-test` label, independent reviewer GO/NO-GO comment, green CI, real-SAP integration run, then label removal.
- Every issue body, PR description and substantive issue comment goes past an independent reviewer before posting (aibap.mcp `CLAUDE.md`).
- Versioning: the aibap.mcp description edit alone would be a patch; it is the adtler bump making a previously failing call succeed that makes the next release a **minor**. Do not ship the description change on its own and call it minor.

## Verified Facts

Established live on 2026-09-18 against an SAP S/4HANA on-premise system and an SAP ERP 6.0 EHP8 system. These drive every design decision below. Do not re-derive them, and where they contradict the issue text, they win.

1. `SUI_TM_MM_APP` holds one row per UIAD object. `APP_ID` is the key and equals the UIAD object name in TADIR. `CAT_ID` names the owning UIAC catalog.
2. `SUI_TM_MM_APP.CAT_ID` equals the UIAC object name in TADIR: customer catalogs appear in TADIR as `R3TR UIAC <name>` under exactly the string their app rows carry in `CAT_ID`.
3. SAP-delivered catalogs are **not** in TADIR at all — only customer catalogs are. This does not affect the implementation, which never reads TADIR, but an integration test must not look for its fixture there.
4. Both UIAC and UIAD are real `R3TR` TADIR objects for customer content, so both are transportable and both belong in a completeness check.
5. A UIAD is **not** a leaf. These columns carry a launch target: `TCODE` (transaction), `WD_APPL_ID` (Web Dynpro application), `WCF_TARGET_ID` (WebClient UI target), `UI5_APP_ID` (SAPUI5 app).
6. `APP_TYPE` does not predict which column is filled — type `W` rows carry both `WD_APPL_ID` and `TCODE`, type `C` rows carry both `WCF_TARGET_ID` and `TCODE`. Resolve **by column, not by app type**.
7. Observed `APP_TYPE` values: `T` transaction, `W` Web Dynpro application, `C` WebClient UI application, `U` SAPUI5 app, `G` template-based, `R` URL app. Type `R` rows carry no object reference at all.
8. `SUI_TM_MM_CAT` does **not** exist on the ECC system; `SUI_TM_MM_APP` does. A catalog-existence probe against `SUI_TM_MM_CAT` would fail hard there.
9. The ECC system carries UIAD objects in TADIR but no UIAC objects. An integration test must skip rather than fail where the object class is absent.
10. A `UI5_APP_ID` is a UI5 repository artifact, not resolvable in TADIR under that name. It is still reported, because a human reviewing a transport needs to know it is there.

## Deviations from the issue

Issue #409 proposes three things this plan deliberately does not do.

- **"Add two cases to the switch in `tools/search.go`."** That switch no longer lives in aibap.mcp; #417 moved the dependency engine into adtler on 2026-06-18. The change belongs in `adt/dependencies.go`.
- **"Confirm UIAC exists via `SUI_TM_MM_CAT`."** Dropped — Verified Fact 8. Querying `SUI_TM_MM_APP` alone works on both system types, and an empty result is already an honest answer.
- **"UIAD objects are leaf nodes."** Wrong — Verified Facts 5 and 6. A UIAD resolves to its launch target.

## Scope note (for the PR bodies and the redacted issue)

`SUI_TM_MM_APP` is Fiori launchpad designtime metadata, not application or business data. The query reads one row per repository object and returns only object names and launch-target names. That is the same class of read as the existing `D010TAB`, `DD0xL`, `SEOMETAREL` and `TFDIR` queries in `adt/dependencies.go`, and it matches `run_query`'s own `development_metadata` purpose. The change moves *toward* the scope guardrail: it replaces caller-authored SQL with two typed, bounded lookups. Two conditions hold it there — keep it to these two typed cases, and do not add a generic "read a Fiori config table" surface.

## File Structure

**adtler** (branch `feat/<N>-uiac-uiad-dependencies`, `<N>` = the adtler issue from Task 0)

- `adt/dependencies.go` — five new `UseType*` constants, `uiadTarget` type, `uiadTargetColumns` var, `uiacDeps` and `uiadDeps` methods, two `case` arms, updated doc comment and `default` error text.
- `adt/dependencies_test.go` (package `adt`) — table test for the column map.
- `adt/dependencies_http_test.go` (package `adt_test`) — `multiColumnDataPreview` helper plus three httptest-backed tests.
- `adt/uiac_dependencies_integration_test.go` (new, build tag `integration`) — `eachSystem(t)` test discovering its fixture at runtime.

**aibap.mcp** (branch `feat/409-uiac-uiad-dependencies`, already created)

- `tools/search.go` — extend the tool description, the `object_type` parameter text, the `object_name` examples.
- `tools/search_test.go` — assert the new types are advertised.
- `README.md` — the `get_object_dependencies` row still says "queries WBCROSSGT", which is stale; correct it and mention UIAC/UIAD.
- `go.mod` / `go.sum` — consume the new adtler version.

---

### Task 0: Process prerequisites (before any code)

aibap.mcp `CLAUDE.md` requires the blocked-by-adtler bookkeeping *immediately* once it is clear the fix needs an adtler change, and the bump PR later derives its `Closes` lines from the tracker. Doing this last would be circular.

**Files:** none (GitHub state)

- [ ] **Step 1: Confirm the aibap.mcp side**

`#409` is assigned and carries the `blocked-by-adtler` label already. Verify both still hold.

- [ ] **Step 2: File the adtler issue**

Body states: what UIAC and UIAD resolve to, Verified Facts 2, 5, 6 and 8, the scope note above, and a link to aibap.mcp#409 as the consumer issue. Note the issue number `<N>` — the branch name in Task 1 depends on it. Independent review of the body before posting.

- [ ] **Step 3: Add the reproducer snippet to aibap.mcp#409**

Required content per `CLAUDE.md`: the MCP tool name and an arguments JSON using `{"system": "<alias>"}`, the target system named by type and release level in prose, session preconditions if any, the "fixed" expected output and the "broken" current output. Cover **both** new object types — one `UIAC` call, one `UIAD` call — because the bump PR verifies one MCP call per fix claim. Use an SAP-delivered catalog name, never an internal one. Current broken output for both:

```
Error: unsupported object type "UIAC": supported are PROG, FUGR, FUNC, CLAS, INTF, TABL, DTEL, DOMA, TTYP
```

- [ ] **Step 4: Redact issue #409 and append the bullet to tracking issue #508**

Replace the internal-namespace object names and the internal catalog name in #409's body with SAP-delivered or placeholder names, and rewrite its implementation proposal to point at adtler. Do not repeat the redacted values in the edit comment. Then append to #508, in the documented format, linking the adtler issue from Step 2:

```
- [ ] #409 — get_object_dependencies rejects UIAC/UIAD (adtler: Hochfrequenz/adtler#<N>)
```

#508 stays open after #409 ticks — #377, #378 and #500 are still on it, so the "defer creation" clause does not apply. Both the rewritten body and the edit comment are substantive text: independent review before posting.

---

### Task 1: adtler — use-type constants and the column map

**Files:**
- Modify: `adt/dependencies.go` (the `UseType*` const block, new type and var below it)
- Test: `adt/dependencies_test.go`

**Interfaces:**
- Produces: `UseTypeUIApp`, `UseTypeTransaction`, `UseTypeWebDynproApp`, `UseTypeWebClientTarget`, `UseTypeUI5App`; `uiadTarget{Column, UseType string}`; `uiadTargetColumns` — Tasks 2 and 3 consume these.

Branch first: `git checkout -b feat/<N>-uiac-uiad-dependencies origin/main` in the adtler worktree, `<N>` from Task 0 Step 2.

- [ ] **Step 1: Write the failing test**

Add to `adt/dependencies_test.go` (package `adt`, so the unexported var is visible):

```go
func TestUIADTargetColumns(t *testing.T) {
	want := []struct{ col, use string }{
		{"TCODE", UseTypeTransaction},
		{"WD_APPL_ID", UseTypeWebDynproApp},
		{"WCF_TARGET_ID", UseTypeWebClientTarget},
		{"UI5_APP_ID", UseTypeUI5App},
	}
	if len(uiadTargetColumns) != len(want) {
		t.Fatalf("uiadTargetColumns: got %d entries, want %d", len(uiadTargetColumns), len(want))
	}
	for i, w := range want {
		if uiadTargetColumns[i].Column != w.col || uiadTargetColumns[i].UseType != w.use {
			t.Errorf("uiadTargetColumns[%d]: got %+v, want {%s %s}", i, uiadTargetColumns[i], w.col, w.use)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./adt/ -run TestUIADTargetColumns -v`
Expected: FAIL — compile error, `uiadTargetColumns` undefined.

- [ ] **Step 3: Write minimal implementation**

Extend the existing `UseType*` const block in `adt/dependencies.go` with these five. The block is `=`-aligned and the longest new name exceeds every current one, so gofmt will realign the whole block — run `gofmt -w adt/` before staging so the realignment lands in this commit:

```go
	// UseTypeUIApp is a UIAD app entry contained in a UIAC catalog.
	UseTypeUIApp = "UI_APP"
	// UseTypeTransaction is a transaction code launched by a UIAD app entry.
	UseTypeTransaction = "TRANSACTION"
	// UseTypeWebDynproApp is a Web Dynpro application launched by a UIAD app entry.
	UseTypeWebDynproApp = "WEB_DYNPRO_APP"
	// UseTypeWebClientTarget is a WebClient UI target launched by a UIAD app entry.
	UseTypeWebClientTarget = "WEB_CLIENT_TARGET"
	// UseTypeUI5App is a SAPUI5 repository app launched by a UIAD app entry.
	// Unlike the other targets it is not a TADIR object, so it cannot be
	// classified further; it is reported as a plain reference.
	UseTypeUI5App = "UI5_APP"
```

Then add below the const block:

```go
// uiadTarget names one SUI_TM_MM_APP column that may reference a launch target
// and the use type reported for it.
type uiadTarget struct {
	Column  string
	UseType string
}

// uiadTargetColumns lists the SUI_TM_MM_APP columns that carry a launch target,
// in the order they are reported. Resolution is by column rather than by
// APP_TYPE: a Web Dynpro app entry carries both WD_APPL_ID and TCODE, and a
// WebClient UI entry carries both WCF_TARGET_ID and TCODE, so keying off
// APP_TYPE would silently drop the second reference.
var uiadTargetColumns = []uiadTarget{
	{Column: "TCODE", UseType: UseTypeTransaction},
	{Column: "WD_APPL_ID", UseType: UseTypeWebDynproApp},
	{Column: "WCF_TARGET_ID", UseType: UseTypeWebClientTarget},
	{Column: "UI5_APP_ID", UseType: UseTypeUI5App},
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./adt/ -run TestUIADTargetColumns -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -w adt/ && go vet ./... && go test ./adt/...
git add adt/dependencies.go adt/dependencies_test.go
git commit -m "feat: add use types and column map for UIAC/UIAD dependencies"
```

---

### Task 2: adtler — UIAC resolves to its app entries

**Files:**
- Modify: `adt/dependencies.go` (new method `uiacDeps`, new `case "UIAC"`)
- Test: `adt/dependencies_http_test.go`

**Interfaces:**
- Consumes: `UseTypeUIApp` from Task 1.
- Produces: `func (c *httpClient) uiacDeps(ctx context.Context, catID string, maxResults int) ([]ObjectDependency, error)`.

- [ ] **Step 1: Write the failing test**

Add to `adt/dependencies_http_test.go`. It reuses `oneColumnDataPreview` (same file) and `csrfEndpoint` (`adt/client_test.go`, same package `adt_test`); no new imports needed:

```go
// TestGetObjectDependencies_UIAC exercises the UIAC path: the catalog query
// returns two app ids, reported as UI_APP dependencies. SUI_TM_MM_CAT must
// never be queried — that table is absent on ECC systems.
func TestGetObjectDependencies_UIAC(t *testing.T) {
	var sawCatTable bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		sql := string(bodyBytes)
		if strings.Contains(sql, "SUI_TM_MM_CAT") {
			sawCatTable = true
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.datapreview.table.v1+xml")
		if strings.Contains(sql, "FROM SUI_TM_MM_APP") {
			_, _ = w.Write([]byte(oneColumnDataPreview("APP_ID", "APPONE", "APPTWO")))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	c := adt.NewClient(cfg)
	res, err := c.GetObjectDependencies(context.Background(), "UIAC", "SAP_TC_EXAMPLE", 200, 3)
	if err != nil {
		t.Fatalf("GetObjectDependencies: %v", err)
	}
	if sawCatTable {
		t.Error("SUI_TM_MM_CAT was queried; it does not exist on ECC systems")
	}
	if res.Count != 2 {
		t.Fatalf("count: got %d, want 2", res.Count)
	}
	if res.Dependencies[0].Name != "APPONE" || res.Dependencies[0].UseType != adt.UseTypeUIApp {
		t.Errorf("dep[0]: got %+v, want {APPONE UI_APP}", res.Dependencies[0])
	}
	if res.Dependencies[1].Name != "APPTWO" {
		t.Errorf("dep[1] name: got %q, want APPTWO", res.Dependencies[1].Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./adt/ -run TestGetObjectDependencies_UIAC -v`
Expected: FAIL with `unsupported object type "UIAC"`.

- [ ] **Step 3: Write minimal implementation**

```go
// uiacDeps lists the UIAD app entries a UIAC catalog contains. The catalog
// itself is not probed: SUI_TM_MM_CAT is absent on ECC systems, and an empty
// SUI_TM_MM_APP result is already the correct answer for a catalog with no
// app entries.
func (c *httpClient) uiacDeps(ctx context.Context, catID string, maxResults int) ([]ObjectDependency, error) {
	qr, err := c.RunQuery(ctx,
		fmt.Sprintf("SELECT APP_ID FROM SUI_TM_MM_APP WHERE CAT_ID = '%s' ORDER BY APP_ID", EscapeValue(catID)),
		maxResults)
	if err != nil {
		return nil, err
	}
	if qr == nil {
		return nil, nil
	}
	idx := queryColumnIndexes(qr, "APP_ID")
	deps := make([]ObjectDependency, 0, len(qr.Rows))
	for _, row := range qr.Rows {
		if v := queryCell(row, idx["APP_ID"]); v != "" {
			deps = append(deps, ObjectDependency{Name: v, UseType: UseTypeUIApp})
		}
	}
	return deps, nil
}
```

And the case, after `case "INTF":` in `GetObjectDependencies`:

```go
	case "UIAC":
		deps, err := c.uiacDeps(ctx, objectName, maxResults)
		if err != nil {
			return nil, err
		}
		return newDependencyResult(objectType, objectName, deps, nil), nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./adt/ -run TestGetObjectDependencies_UIAC -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -w adt/ && go vet ./... && go test ./adt/...
git add adt/dependencies.go adt/dependencies_http_test.go
git commit -m "feat: resolve UIAC catalog dependencies to their UIAD app entries"
```

---

### Task 3: adtler — UIAD resolves to its launch target

**Files:**
- Modify: `adt/dependencies.go` (new method `uiadDeps`, new `case "UIAD"`, doc comment, `default` error)
- Test: `adt/dependencies_http_test.go`

**Interfaces:**
- Consumes: `uiadTargetColumns` from Task 1.
- Produces: `func (c *httpClient) uiadDeps(ctx context.Context, appID string) ([]ObjectDependency, []string, error)`.

**The constraint that matters here:** columns are resolved by name via `queryColumnIndexes` / `queryCell` (`adt/transport.go`), never by position. `transposeDataPreview` builds rows from the document order of the response's `<dataPreview:columns>` elements, which the endpoint is free to reorder or omit. The existing positional code elsewhere in `dependencies.go` only works because those SELECT lists happen to match the tables' physical field order; this query's order does not, so positional indexing would eventually report a transaction code as a Web Dynpro application.

- [ ] **Step 1: Write the failing tests**

Add the helper and both tests to `adt/dependencies_http_test.go`:

```go
// multiColumnDataPreview builds a one-row datapreview response over the given
// column/value pairs, in the order given.
func multiColumnDataPreview(cols [][2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n" +
		`<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">` + "\n" +
		`  <dataPreview:totalRows>1</dataPreview:totalRows>`)
	for _, c := range cols {
		b.WriteString("\n  <dataPreview:columns>\n" +
			`    <dataPreview:metadata dataPreview:name="` + c[0] + `" dataPreview:type="C" dataPreview:keyAttribute="false" dataPreview:colType="" dataPreview:isKeyFigure="false"/>` + "\n" +
			`    <dataPreview:dataSet><dataPreview:data>` + c[1] + `</dataPreview:data></dataPreview:dataSet>` + "\n" +
			"  </dataPreview:columns>")
	}
	b.WriteString("\n</dataPreview:tableData>")
	return b.String()
}

// TestGetObjectDependencies_UIAD_MultipleTargets covers a Web Dynpro app entry,
// which references both a Web Dynpro application and a transaction, so both are
// reported. The response deliberately lists its columns in an order matching
// neither the SELECT list nor uiadTargetColumns: the data-preview endpoint may
// reorder columns, so the implementation has to address them by name.
// Positional indexing passes a same-order fixture and fails this one.
func TestGetObjectDependencies_UIAD_MultipleTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.datapreview.table.v1+xml")
		_, _ = w.Write([]byte(multiColumnDataPreview([][2]string{
			{"UI5_APP_ID", ""},
			{"APP_TYPE", "W"},
			{"WD_APPL_ID", "WDA_EXAMPLE"},
			{"TCODE", "SE38"},
			{"WCF_TARGET_ID", ""},
		})))
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	c := adt.NewClient(cfg)
	res, err := c.GetObjectDependencies(context.Background(), "UIAD", "APPID00000000000000000000000001", 200, 3)
	if err != nil {
		t.Fatalf("GetObjectDependencies: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("count: got %d, want 2 (%+v)", res.Count, res.Dependencies)
	}
	if res.Dependencies[0].Name != "SE38" || res.Dependencies[0].UseType != adt.UseTypeTransaction {
		t.Errorf("dep[0]: got %+v, want {SE38 TRANSACTION}", res.Dependencies[0])
	}
	if res.Dependencies[1].Name != "WDA_EXAMPLE" || res.Dependencies[1].UseType != adt.UseTypeWebDynproApp {
		t.Errorf("dep[1]: got %+v, want {WDA_EXAMPLE WEB_DYNPRO_APP}", res.Dependencies[1])
	}
}

// TestGetObjectDependencies_UIAD_NoTarget covers a URL app entry: it exists but
// launches no repository object, reported as an empty list plus a warning
// rather than as an error.
func TestGetObjectDependencies_UIAD_NoTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.datapreview.table.v1+xml")
		_, _ = w.Write([]byte(multiColumnDataPreview([][2]string{
			{"APP_TYPE", "R"},
			{"TCODE", ""},
			{"WD_APPL_ID", ""},
			{"WCF_TARGET_ID", ""},
			{"UI5_APP_ID", ""},
		})))
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	c := adt.NewClient(cfg)
	res, err := c.GetObjectDependencies(context.Background(), "UIAD", "APPID00000000000000000000000001", 200, 3)
	if err != nil {
		t.Fatalf("GetObjectDependencies: %v", err)
	}
	if res.Count != 0 {
		t.Fatalf("count: got %d, want 0 (%+v)", res.Count, res.Dependencies)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "no launch target") {
		t.Errorf("warnings: got %v, want one mentioning \"no launch target\"", res.Warnings)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./adt/ -run TestGetObjectDependencies_UIAD -v`
Expected: FAIL with `unsupported object type "UIAD"`.

- [ ] **Step 3: Write minimal implementation**

```go
// uiadDeps resolves the launch target of a UIAD app entry. Every target column
// that is filled is reported; see uiadTargetColumns for why APP_TYPE is not
// used to select one. Columns are addressed by name because the data-preview
// endpoint may reorder or omit them. An app entry that launches no repository
// object (a URL app) yields no dependencies and one warning.
func (c *httpClient) uiadDeps(ctx context.Context, appID string) ([]ObjectDependency, []string, error) {
	names := make([]string, 0, len(uiadTargetColumns)+1)
	names = append(names, "APP_TYPE")
	for _, t := range uiadTargetColumns {
		names = append(names, t.Column)
	}
	qr, err := c.RunQuery(ctx,
		fmt.Sprintf("SELECT %s FROM SUI_TM_MM_APP WHERE APP_ID = '%s'",
			strings.Join(names, ", "), EscapeValue(appID)),
		1)
	if err != nil {
		return nil, nil, err
	}
	if qr == nil || len(qr.Rows) == 0 {
		return nil, []string{fmt.Sprintf("no SUI_TM_MM_APP entry for app id %q", appID)}, nil
	}
	idx := queryColumnIndexes(qr, names...)
	row := qr.Rows[0]
	deps := make([]ObjectDependency, 0, len(uiadTargetColumns))
	for _, t := range uiadTargetColumns {
		if v := queryCell(row, idx[t.Column]); v != "" {
			deps = append(deps, ObjectDependency{Name: v, UseType: t.UseType})
		}
	}
	if len(deps) == 0 {
		return nil, []string{fmt.Sprintf("app entry has no launch target (APP_TYPE %q)", queryCell(row, idx["APP_TYPE"]))}, nil
	}
	return deps, nil, nil
}
```

And the case, directly after `case "UIAC":`:

```go
	case "UIAD":
		deps, warns, err := c.uiadDeps(ctx, objectName)
		if err != nil {
			return nil, err
		}
		return newDependencyResult(objectType, objectName, deps, warns), nil
```

Update the `default` arm:

```go
	default:
		return nil, fmt.Errorf("unsupported object type %q: supported are PROG, FUGR, FUNC, CLAS, INTF, TABL, DTEL, DOMA, TTYP, UIAC, UIAD", objectType)
```

And append to the `GetObjectDependencies` doc comment, stating the ignored parameters explicitly — a UIAD row has at most four targets, so `maxResults` is not applied there:

```go
// UIAC (Fiori catalog) lists the UIAD app entries it contains, honouring
// maxResults; UIAD lists the launch target of a single app entry and ignores
// maxResults, since one app entry has at most four targets. Both read
// SUI_TM_MM_APP and both ignore maxDepth.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./adt/... -run TestGetObjectDependencies -v`
Expected: PASS, including the pre-existing PROG/CLAS/DDIC tests.

- [ ] **Step 5: Commit**

```bash
gofmt -w adt/ && go vet ./... && go test ./adt/...
git add adt/dependencies.go adt/dependencies_http_test.go
git commit -m "feat: resolve UIAD app entries to their launch target objects"
```

---

### Task 4: adtler — multi-system integration test

adtler's `CLAUDE.md` requires a multi-system integration test exercising the change's exact path on both system types. Verified Facts 8 and 9 are what this guards: the ECC system has `SUI_TM_MM_APP` but no `SUI_TM_MM_CAT` and no UIAC objects.

**Files:**
- Create: `adt/uiac_dependencies_integration_test.go` (build tag `integration`)

- [ ] **Step 1: Write the test**

Read `adt/integration_helpers_test.go` for `eachSystem(t)` and the `integrationSystem` fields, plus one existing integration test for house style, before writing. Requirements:

- Discover the fixture at runtime — select a `CAT_ID` from `SUI_TM_MM_APP` through the client's own query path and use what comes back. **Never hardcode a catalog or app name**; real names here are internal data and must not enter a public repository.
- `t.Skip` when the system has no rows to work with, rather than failing (Verified Fact 9).
- Call `GetObjectDependencies(ctx, "UIAC", <discovered catalog>, …)`; assert no error and that every dependency carries `UseTypeUIApp`.
- Take one app id from that result, call `GetObjectDependencies(ctx, "UIAD", <that app id>, …)`; assert no error and that each dependency's use type is one of the four launch-target constants — never `UseTypeUIApp`, which only a UIAC lookup returns — or, when the list is empty, that a warning explains why.
- Log counts, never object names, so CI logs carry no internal identifiers.

- [ ] **Step 2: Verify it builds and vets under the integration tag**

Run: `go build -tags integration ./adt/... && go vet -tags integration ./adt/...`
Expected: both clean.

- [ ] **Step 3: Commit**

```bash
gofmt -w adt/ && go test ./adt/...
git add adt/uiac_dependencies_integration_test.go
git commit -m "test: add multi-system integration test for UIAC/UIAD dependencies"
```

---

### Task 5: adtler — PR and the review cycle

**Files:** none (process task)

- [ ] **Step 1: Push and open the PR**

Body states: what the two object types resolve to; that columns are resolved by name and why; the three deviations from the requesting issue; the scope note; and the systems the behaviour was verified against, by type and release level. Link both the adtler issue and aibap.mcp#409. Add the `needs:integration-test` label. Independent review of the body before posting.

- [ ] **Step 2: Independent reviewer agent**

Per adtler `CLAUDE.md` step 2: a fresh agent with no prior context reads the PR diff and both issues in its own worktree, runs `go test ./...` and `go build -tags integration ./adt/...`, and posts a structured **GO** / **GO with notes** / **NO-GO** comment on the PR.

- [ ] **Step 3: CI green, then the real-SAP integration run**

Run the new integration test against both system types and post the per-system result on the PR — no credentials, no hostnames, no object names. Then remove `needs:integration-test`.

---

### Task 6: aibap.mcp — advertise the new object types

**Files:**
- Modify: `tools/search.go`, `README.md`
- Test: `tools/search_test.go`

- [ ] **Step 1: Write the failing test**

Uses `listRegisteredTools` / `listedTool` (`tools/elicitation_session_test.go`, same package `tools_test`) and `newTestServer` (`tools/source_test.go`). `"strings"` is not currently imported in `tools/search_test.go` — add it:

```go
// TestGetObjectDependenciesAdvertisesUITypes guards the tool surface: callers
// pick object types out of tools/list, so UIAC and UIAD have to be named both
// in the tool description and in the object_type parameter description.
func TestGetObjectDependenciesAdvertisesUITypes(t *testing.T) {
	s := newTestServer(&mockClient{})
	var tool listedTool
	for _, lt := range listRegisteredTools(t, s) {
		if lt.Name == "get_object_dependencies" {
			tool = lt
			break
		}
	}
	if tool.Name == "" {
		t.Fatal("get_object_dependencies is not registered")
	}
	props, _ := tool.InputSchema["properties"].(map[string]any)
	objType, _ := props["object_type"].(map[string]any)
	paramDesc, _ := objType["description"].(string)
	for _, want := range []string{"UIAC", "UIAD"} {
		if !strings.Contains(tool.Description, want) {
			t.Errorf("tool description does not mention %s", want)
		}
		if !strings.Contains(paramDesc, want) {
			t.Errorf("object_type description does not mention %s: %q", want, paramDesc)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tools/ -run TestGetObjectDependenciesAdvertisesUITypes -v`
Expected: FAIL — description does not mention UIAC.

- [ ] **Step 3: Write minimal implementation**

In `tools/search.go`, extend the supported-types list inside `mcp.WithDescription`, matching the existing line formatting exactly:

```go
				"  UIAC — Fiori catalog: queries SUI_TM_MM_APP (CAT_ID) for the UIAD app entries it contains\n"+
				"  UIAD — Fiori catalog app entry: queries SUI_TM_MM_APP (APP_ID) for its launch target "+
				"(transaction, Web Dynpro application, WebClient UI target or SAPUI5 app)\n"+
```

Update the `object_type` parameter description to list both new types, and append a UIAC example to the `object_name` description using an SAP-delivered catalog name.

Then fix `README.md`: the `get_object_dependencies` row claims it "queries WBCROSSGT", wrong since the engine moved to D010TAB/DD0xL/SEOMETAREL. Correct it and mention the Fiori catalog types.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./... && go test ./...
git add tools/search.go tools/search_test.go README.md
git commit -m "feat(#409): advertise UIAC/UIAD support on get_object_dependencies"
```

---

### Task 7: aibap.mcp — consume the adtler change and open the PR

**Files:** `go.mod`, `go.sum`, plus a throwaway verification harness

`CLAUDE.md` prefers the automated dependabot bump path. This change rides in the #409 feature branch rather than a separate `chore/bump-adtler-…` PR, because the description edit and the bump are only meaningful together — state that explicitly in the PR body, and note that the tracker bullet for #409 is satisfied by this PR's `Closes #409`.

- [ ] **Step 1: Pin adtler**

Published tag: `go get github.com/Hochfrequenz/adtler@vX.Y.Z`. Not yet published: pin the merged commit, yielding a pseudo-version, and keep the PR a **draft** — pseudo-versions must never reach `main`. Once the real tag publishes, push a follow-up commit re-pinning to `@vX.Y.Z` before marking the PR ready.

```bash
go get github.com/Hochfrequenz/adtler@<version-or-sha> && go mod tidy && go test ./...
```

- [ ] **Step 2: Reproducer harness**

Add `tools/bump_<version>_verify_integration_test.go` (build tag `integration`, functions prefixed `TestBumpVerify_`, header comment `// Delete after the bump PR merges.`) carrying #409's reproducer from Task 0 Step 3. List it in the PR's Test Plan as a follow-up deletion item and remove it before merge.

- [ ] **Step 3: Verify live on both system types**

Run the reproducer against the S/4 system and the ECC system. The UIAC call is expected to return app entries on S/4; on ECC, where no UIAC objects exist, an empty result without error is the pass condition — the case the dropped `SUI_TM_MM_CAT` probe would have broken. Record both outputs for the PR body, with no object names.

- [ ] **Step 4: Open the PR**

`Closes #409`. State the new tool surface and that the next release is a minor. Independent review of the body before posting.

---

### Task 8: Close the loop

- [ ] **Step 1:** Tick #409's bullet on tracking issue #508. #508 stays open — #377, #378 and #500 remain on it.
- [ ] **Step 2:** Remove `blocked-by-adtler` from #409 once the bump has merged.
- [ ] **Step 3:** Delete the throwaway harness file from Task 7 Step 2.
