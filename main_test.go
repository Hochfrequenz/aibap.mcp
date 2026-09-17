package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// TestConsentFlagReachesTheToolList is the guard for the one step nothing else
// covers: the path from the --consent flag value to the annotation a client
// actually receives. The tools-package tests build their own server, and
// ConsentStrict is the zero value, so dropping tools.WithConsentMode(consent)
// from the registration would leave every other test green while
// --consent=prompt silently stopped doing anything.
//
// It goes through buildServer — the same function run() calls — rather than
// repeating the registration, so it fails on that deletion instead of
// reproducing it.
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

			annotated := len(annotatedToolNames(t, s))
			switch {
			case tc.wantAnnotated && annotated == 0:
				t.Errorf("--consent=%q annotated no tool; the flag is not reaching registration", tc.flag)
			case !tc.wantAnnotated && annotated != 0:
				t.Errorf("--consent=%q annotated %d tool(s); prompt mode must annotate none", tc.flag, annotated)
			}
		})
	}
}

// TestApprovalSectionNamesEveryAnnotatedTool keeps the APPROVAL paragraph in
// the server instructions from drifting away from the tools actually annotated.
// The paragraph is what the model reads before choosing a tool, so a name
// missing from it — or one listed that is no longer annotated — misinforms the
// only reader it has.
func TestApprovalSectionNamesEveryAnnotatedTool(t *testing.T) {
	instructions := serverInstructions([]string{"SYS_A"}, "SYS_A", false)
	s := buildServer(nil, nil, tools.DefaultGroups(), nil, tools.ConsentStrict, "")

	annotated := annotatedToolNames(t, s)
	if len(annotated) == 0 {
		t.Fatal("strict mode annotated no tool — the wiring under test is broken")
	}
	for _, name := range annotated {
		if !strings.Contains(instructions, name) {
			t.Errorf(
				"%s requires approval on every call but is not named in the APPROVAL section; "+
					"add it there when adding it to tools.irreversibleTools",
				name,
			)
		}
	}
}

// annotatedToolNames lists the tools carrying the requires-user-interaction
// key in the server's tools/list response — the wire form a client sees.
func annotatedToolNames(t *testing.T, s *server.MCPServer) []string {
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
	var names []string
	for _, tool := range envelope.Result.Tools {
		if tool.Meta[tools.MetaRequiresUserInteraction] == true {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	return names
}

// serverInstructions must only advertise the debugger capability when the
// opt-in "debug" group is enabled — otherwise the instructions promise tools
// the client cannot see (#429). The debugger tools themselves are untouched.
func TestServerInstructions_DebugLineIsConditional(t *testing.T) {
	const debugClaim = "Debugging (breakpoints, stepping, variable inspection)"
	// Assert the leading newline too, so a dropped "\n" (which would run the
	// bullet onto the DDIC line) is caught rather than passing on the bare claim.
	const debugBullet = "\n- " + debugClaim

	withDebug := serverInstructions([]string{"HF S/4"}, "HF S/4", true)
	if !strings.Contains(withDebug, debugBullet) {
		t.Errorf("debug enabled: instructions should advertise debugging as its own bullet, got:\n%s", withDebug)
	}

	withoutDebug := serverInstructions([]string{"HF S/4"}, "HF S/4", false)
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
