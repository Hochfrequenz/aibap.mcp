//go:build integration

package tools_test

import (
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

			if !strings.Contains(textOf(res), "CLASSRUN_OK") {
				t.Errorf("run_class(%q) console output missing CLASSRUN_OK; got: %s", className, textOf(res))
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
			if textOf(res) == "" {
				t.Errorf("run_class(%q) expected non-empty error text on HFQ", className)
			}
		})
	}
}
