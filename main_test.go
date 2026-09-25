package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// setHomeDir points os.UserHomeDir() at dir for the duration of the test,
// regardless of platform (Windows reads USERPROFILE, everything else HOME).
func setHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// TestFindConfigFile covers #528: a stale cwd-relative config.json must not
// silently shadow the documented ~/.config/sap-mcp/systems.json default.
func TestFindConfigFile(t *testing.T) {
	t.Run("documented default wins when both exist", func(t *testing.T) {
		home := t.TempDir()
		setHomeDir(t, home)
		documented := filepath.Join(home, ".config", "sap-mcp", "systems.json")
		if err := os.MkdirAll(filepath.Dir(documented), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(documented, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := os.WriteFile(filepath.Join(cwd, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := findConfigFile(); got != documented {
			t.Errorf("findConfigFile() = %q, want documented default %q", got, documented)
		}
	})

	t.Run("falls back to cwd config.json when documented default is absent", func(t *testing.T) {
		setHomeDir(t, t.TempDir()) // no ~/.config/sap-mcp/systems.json created

		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := os.WriteFile(filepath.Join(cwd, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := findConfigFile(); got != "config.json" {
			t.Errorf("findConfigFile() = %q, want %q", got, "config.json")
		}
	})

	t.Run("neither exists: points at the documented default for a clear error", func(t *testing.T) {
		home := t.TempDir()
		setHomeDir(t, home)
		t.Chdir(t.TempDir())

		want := filepath.Join(home, ".config", "sap-mcp", "systems.json")
		if got := findConfigFile(); got != want {
			t.Errorf("findConfigFile() = %q, want %q", got, want)
		}
	})
}

// TestConsentFlagReachesTheToolList covers the step from a parsed consent mode
// to the annotation a client actually receives. The tools-package tests build
// their own server, and ConsentStrict is the zero value, so dropping
// tools.WithConsentMode(consent) from the registration would leave every other
// test green while --consent=prompt silently stopped doing anything. It goes
// through buildServer — the same function run() calls — so it fails on that
// deletion rather than reproducing it.
//
// What it does not cover, because run() is not callable from a test: the two
// lines in run() that read the flag and hand the parsed mode to buildServer.
// Hard-coding a mode there would still pass.
// wantAnnotatedCount is the size of tools.irreversibleTools. Spelled out here
// so annotating a subset by accident fails rather than passing a "more than
// zero" check; the names themselves are pinned in the tools package.
const wantAnnotatedCount = 6

func TestConsentFlagReachesTheToolList(t *testing.T) {
	for _, tc := range []struct {
		flag          string
		wantAnnotated bool
	}{
		{flag: "", wantAnnotated: true},
		{flag: "strict", wantAnnotated: true},
		{flag: "prompt", wantAnnotated: false},
	} {
		t.Run("consent_"+tc.flag, func(t *testing.T) {
			consent, err := tools.ParseConsentMode(tc.flag)
			if err != nil {
				t.Fatalf("ParseConsentMode(%q): %v", tc.flag, err)
			}
			// A nil client, selector and fallback are enough: buildServer
			// only captures them, and tools/list never invokes a handler.
			s := buildServer(nil, nil, tools.DefaultGroups(), nil, consent, "")

			annotated := annotatedToolNames(t, s)
			switch {
			case tc.wantAnnotated && len(annotated) != wantAnnotatedCount:
				t.Errorf(
					"--consent=%q annotated %v; expected all %d irreversible tools",
					tc.flag, annotated, wantAnnotatedCount,
				)
			case !tc.wantAnnotated && len(annotated) != 0:
				t.Errorf("--consent=%q annotated %v; prompt mode must annotate none", tc.flag, annotated)
			}
		})
	}
}

// TestApprovalSectionNamesEveryAnnotatedTool keeps the APPROVAL paragraph in
// the server instructions from drifting away from the tools actually annotated.
// The paragraph is what the model reads before choosing a tool, so a name
// missing from it misinforms the only reader it has.
//
// The search is scoped to the paragraph rather than run over the whole
// instructions text: release_transport and run_class are both named in the
// BEST FOR list above it, so a whole-text search would pass for those two
// however the paragraph read.
func TestApprovalSectionNamesEveryAnnotatedTool(t *testing.T) {
	section := approvalSection(t, serverInstructions([]string{"SYS_A"}, "SYS_A", false))
	s := buildServer(nil, nil, tools.DefaultGroups(), nil, tools.ConsentStrict, "")

	annotated := annotatedToolNames(t, s)
	if len(annotated) == 0 {
		t.Fatal("strict mode annotated no tool — the wiring under test is broken")
	}
	for _, name := range annotated {
		if !strings.Contains(section, name) {
			t.Errorf(
				"%s requires approval on every call but is not named in the APPROVAL section; "+
					"add it there when adding it to tools.irreversibleTools",
				name,
			)
		}
	}

	// The other direction: a name left in the paragraph after the tool stopped
	// being annotated tells the model to expect a prompt that never comes.
	for _, name := range toolNamesIn(t, s) {
		if !strings.Contains(section, name) {
			continue
		}
		if !slices.Contains(annotated, name) {
			t.Errorf(
				"the APPROVAL section names %s, which is not annotated; remove it there when "+
					"removing it from tools.irreversibleTools",
				name,
			)
		}
	}
}

// approvalSection returns the first paragraph after the APPROVAL heading —
// the one that enumerates the tools requiring approval. Assertions are scoped
// to it for two reasons: a whole-instructions search would be satisfied by the
// BEST FOR list above, and the paragraphs below it deliberately name tools
// that are *not* annotated, such as run_query.
func approvalSection(t *testing.T, instructions string) string {
	t.Helper()
	const heading = "APPROVAL:\n"
	start := strings.Index(instructions, heading)
	if start == -1 {
		t.Fatalf("instructions have no %q heading", strings.TrimSpace(heading))
	}
	rest := instructions[start+len(heading):]
	if end := strings.Index(rest, "\n\n"); end != -1 {
		return rest[:end]
	}
	t.Fatal("the APPROVAL section is not followed by a blank line — cannot scope the assertion")
	return ""
}

// toolNamesIn lists every tool the server exposes.
func toolNamesIn(t *testing.T, s *server.MCPServer) []string {
	t.Helper()
	names, _ := listedTools(t, s)
	return names
}

// annotatedToolNames lists the tools carrying the requires-user-interaction
// key in the server's tools/list response — the wire form a client sees.
func annotatedToolNames(t *testing.T, s *server.MCPServer) []string {
	t.Helper()
	_, annotated := listedTools(t, s)
	return annotated
}

// listedTools returns every tool name and the annotated subset, read from the
// server's tools/list response.
func listedTools(t *testing.T, s *server.MCPServer) (all, annotated []string) {
	t.Helper()
	resp := s.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	var envelope struct {
		Result struct {
			Tools []struct {
				Name string         `json:"name"`
				Meta map[string]any `json:"_meta,omitempty"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if len(envelope.Result.Tools) == 0 {
		t.Fatal("tools/list returned zero tools — test server misconfigured")
	}
	for _, tool := range envelope.Result.Tools {
		all = append(all, tool.Name)
		if tool.Meta[tools.MetaRequiresUserInteraction] == true {
			annotated = append(annotated, tool.Name)
		}
	}
	sort.Strings(all)
	sort.Strings(annotated)
	return all, annotated
}

// serverInstructions must only advertise the debugger capability when the
// opt-in "debug" group is enabled — otherwise the instructions promise tools
// the client cannot see (#429). The debugger tools themselves are untouched.
func TestServerInstructions_DebugLineIsConditional(t *testing.T) {
	const debugClaim = "Debugging (breakpoints, stepping, variable inspection)"
	// Assert the leading newline too, so a dropped "\n" (which would run the
	// bullet onto the DDIC line) is caught rather than passing on the bare claim.
	const debugBullet = "\n- " + debugClaim

	withDebug := serverInstructions([]string{"SYS_A"}, "SYS_A", true)
	if !strings.Contains(withDebug, debugBullet) {
		t.Errorf("debug enabled: instructions should advertise debugging as its own bullet, got:\n%s", withDebug)
	}

	withoutDebug := serverInstructions([]string{"SYS_A"}, "SYS_A", false)
	if strings.Contains(withoutDebug, debugClaim) {
		t.Errorf("debug disabled: instructions must NOT advertise debugging, got:\n%s", withoutDebug)
	}
}

// The non-conditional parts of the blurb (and the system list) must always be
// present regardless of the debug flag — guards against the %s rewrite dropping
// or misordering content.
func TestServerInstructions_AlwaysPresentContent(t *testing.T) {
	for _, debug := range []bool{true, false} {
		got := serverInstructions([]string{"SYS_A", "SYS_B"}, "SYS_A", debug)
		for _, want := range []string{
			"BEST FOR:",
			"get_source, patch_source",
			"DDIC lookups (get_object_info, get_ddic_info)",
			// run_class is in the default-on "system" group, so unlike the
			// debugger bullet this one is unconditional (#473).
			"run_class: runs a global, active class implementing IF_OO_ADT_CLASSRUN",
			// Approval is the client's, not this server's: nothing here asks
			// a second time, and a refusal comes from the client rather than
			// from SAP (#507, and the honesty rule #475 established).
			"APPROVAL:",
			"This server does not ask for confirmation itself",
			"refused by the client, not by SAP",
			// The marking only asks; clients may ignore it, and older ones
			// do. Stating it as enforcement would be the same class of
			// untrue documentation #475 set out to remove.
			"The marking is a request, not a guarantee",
			// run_query's purpose gate is a scope check, not an approval step,
			// and must not be described as one (#507).
			"run_query is not part of that set",
			"SAP API POLICY",
			// The policy must stay tool-agnostic: run_class can reach business
			// data by SELECTing inside ABAP, bypassing run_query's purpose gate.
			"the restriction follows the data being touched, not the tool used to touch it",
			"AVAILABLE SYSTEMS: SYS_A, SYS_B (default: \"SYS_A\")",
			"Use select_system to switch between systems.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("debug=%v: instructions missing %q", debug, want)
			}
		}
	}
}
