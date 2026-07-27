//go:build integration

// Bump-verify harness for the adtler v0.3.13 bump. Collects the reproducer from
// the linked issue (#460, classrun load generation) and drives it end-to-end
// through the real MCP tool handlers against live SAP.
//
// Delete after the bump PR merges (tracked in the PR Test Plan).
//
//	MCP_INTEGRATION_SYSTEMS="hfq,s4u" \
//	  go test -tags integration -run TestBumpVerify ./tools/...
//
// Reuses the shared harness in integration_test.go (sharedServer,
// mustSelectSystem, requireReachable, callTool, textOf).

package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rc460Source returns a minimal IF_OO_ADT_CLASSRUN class writing marker.
// Double-quoted + \n (no Go backtick) per CLAUDE.md.
func rc460Source(marker string) string {
	return "CLASS zcl_adt_mcp_rc460 DEFINITION PUBLIC FINAL CREATE PUBLIC.\n" +
		"  PUBLIC SECTION.\n" +
		"    INTERFACES if_oo_adt_classrun.\n" +
		"ENDCLASS.\n\n" +
		"CLASS zcl_adt_mcp_rc460 IMPLEMENTATION.\n" +
		"  METHOD if_oo_adt_classrun~main.\n" +
		"    out->write( '" + marker + "' ).\n" +
		"  ENDMETHOD.\n" +
		"ENDCLASS.\n"
}

// TestBumpVerify_460_ClassrunFreshAndStale reproduces aibap.mcp#460 through the
// MCP tools: create a $TMP classrun class, then across one session
// set-source → activate → run_class through two versions.
//
//   - Defect 1 (fresh class): the FIRST run must return the class's real output
//     (V1), not "does not implement if_oo_adt_classrun~main". Pre-adtler-v0.3.13
//     this soft-failed on S/4.
//   - Defect 2 (stale after re-activation): the SECOND run (after changing the
//     source to V2 and re-activating) must return V2, not the stale V1.
//
// Both are fixed in adtler v0.3.13 (RunClass runs each classrun on an isolated
// fresh session). ECC/R3 always worked and acts as the regression guard.
func TestBumpVerify_460_ClassrunFreshAndStale(t *testing.T) {
	const name = "ZCL_ADT_MCP_RC460"
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_rc460"

	runExpect := func(t *testing.T, sys, want string) {
		t.Helper()
		res := callTool(t, sharedServer, "run_class", map[string]interface{}{"class_name": name})
		if res.IsError {
			t.Fatalf("%s: run_class returned IsError: %s", sys, textOf(res))
		}
		var payload struct {
			ClassName     string `json:"class_name"`
			ConsoleOutput string `json:"console_output"`
		}
		if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
			t.Fatalf("%s: unmarshal run_class result: %v\nraw: %s", sys, err, textOf(res))
		}
		if !strings.Contains(payload.ConsoleOutput, want) {
			t.Errorf("%s: console_output %q does not contain %q — #460 not fixed at this version",
				sys, payload.ConsoleOutput, want)
		}
		t.Logf("%s: run_class -> %q (want %q)", sys, strings.TrimSpace(payload.ConsoleOutput), want)
	}

	setAndActivate := func(t *testing.T, sys, marker string) {
		t.Helper()
		file := filepath.Join(t.TempDir(), "src.txt")
		if err := os.WriteFile(file, []byte(rc460Source(marker)), 0o644); err != nil {
			t.Fatalf("%s: write temp source: %v", sys, err)
		}
		if r := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
			"object_uri": uri, "file_path": file,
		}); r.IsError {
			t.Fatalf("%s: set_source_from_file (%s): %s", sys, marker, textOf(r))
		}
		if r := callTool(t, sharedServer, "activate_object", map[string]interface{}{
			"object_uri": uri,
		}); r.IsError {
			t.Fatalf("%s: activate_object (%s): %s", sys, marker, textOf(r))
		}
	}

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			// Fresh $TMP class (no transport). Tolerate a leftover from a prior run.
			if createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "CLAS", "name": name, "package": "$TMP",
				"description": "aibap #460 classrun bump reproducer",
			}); createR.IsError {
				exR := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
				var ex struct {
					Exists bool `json:"exists"`
				}
				if exR.IsError || json.Unmarshal([]byte(textOf(exR)), &ex) != nil || !ex.Exists {
					t.Fatalf("%s: create_object(CLAS $TMP): %s", sys, textOf(createR))
				}
				t.Logf("%s: class %s already exists, reusing", sys, name)
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				if d := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": uri}); d.IsError {
					t.Logf("%s: WARNING cleanup could not delete %s: %s", sys, name, textOf(d))
				}
			})

			// Defect 1: fresh class must run and return its real output.
			setAndActivate(t, sys, "RC460_V1")
			runExpect(t, sys, "RC460_V1")

			// Defect 2: after change + re-activate, run must return the NEW version.
			setAndActivate(t, sys, "RC460_V2")
			runExpect(t, sys, "RC460_V2")
		})
	}
}
