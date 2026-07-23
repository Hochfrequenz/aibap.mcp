package main

import (
	"strings"
	"testing"
)

// serverInstructions must only advertise the debugger capability when the
// opt-in "debug" group is enabled — otherwise the instructions promise tools
// the client cannot see (#429). The debugger tools themselves are untouched.
func TestServerInstructions_DebugLineIsConditional(t *testing.T) {
	const debugClaim = "Debugging (breakpoints, stepping, variable inspection)"

	withDebug := serverInstructions([]string{"HF S/4"}, "HF S/4", true)
	if !strings.Contains(withDebug, debugClaim) {
		t.Errorf("debug enabled: instructions should advertise debugging, got:\n%s", withDebug)
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
			"SAP API POLICY",
			"AVAILABLE SYSTEMS: SYS_A, SYS_B (default: \"SYS_A\")",
			"Use select_system to switch between systems.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("debug=%v: instructions missing %q", debug, want)
			}
		}
	}
}
