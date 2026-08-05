//go:build integration

package tools_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestIntegrationRunClass executes the ZCL_ADT_MCP_CLASSRUN_TST fixture on
// each configured system and asserts the known console output. Verified live
// 2026-07-23: the console string CLASSRUN_OK is present on both HFQ and S4U,
// but written differently (HFQ: out->write_text, S4U: out->write) — so the
// assertion below MUST be a substring check, never string equality.
func TestIntegrationRunClass(t *testing.T) {
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_classrun_tst"
	const className = "ZCL_ADT_MCP_CLASSRUN_TST"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "run_class", map[string]interface{}{
				"class_name": className,
			})
			if res.IsError {
				t.Fatalf("run_class(%q) returned IsError=true: %s", className, textOf(res))
			}

			// Parse the typed ClassRunResult and assert on named fields (matching
			// the harness convention, e.g. TestIntegration_GetSource). A raw
			// substring on the whole JSON blob would still pass if a
			// serialization regression swapped the two fields or emitted the
			// output under the wrong key.
			var payload struct {
				ClassName     string `json:"class_name"`
				ConsoleOutput string `json:"console_output"`
			}
			if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
				t.Fatalf("unmarshal run_class result: %v\nraw: %s", err, textOf(res))
			}
			// The tool must echo back the class it actually ran.
			if payload.ClassName != className {
				t.Errorf("class_name: got %q, want %q", payload.ClassName, className)
			}
			// The sentinel must land in console_output specifically — it appears
			// in neither the class name nor any request argument, so a match
			// here can only come from the class actually executing.
			if !strings.Contains(payload.ConsoleOutput, "CLASSRUN_OK") {
				t.Errorf("console_output missing CLASSRUN_OK; got: %q", payload.ConsoleOutput)
			}
		})
	}
}

// TestIntegrationRunClass_DataWrite executes the ZCL_ADT_MCP_CLASSRUN_DATA
// fixture, which calls out->write( data = lt_rows name = 'ADT_MCP_CLASSRUN_DATA' )
// on a two-row internal table (issue #465 Part B: this call form is not
// derivable from the tool description alone, so it needs its own coverage
// rather than an assertion on the description string).
//
// Verified live 2026-08-05: the row values (and column header) are rendered
// identically on both systems, but the `name` label is only rendered on S4U —
// HFQ silently drops it. Mirrors the divergence pattern already documented in
// TestIntegrationRunClass_Error: assert what is common, document what isn't.
func TestIntegrationRunClass_DataWrite(t *testing.T) {
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_classrun_data"
	const className = "ZCL_ADT_MCP_CLASSRUN_DATA"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "run_class", map[string]interface{}{
				"class_name": className,
			})
			if res.IsError {
				t.Fatalf("run_class(%q) returned IsError=true: %s", className, textOf(res))
			}

			var payload struct {
				ClassName     string `json:"class_name"`
				ConsoleOutput string `json:"console_output"`
			}
			if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
				t.Fatalf("unmarshal run_class result: %v\nraw: %s", err, textOf(res))
			}
			// The tool must echo back the class it actually ran.
			if payload.ClassName != className {
				t.Errorf("class_name: got %q, want %q", payload.ClassName, className)
			}

			// Row values are the part of the contract every system honours —
			// these can only appear here via the data-dump form actually running.
			for _, want := range []string{"ADT_MCP_ROW_ONE", "ADT_MCP_ROW_TWO"} {
				if !strings.Contains(payload.ConsoleOutput, want) {
					t.Errorf("console_output missing row value %q; got: %q", want, payload.ConsoleOutput)
				}
			}

			if strings.EqualFold(sys, "hfq") {
				t.Logf("HFQ drops the out->write name= label; row-value check above is the coverage for this system")
				return
			}
			if !strings.Contains(payload.ConsoleOutput, "ADT_MCP_CLASSRUN_DATA") {
				t.Errorf("console_output missing the name= label on %s; got: %q", sys, payload.ConsoleOutput)
			}
		})
	}
}

// TestIntegrationRunClass_Error executes the throwing ZCL_ADT_MCP_CLASSRUN_ERR
// fixture. Verified live 2026-07-23: this fixture diverges by system. On HFQ
// it raises cx_sy_zerodivide (uncaught -> adtler maps it to a *adt.ADTError
// with status >= 500 -> run_class returns IsError=true). On S4U the class is
// an empty CREATE PRIVATE stub that does not implement IF_OO_ADT_CLASSRUN, so
// it would not produce the same clean error signal. Per project-owner
// decision, the error assertion runs on HFQ only; every other system skips.
func TestIntegrationRunClass_Error(t *testing.T) {
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_classrun_err"
	const className = "ZCL_ADT_MCP_CLASSRUN_ERR"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			if !strings.EqualFold(sys, "hfq") {
				t.Skipf("ERR fixture only reliably throws on HFQ; %s fixture diverges", sys)
				return
			}

			res := callTool(t, sharedServer, "run_class", map[string]interface{}{
				"class_name": className,
			})
			if !res.IsError {
				t.Fatalf("run_class(%q) expected IsError=true on HFQ, got success: %s", className, textOf(res))
			}
			txt := textOf(res)
			t.Logf("HFQ run_class(%q) error text: %s", className, txt)

			// IsError alone is not enough: the handler's own existence pre-check
			// (GetObjectInfo) also yields IsError=true, with a "does not exist"
			// message, BEFORE RunClass is ever called. This test must prove the
			// classrun runtime-exception path was exercised — not the pre-check
			// or any unrelated error. adtler maps an uncaught ABAP exception
			// (cx_sy_zerodivide here) to a *adt.ADTError with status >= 500,
			// rendered as "SAP ADT error 5xx: ...".
			if strings.Contains(txt, "does not exist") {
				t.Fatalf("run_class hit the existence pre-check, not the classrun runtime-exception path: %s", txt)
			}
			if !strings.Contains(txt, "SAP ADT error 5") {
				t.Errorf("expected a 5xx server dump (cx_sy_zerodivide), got: %s", txt)
			}
		})
	}
}
