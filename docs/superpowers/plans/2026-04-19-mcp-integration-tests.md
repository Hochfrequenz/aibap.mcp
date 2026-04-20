# MCP-layer Integration Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `//go:build integration`-gated tests in `tools/` that exercise five read-only MCP tools (plus one negative test) end-to-end against real SAP systems (`hfq` + `s4u`), with loud skips when systems or fixtures are unreachable.

**Architecture:** One shared in-process `server.MCPServer` constructed in `TestMain` with a real adtler `ClientRegistry` over all reachable target systems. Tests iterate `MCP_INTEGRATION_SYSTEMS` (default `hfq,s4u`), call `select_system` per subtest to switch, then invoke each tool via the existing `callTool()` JSON-RPC helper and assert on the JSON shape (not SAP content — that's adtler's job). Reachability and fixture presence are pre-checked, skipped loudly (`t.Logf` + `t.Skipf`) when absent.

**Tech Stack:** Go 1.26, `github.com/Hochfrequenz/adtler` (real ADT client), `github.com/mark3labs/mcp-go`, `//go:build integration` build tag. No new third-party dependencies.

**Spec:** `docs/superpowers/specs/2026-04-19-mcp-integration-tests-design.md`

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `tools/testhelper_test.go` | **create** (no build tag) | Shared helpers reusable by unit + integration tests: `callTool` (extracted from `source_test.go`) and nothing else for now. |
| `tools/source_test.go` | **modify** | Remove the `callTool` function (lines ~308-343) — now lives in `testhelper_test.go`. Keep `newTestServer*` helpers and `mockClient` in place (those stay unit-test-only). |
| `tools/integration_test.go` | **create** (`//go:build integration`) | `TestMain`, target-system parsing, reachability probe, `newIntegrationServer`, skip helpers, all six integration test functions. |
| `.env` | **delete** | Stale leftover with unused `SAP_INTEGRATION_*` values. Confirmed nothing in the codebase reads it. |
| `README.md` | **modify** | New "Running integration tests locally" subsection under Testing. |
| `CLAUDE.md` | **modify** | Testing section paragraph distinguishing unit vs. integration tests. |

Boundaries: `testhelper_test.go` stays tag-less so integration tests can use `callTool` without pulling in all of `source_test.go`'s mock client machinery. `integration_test.go` contains only integration-specific code, all behind the build tag.

---

## Task 1: Extract `callTool` into `tools/testhelper_test.go`

Refactor only, no behavior change. Proves unit tests still work after the move.

**Files:**
- Create: `tools/testhelper_test.go`
- Modify: `tools/source_test.go` (remove one function)

- [ ] **Step 1: Create `tools/testhelper_test.go`**

```go
package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// callTool invokes a tool via HandleMessage using JSON-RPC protocol.
// Shared by unit tests (in *_test.go files without build tags) and
// integration tests (in integration_test.go behind //go:build integration).
func callTool(t *testing.T, s *server.MCPServer, toolName string, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		toolName, string(argsJSON))

	resp := s.HandleMessage(context.Background(), []byte(msg))

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var envelope struct {
		Result *mcp.CallToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("unmarshal response envelope: %v\nraw: %s", err, string(respBytes))
	}
	if envelope.Error != nil {
		t.Fatalf("JSON-RPC error calling %q: code=%d msg=%s", toolName, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		t.Fatalf("nil result for tool %q\nraw: %s", toolName, string(respBytes))
	}
	return envelope.Result
}
```

- [ ] **Step 2: Remove the `callTool` function from `tools/source_test.go`**

Delete lines 308-344 (the `// callTool invokes...` comment through the closing brace). Also delete the `"fmt"` import if it becomes unused — run `go build ./...` to check; reinstate the import if other code in `source_test.go` still needs it.

- [ ] **Step 3: Run unit tests, verify green**

Run: `go test ./...`
Expected: all packages pass. `go vet ./...` also clean.

If anything red: re-add whatever you accidentally broke. Do not proceed.

- [ ] **Step 4: Commit**

```bash
git add tools/testhelper_test.go tools/source_test.go
git commit -m "$(cat <<'EOF'
test(#277): extract callTool helper into tools/testhelper_test.go

Prepare for integration tests behind //go:build integration that reuse
callTool without pulling in source_test.go's mock machinery.
No behavior change.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Create `integration_test.go` scaffolding + target-system env parsing

Lay down the build-tagged test file with `TestMain` stub, a pure helper for parsing `MCP_INTEGRATION_SYSTEMS`, and a unit test for that helper. The unit test runs only with `-tags integration`, so it doesn't affect default `go test ./...`.

**Files:**
- Create: `tools/integration_test.go`

- [ ] **Step 1: Write the failing unit test for `parseTargetSystems`**

Create `tools/integration_test.go` with this initial content:

```go
//go:build integration

package tools_test

import (
	"reflect"
	"testing"
)

func TestParseTargetSystems(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"empty falls back to default", "", []string{"hfq", "s4u"}},
		{"single value", "hfq", []string{"hfq"}},
		{"comma separated", "hfq,s4u", []string{"hfq", "s4u"}},
		{"whitespace trimmed", " hfq , s4u ", []string{"hfq", "s4u"}},
		{"empty entries skipped", "hfq,,s4u,", []string{"hfq", "s4u"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTargetSystems(c.env)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseTargetSystems(%q) = %v; want %v", c.env, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, verify it fails to compile**

Run: `go test -tags integration -run TestParseTargetSystems ./tools/...`
Expected: compile error `undefined: parseTargetSystems`.

- [ ] **Step 3: Implement `parseTargetSystems`**

Append to `tools/integration_test.go`:

```go
import "strings"

// parseTargetSystems splits MCP_INTEGRATION_SYSTEMS into a slice of system
// keys. Empty input returns the default [hfq, s4u]. Entries are trimmed;
// empty entries are dropped.
func parseTargetSystems(env string) []string {
	if strings.TrimSpace(env) == "" {
		return []string{"hfq", "s4u"}
	}
	raw := strings.Split(env, ",")
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if trimmed := strings.TrimSpace(e); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"hfq", "s4u"}
	}
	return out
}
```

Note: Go does not allow two `import` blocks. Merge into the existing `import` block at the top of the file so it becomes:

```go
import (
	"reflect"
	"strings"
	"testing"
)
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test -tags integration -run TestParseTargetSystems -v ./tools/...`
Expected: `PASS` with 5 subtests.

- [ ] **Step 5: Run default `go test` to confirm no regression**

Run: `go test ./...`
Expected: all packages pass. The integration file is excluded without `-tags integration`.

- [ ] **Step 6: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): scaffold integration_test.go + target-system env parsing

//go:build integration guard, parseTargetSystems helper driven by
MCP_INTEGRATION_SYSTEMS (default hfq,s4u), table-driven unit test.
No SAP access required for this layer.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `TestMain` — config load, reachability probe, per-system status map

Wire up the real environment: load `systems.json`, build a `ClientRegistry` over the **intersection** of `MCP_INTEGRATION_SYSTEMS` and what's actually in the config, probe each for reachability, and store the result in a package-level map consulted by test skip helpers.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Add package-level state, `TestMain`, and skip helpers**

Append to `tools/integration_test.go`:

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)

// Package-level state populated by TestMain and consulted by every
// integration subtest.
var (
	integrationSystems []string           // parsed MCP_INTEGRATION_SYSTEMS
	appConfig          *config.AppConfig  // loaded from SAP_CONFIG_FILE / default path
	registry           *adt.ClientRegistry // shared across all tests
	sharedServer       *server.MCPServer   // one MCP server with all reachable systems
	reachable          = map[string]bool{} // systemKey -> reachable
	reachableMu        sync.RWMutex
)

func TestMain(m *testing.M) {
	integrationSystems = parseTargetSystems(os.Getenv("MCP_INTEGRATION_SYSTEMS"))

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: config.Load(%q) failed: %v\n", cfgPath, err)
		os.Exit(2)
	}
	appConfig = cfg

	// Build adtler clients for target systems that are actually present in config.
	clients, err := adt.NewClientsFromConfig(&cfg.Config, "mcp-server-abap")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: NewClientsFromConfig failed: %v\n", err)
		os.Exit(2)
	}

	// Restrict to intersection of requested and configured systems.
	inConfig := func(k string) bool { _, ok := clients[k]; return ok }
	var present []string
	for _, sys := range integrationSystems {
		if inConfig(sys) {
			present = append(present, sys)
		} else {
			fmt.Fprintf(os.Stderr, "integration tests: SKIP system=%s reason=not-in-config (MCP_INTEGRATION_SYSTEMS=%v)\n",
				sys, integrationSystems)
		}
	}
	if len(present) == 0 {
		fmt.Fprintf(os.Stderr, "integration tests: no requested systems present in config %q; requested=%v\n",
			cfgPath, integrationSystems)
		os.Exit(2)
	}

	// Probe reachability. A "cheap" ping: search_objects for a nonsense pattern.
	// If the ADT client returns without a network-level error we call it reachable.
	// A 404/empty-result is still "reachable" — the server answered.
	ctx := context.Background()
	for _, sys := range present {
		client := clients[sys]
		_, err := client.SearchObjects(ctx, "ZZZZZ_PING_DO_NOT_EXIST_*", "", 1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "integration tests: SKIP system=%s reason=unreachable err=%v\n", sys, err)
			reachable[sys] = false
			continue
		}
		reachable[sys] = true
	}

	// Build a registry over the present systems. Prefer the first *reachable*
	// system as the initial default; otherwise fall back to the first present
	// system so the registry still constructs (tests will just t.Skip).
	registryClients := map[string]adt.Client{}
	for _, sys := range present {
		registryClients[sys] = clients[sys]
	}
	defaultSys := present[0]
	for _, sys := range present {
		if reachable[sys] {
			defaultSys = sys
			break
		}
	}
	registry, err = adt.NewClientRegistry(registryClients, defaultSys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: NewClientRegistry failed: %v\n", err)
		os.Exit(2)
	}

	sharedServer = server.NewMCPServer("mcp-server-abap-integration", "0.0.0")
	tools.RegisterAllWithLockMap(
		sharedServer,
		registry,
		registry, // ClientRegistry implements SystemSelector
		adt.NewLockMap(),
		tools.ParseToolGroups([]string{"all"}),
		nil, nil,
	)

	printReachabilitySummary(present)

	code := m.Run()
	os.Exit(code)
}

// resolveConfigPath returns SAP_CONFIG_FILE if set, otherwise the default
// ~/.config/sap-mcp/systems.json (matching main.go).
func resolveConfigPath() string {
	if p := os.Getenv("SAP_CONFIG_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "systems.json"
	}
	return filepath.Join(home, ".config", "sap-mcp", "systems.json")
}

func isReachable(sys string) bool {
	reachableMu.RLock()
	defer reachableMu.RUnlock()
	return reachable[sys]
}

// requireReachable skips the subtest if the named system is not reachable.
// Logs loudly so the skip reason shows up in test output regardless of -v.
func requireReachable(t *testing.T, sys string) {
	t.Helper()
	if !isReachable(sys) {
		t.Logf("SKIP system=%s reason=unreachable-or-missing — see TestMain log", sys)
		t.Skipf("system %q unreachable or not in config", sys)
	}
}

// printReachabilitySummary prints a single-line, grep-friendly summary so
// developers can tell at a glance which systems were actually covered.
func printReachabilitySummary(present []string) {
	parts := make([]string, 0, len(present))
	for _, sys := range present {
		status := "UNREACHABLE"
		if reachable[sys] {
			status = "OK"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", sys, status))
	}
	fmt.Fprintf(os.Stderr, "integration targets: ")
	for i, p := range parts {
		if i > 0 {
			fmt.Fprint(os.Stderr, " ")
		}
		fmt.Fprint(os.Stderr, p)
	}
	fmt.Fprintln(os.Stderr)
}
```

Merge new imports into the existing `import` block at the top so it becomes:

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/server"
)
```

- [ ] **Step 2: Verify it compiles and the existing `parseTargetSystems` test still passes**

Run: `go test -tags integration -run TestParseTargetSystems -v ./tools/...`
Expected: `PASS`. The unit test doesn't touch SAP. Any compile error fails here.

Note: if you don't have a valid `systems.json` locally, `TestMain` will `os.Exit(2)` before `m.Run()` gets called, and the parse test won't run. That's acceptable — the purpose of the file is to run against a real environment.

- [ ] **Step 3: Verify default `go test` still passes**

Run: `go test ./...`
Expected: green. Integration file is excluded without the tag.

- [ ] **Step 4: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestMain with config load + reachability probe

Build a real ClientRegistry over MCP_INTEGRATION_SYSTEMS ∩ configured
systems, probe each with a cheap search_objects call, store result in
a package-level reachability map, print a grep-friendly summary line.
requireReachable helper surfaces skip reasons loudly in test output.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Fixture-presence helper (`requireFixture`) and `mustSelectSystem`

Small glue helpers used by every per-tool test. Both wrap `callTool` and are trivial to write — no SAP access required to add them.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Append helpers to `tools/integration_test.go`**

```go
// mustSelectSystem calls the select_system tool and fails the test on error.
// Should be called at the top of every per-system subtest so the shared
// server points at the right system for the subsequent tool call.
func mustSelectSystem(t *testing.T, s *server.MCPServer, sys string) {
	t.Helper()
	res := callTool(t, s, "select_system", map[string]interface{}{
		"system": sys,
	})
	if res.IsError {
		t.Fatalf("select_system(%q) unexpectedly returned IsError=true: %s", sys, textOf(res))
	}
}

// requireFixture skips the subtest if the named ABAP object is missing on
// the currently-selected system. Uses object_exists (read-only).
// Caller must have already called mustSelectSystem for `sys`.
func requireFixture(t *testing.T, s *server.MCPServer, sys, objectURI string) {
	t.Helper()
	res := callTool(t, s, "object_exists", map[string]interface{}{
		"object_uri": objectURI,
	})
	if res.IsError {
		t.Logf("SKIP system=%s fixture=%s reason=object_exists-errored text=%s", sys, objectURI, textOf(res))
		t.Skipf("fixture %q missing or unreachable on %s — install Hochfrequenz/Z_ADT_MCP_TEST", objectURI, sys)
	}

	// object_exists returns JSON like {"exists": true/false}.
	var payload struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
		t.Fatalf("requireFixture: could not parse object_exists result %q: %v", textOf(res), err)
	}
	if !payload.Exists {
		t.Logf("SKIP system=%s fixture=%s reason=not-installed", sys, objectURI)
		t.Skipf("fixture %q not installed on %s — install Hochfrequenz/Z_ADT_MCP_TEST", objectURI, sys)
	}
}

// textOf extracts the Text content from a CallToolResult, or "" if empty.
func textOf(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
```

Add the new imports to the top block: `"encoding/json"` and `"github.com/mark3labs/mcp-go/mcp"`.

- [ ] **Step 2: Confirm `object_exists` really returns `{"exists": ...}`**

Quick sanity check — grep for the tool's JSON output shape:

Run: `grep -n 'json.Marshal' tools/objects.go 2>/dev/null || grep -rn 'object_exists' tools/*.go | head -5`
Expected: find the `object_exists` tool registration. Read the marshalled map literal. If the JSON key is not `"exists"`, **update `requireFixture`** to match.

If the tool returns a different shape (e.g., `{"object_exists": true}` or a plain string), adjust the `payload` struct and JSON tag. Do **not** proceed until the real shape is known and the helper matches it.

- [ ] **Step 3: Verify it compiles**

Run: `go test -tags integration -run TestParseTargetSystems -v ./tools/...`
Expected: `PASS` (still no new test added, just making sure the file still compiles).

- [ ] **Step 4: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add mustSelectSystem + requireFixture helpers

mustSelectSystem switches the shared MCP server to a target system.
requireFixture probes object_exists and t.Skipf's loudly when the
Z_ADT_MCP_TEST fixture isn't installed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `TestIntegration_SelectSystem` — first real integration test

Smallest end-to-end test that doesn't need a fixture. Proves the whole harness works.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_SelectSystem(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)

			res := callTool(t, sharedServer, "select_system", map[string]interface{}{
				"system": sys,
			})
			if res.IsError {
				t.Fatalf("select_system(%q) returned IsError=true: %s", sys, textOf(res))
			}
			msg := textOf(res)
			if !strings.Contains(msg, sys) {
				t.Errorf("select_system response %q does not mention %q", msg, sys)
			}

			// Follow-up: ActiveName on the registry should now equal sys.
			if got := registry.ActiveName(); got != sys {
				t.Errorf("registry.ActiveName() = %q; want %q", got, sys)
			}
		})
	}
}
```

- [ ] **Step 2: Run against your local environment**

Run: `go test -tags integration -v -count=1 -run TestIntegration_SelectSystem ./tools/...`

Expected output (if both systems reachable):
```
=== RUN   TestIntegration_SelectSystem
=== RUN   TestIntegration_SelectSystem/hfq
--- PASS: TestIntegration_SelectSystem/hfq (…s)
=== RUN   TestIntegration_SelectSystem/s4u
--- PASS: TestIntegration_SelectSystem/s4u (…s)
PASS
integration targets: hfq=OK s4u=OK
```

If one system is unreachable (VPN off, etc.), that subtest prints `--- SKIP` with a message pointing at the TestMain log. Overall result is still `PASS`.

If the test fails — e.g., the response doesn't mention the system name — it's either (a) a genuine wrapper regression (fix it) or (b) the assertion is too strict for your environment (loosen the `strings.Contains` check, commit message should explain why).

- [ ] **Step 3: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_SelectSystem

First real integration test. Iterates MCP_INTEGRATION_SYSTEMS, calls
select_system per subtest, asserts the response text references the
target system and registry.ActiveName() matches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `TestIntegration_SearchObjects`

Exercises MCP arg parsing for multi-param tools (`query`, `object_type`, `max_results`) and asserts on an array-shaped JSON response.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_SearchObjects(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "search_objects", map[string]interface{}{
				"query":       "Z_ADT_MCP_TEST*",
				"max_results": 50,
			})
			if res.IsError {
				t.Fatalf("search_objects returned IsError=true: %s", textOf(res))
			}

			// ObjectInfo has no JSON tags → fields are capitalized.
			var results []struct {
				Name string
				Type string
				URI  string
			}
			if err := json.Unmarshal([]byte(textOf(res)), &results); err != nil {
				t.Fatalf("unmarshal search_objects result: %v\nraw: %s", err, textOf(res))
			}

			if len(results) == 0 {
				t.Skipf("Z_ADT_MCP_TEST* returned no results on %s — install Hochfrequenz/Z_ADT_MCP_TEST", sys)
			}

			found := false
			for _, r := range results {
				if strings.HasPrefix(strings.ToUpper(r.Name), "Z_ADT_MCP_TEST") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no result matched Z_ADT_MCP_TEST* prefix; got %d results, first: %+v", len(results), results[0])
			}
		})
	}
}
```

- [ ] **Step 2: Run against your environment**

Run: `go test -tags integration -v -count=1 -run TestIntegration_SearchObjects ./tools/...`

Expected: PASS per reachable system with the fixture package installed. If SAP returns an empty result set, the test skips with a clear message rather than failing — installing `Z_ADT_MCP_TEST` fixes it.

- [ ] **Step 3: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_SearchObjects

Exercises multi-param tool arg parsing and top-level JSON array
response shape. Skips (not fails) when Z_ADT_MCP_TEST* returns no
matches, pointing at the fixture package install.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `TestIntegration_GetSource` — lock-map-integrated path

Exercises the single-URI `get_source` flow, including the lock-map write after read. Asserts on the `{source, etag}` wrapper shape.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_GetSource(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "get_source", map[string]interface{}{
				"object_uri": uri,
			})
			if res.IsError {
				t.Fatalf("get_source returned IsError=true: %s", textOf(res))
			}

			var payload struct {
				Source string `json:"source"`
				ETag   string `json:"etag"`
			}
			if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
				t.Fatalf("unmarshal get_source result: %v\nraw: %s", err, textOf(res))
			}
			if payload.Source == "" {
				t.Errorf("get_source returned empty source; raw: %s", textOf(res))
			}
			if !strings.Contains(strings.ToUpper(payload.Source), "REPORT") {
				t.Errorf("get_source body lacks REPORT keyword; got: %q", payload.Source)
			}
			// ETag is optional on some systems/objects — don't assert on it.
		})
	}
}
```

- [ ] **Step 2: Run**

Run: `go test -tags integration -v -count=1 -run TestIntegration_GetSource$ ./tools/...`

(The `$` anchor avoids matching `TestIntegration_GetSource_NonExistent` added in Task 10.)

Expected: PASS per reachable system with `Z_ADT_MCP_TEST_REPORT` installed. Skipped loudly if not.

- [ ] **Step 3: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_GetSource

Asserts {source, etag} wrapper shape on Z_ADT_MCP_TEST_REPORT and
checks source body contains the REPORT keyword.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `TestIntegration_GetTextElements`

Weakest-assertion test since the fixture has no text elements. Proves the wrapper doesn't crash and returns valid JSON (possibly `{}`).

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_GetTextElements(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "get_text_elements", map[string]interface{}{
				"object_uri": uri,
			})
			if res.IsError {
				t.Fatalf("get_text_elements returned IsError=true: %s", textOf(res))
			}

			// TextElements fields are `symbols`/`selections` with omitempty.
			// The fixture has neither, so we only assert that the body parses
			// as a JSON object. This catches wrapper-layer regressions like
			// "tool returned empty content" or "returned invalid JSON".
			// Stronger assertions require extending the fixture (see spec).
			var payload map[string]json.RawMessage
			if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
				t.Fatalf("get_text_elements did not return a JSON object: %v\nraw: %s", err, textOf(res))
			}
			// Permitted shapes: {}, {"symbols":...}, {"selections":...}, or both.
			for k := range payload {
				if k != "symbols" && k != "selections" {
					t.Errorf("unexpected key %q in get_text_elements result; full body: %s", k, textOf(res))
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run**

Run: `go test -tags integration -v -count=1 -run TestIntegration_GetTextElements ./tools/...`

Expected: PASS. A raw body of `{}` is valid and expected for this fixture.

- [ ] **Step 3: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_GetTextElements

Asserts wrapper returns a valid JSON object (possibly empty) and
contains only the expected symbols/selections keys. Fixture has no
text elements so this is deliberately a weak assertion — follow-up
issue tracks extending Z_ADT_MCP_TEST for stronger coverage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `TestIntegration_SyntaxCheck`

Uses the `Z_ADT_MCP_TEST_SYNWARN` fixture (declares an unused variable → emits a `W` warning).

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_SyntaxCheck(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_synwarn"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "syntax_check", map[string]interface{}{
				"object_uri": uri,
			})
			if res.IsError {
				t.Fatalf("syntax_check returned IsError=true: %s", textOf(res))
			}

			// Single-URI call returns a top-level JSON array of SyntaxMessage.
			// SyntaxMessage has no JSON tags → fields are capitalized.
			var msgs []struct {
				Type   string
				Text   string
				Line   int
				Column int
			}
			if err := json.Unmarshal([]byte(textOf(res)), &msgs); err != nil {
				t.Fatalf("unmarshal syntax_check result: %v\nraw: %s", err, textOf(res))
			}

			sawWarning := false
			for _, m := range msgs {
				if m.Type == "W" {
					sawWarning = true
					break
				}
			}
			if !sawWarning {
				t.Errorf("expected at least one W message from SYNWARN fixture; got %+v", msgs)
			}
		})
	}
}
```

- [ ] **Step 2: Run**

Run: `go test -tags integration -v -count=1 -run TestIntegration_SyntaxCheck ./tools/...`

Expected: PASS. If the fixture is installed but the system's syntax checker emits a different type (e.g., `I` instead of `W` for newer kernel versions), adjust the assertion with a comment explaining why.

- [ ] **Step 3: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_SyntaxCheck

Runs syntax_check against the Z_ADT_MCP_TEST_SYNWARN fixture (which
declares an unused variable) and asserts at least one "W" entry
appears in the top-level JSON array.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `TestIntegration_GetSource_NonExistent` — error round-trip

Verifies SAP-side errors become `IsError=true` `CallToolResult`s, not Go `error` returns from the handler.

**Files:**
- Modify: `tools/integration_test.go`

- [ ] **Step 1: Write the test**

Append to `tools/integration_test.go`:

```go
func TestIntegration_GetSource_NonExistent(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			// A URI that should never exist. Using a prefix that has no
			// realistic chance of collision.
			uri := "/sap/bc/adt/programs/programs/zzzz_not_a_real_object_" + sys

			res := callTool(t, sharedServer, "get_source", map[string]interface{}{
				"object_uri": uri,
			})
			if !res.IsError {
				t.Errorf("expected IsError=true for non-existent object; got body: %s", textOf(res))
			}
			if textOf(res) == "" {
				t.Errorf("expected non-empty error text for non-existent object")
			}
		})
	}
}
```

- [ ] **Step 2: Run**

Run: `go test -tags integration -v -count=1 -run TestIntegration_GetSource_NonExistent ./tools/...`

Expected: PASS. The tool wrapper should surface SAP's 404 as an MCP error result with explanatory text.

If the test fails — `res.IsError == false` — that's a **real wrapper-layer bug**: the handler is silently returning a success result for a missing object. Fix the wrapper before shipping this PR.

- [ ] **Step 3: Run the entire suite together to check cross-test interactions**

Run: `go test -tags integration -v -count=1 ./tools/...`

Expected: all `TestIntegration_*` tests PASS (or skip loudly with clear reasons). The reachability summary line should appear at the start of the output.

- [ ] **Step 4: Commit**

```bash
git add tools/integration_test.go
git commit -m "$(cat <<'EOF'
test(#277): add TestIntegration_GetSource_NonExistent

Negative test: verifies SAP 404s round-trip as MCP IsError=true
with non-empty error text, not Go error returns from the handler.
Catches regressions where the wrapper silently returns success for
missing objects.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Docs — README + CLAUDE.md; delete stale `.env`

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Delete: `.env`

- [ ] **Step 1: Confirm `.env` is untracked and safe to delete**

Run: `git ls-files --error-unmatch .env 2>&1 || echo "UNTRACKED"`
Expected: `UNTRACKED` (the `.env` at repo root was never committed; it's local dev clutter). If the output says anything else (i.e., the file IS tracked), stop and ask before deleting — the assumption was wrong.

- [ ] **Step 2: Delete `.env`**

Run: `rm -f .env`
Verify: `ls -la .env 2>&1 | head -1` — should report `No such file`.

- [ ] **Step 3: Add README section under Testing**

In `README.md`, find the existing "Testing" heading and append this subsection below whatever's already there (paste the content between the two `~~~markdown` fences — the fences themselves are plan-rendering scaffolding, not part of the README):

~~~markdown
### Running integration tests locally

MCP-layer integration tests live in `tools/integration_test.go` behind `//go:build integration`. They're never run by default `go test ./...`.

**Prerequisites:**

- VPN connectivity to the target SAP system(s)
- `~/.config/sap-mcp/systems.json` configured (see Setup above)
- `Z_ADT_MCP_TEST` package installed on each target system — see [Hochfrequenz/Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST)

**Run:**

```bash
go test -tags integration -v -count=1 ./tools/...
```

**Target specific systems:**

```bash
MCP_INTEGRATION_SYSTEMS=hfq go test -tags integration -v -count=1 ./tools/...
```

The default target set is `hfq,s4u`.

**Coverage visibility:** TestMain prints a grep-friendly summary at the top, e.g. `integration targets: hfq=OK s4u=UNREACHABLE`. Always check this — subtests skip loudly when a system or fixture is unreachable rather than failing, so it's possible to get a green `go test` without actually covering everything.
~~~

- [ ] **Step 4: Update CLAUDE.md Testing section**

In `CLAUDE.md`, find the existing Testing section bullet list (around the `- **Test-driven**` bullet). Replace the `- **Unit tests**` and `- **Integration tests**` bullets (or add them if missing) with:

```markdown
- **Unit tests**: `go test ./...` — mock adtler clients, always run, must pass before committing.
- **Integration tests**: `go test -tags integration -v -count=1 ./tools/...` — local-only, real SAP via `~/.config/sap-mcp/systems.json`, requires VPN + `Z_ADT_MCP_TEST` package installed. Covers the MCP wrapper layer only (adtler owns the ADT HTTP/XML/auth layer). Target systems via `MCP_INTEGRATION_SYSTEMS` (default `hfq,s4u`).
```

Adjust wording to match the surrounding style if it diverges.

- [ ] **Step 5: Confirm nothing else references `.env`**

Run: `grep -rn "\.env" --include="*.go" --include="*.md" --include="*.yml" --include="*.yaml" . 2>/dev/null | grep -v node_modules | grep -v docs/superpowers/plans | grep -v docs/superpowers/specs`
Expected: no references to the deleted file. If anything turns up (e.g., a workflow that sources it), fix it in this commit.

- [ ] **Step 6: Verify tests all still pass**

Run: `go test ./...`
Expected: green.

Run (only if you have SAP access): `go test -tags integration -v -count=1 ./tools/...`
Expected: green with the reachability summary line printed.

- [ ] **Step 7: Commit**

```bash
git add README.md CLAUDE.md
git add -u .env   # record the deletion
git commit -m "$(cat <<'EOF'
docs(#277): document integration tests + remove stale .env

README gets a "Running integration tests locally" subsection covering
prerequisites, run command, system targeting via MCP_INTEGRATION_SYSTEMS,
and the coverage-visibility summary line.
CLAUDE.md Testing section now distinguishes unit vs. integration tests.
Deletes the untracked-but-noise .env at repo root (never read by code).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 8: Final check — PR readiness**

Run: `go test ./... && go vet ./... && gofmt -l .`
Expected: green; `gofmt -l` prints nothing. If `gofmt` prints any file, run `gofmt -w` on it and amend or add a follow-up commit.

The branch `feat/277-mcp-integration-tests` is now ready for `gh pr create`. Use a PR body that lists:

- What the PR adds (6 integration tests, TestMain harness, skip helpers)
- How to run them (`go test -tags integration ...`)
- `Closes #277`

---

## Self-Review Notes

Cross-check against the spec:

- **Build-tag gating** → Task 2 (the `//go:build integration` line).
- **Config source via `SAP_CONFIG_FILE` / sap-mcp-config** → Task 3 (`resolveConfigPath`, `config.Load`).
- **Stale `.env` deletion** → Task 11.
- **`MCP_INTEGRATION_SYSTEMS` env-driven target list** → Task 2 (helper + unit test) + Task 3 (consumed by TestMain).
- **Reachability skip + fixture skip, both loud** → Task 3 (`requireReachable`), Task 4 (`requireFixture`), plus `t.Logf`+`t.Skipf` pattern throughout.
- **Grep-friendly summary line** → Task 3 (`printReachabilitySummary`).
- **Shared server with all reachable systems + `select_system` per subtest** → Task 3 (`sharedServer` global), Task 4 (`mustSelectSystem`), Task 5 onwards use it.
- **Helper extraction** → Task 1.
- **`newIntegrationServer` function** — the spec describes it as a standalone helper. **In the plan it's inlined into `TestMain`** because there's only ever one instance and no reason to build multiple. Both outcomes are equivalent; executor should not re-introduce the helper unless a second server is ever needed.
- **All 6 tests with shape assertions** → Tasks 5 through 10, each matching the spec's assertion matrix.
- **Docs (README + CLAUDE.md)** → Task 11.
- **Out-of-scope items (write ops, CI wiring, release-gate script)** — not in any task; out-of-scope per spec.

No placeholders. No "similar to Task N" references. All code blocks are complete and self-contained. Task ordering is sequential — each task builds on the file state left by the previous one.
