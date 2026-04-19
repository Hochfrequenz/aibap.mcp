//go:build integration

package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
		{"all-empty entries fall back to default", ",,,", []string{"hfq", "s4u"}},
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

// Package-level state populated by TestMain and consulted by every
// integration subtest. All writes happen in TestMain before m.Run() is
// called; subtests only read. No mutex needed because Go's testing
// framework establishes a happens-before relationship between TestMain
// and the tests it runs.
var (
	integrationSystems []string            // parsed MCP_INTEGRATION_SYSTEMS
	appConfig          *config.AppConfig   // loaded from SAP_CONFIG_FILE / default path
	registry           *adt.ClientRegistry // shared across all tests
	sharedServer       *server.MCPServer   // one MCP server with all reachable systems
	reachable          = map[string]bool{} // systemKey -> reachable
)

func TestMain(m *testing.M) {
	integrationSystems = parseTargetSystems(os.Getenv("MCP_INTEGRATION_SYSTEMS"))

	// Initialize every requested system as unreachable; we'll flip to true
	// only after a successful probe. This way any downstream failure (config
	// missing, client build failure, ping failure) leaves requireReachable
	// with a clear skip signal.
	for _, sys := range integrationSystems {
		reachable[sys] = false
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: config.Load(%q) failed: %v — all systems will be SKIPPED\n", cfgPath, err)
		printReachabilitySummary(integrationSystems)
		os.Exit(m.Run())
	}
	appConfig = cfg

	clients, err := adt.NewClientsFromConfig(&cfg.Config, "mcp-server-abap")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: NewClientsFromConfig failed: %v — all systems will be SKIPPED\n", err)
		printReachabilitySummary(integrationSystems)
		os.Exit(m.Run())
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
		fmt.Fprintf(os.Stderr, "integration tests: no requested systems present in config %q; requested=%v — all SKIPPED\n",
			cfgPath, integrationSystems)
		printReachabilitySummary(integrationSystems)
		os.Exit(m.Run())
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

	// Build a registry over the present systems. Prefer the first reachable
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
	reg, err := adt.NewClientRegistry(registryClients, defaultSys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests: NewClientRegistry failed: %v — all systems will be SKIPPED\n", err)
		// Mark all unreachable and let tests skip.
		for _, sys := range integrationSystems {
			reachable[sys] = false
		}
		printReachabilitySummary(integrationSystems)
		os.Exit(m.Run())
	}
	registry = reg

	sharedServer = server.NewMCPServer("mcp-server-abap-integration", "0.0.0")
	tools.RegisterAllWithLockMap(
		sharedServer,
		registry,
		registry, // ClientRegistry implements SystemSelector
		adt.NewLockMap(),
		tools.ParseToolGroups([]string{"all"}),
		nil, nil,
	)

	printReachabilitySummary(integrationSystems)

	os.Exit(m.Run())
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
// Accepts the full list of originally-requested systems (not just the
// ones present in config) so not-in-config systems still show as
// UNREACHABLE in the summary — their reachable[sys] stays false by
// default, which is exactly the signal callers want.
func printReachabilitySummary(systems []string) {
	parts := make([]string, 0, len(systems))
	for _, sys := range systems {
		status := "UNREACHABLE"
		if reachable[sys] {
			status = "OK"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", sys, status))
	}
	fmt.Fprintf(os.Stderr, "integration targets: %s\n", strings.Join(parts, " "))
}

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

	// object_exists returns JSON like {"exists": true/false, ...}.
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
				// adtler gates GetTextElements on ADT discovery. Neither hfq nor
				// s4u currently advertise /sap/bc/adt/textelements/programs in
				// their discovery document, so the pre-check returns "not
				// available on this system" before any HTTP call is made.
				// On S/4 the endpoint does in fact work — the discovery check
				// is a known adtler bug (see spec
				// docs/superpowers/specs/2026-04-06-set-text-elements-design.md,
				// "Fix discovery bug"). Until that's fixed, this test skips
				// rather than fails. Any other IsError still fails loudly.
				if strings.Contains(textOf(res), "not available on this system") {
					t.Skipf("SKIP system=%s reason=text-elements-endpoint-not-in-discovery (adtler discovery bug): %s", sys, textOf(res))
				}
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
