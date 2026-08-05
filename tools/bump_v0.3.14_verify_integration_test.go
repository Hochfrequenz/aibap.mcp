//go:build integration

// Delete after the bump PR merges.

package tools_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBumpVerify_RunClassLongRunning is the reproducer from issue #465 Part A:
// adtler v0.3.13's RunClass went through the 30-second short-timeout HTTP
// client, capping every classrun regardless of the caller's needs. adtler
// v0.3.14 (Hochfrequenz/adtler#115) moves RunClass onto the long-timeout
// client with a 5-minute default deadline, matching RunQuery.
//
// Fixture: ZCL_AIBAP_TIMEOUT in $TMP on s4u, a busy loop that runs for 45s
// before writing "elapsed 45.00 s" via out->write. Before the bump this fails
// with "context deadline exceeded (Client.Timeout exceeded while awaiting
// headers)" at ~30s; after the bump it must return cleanly.
func TestBumpVerify_RunClassLongRunning(t *testing.T) {
	const sys = "S4U"
	const uri = "/sap/bc/adt/oo/classes/zcl_aibap_timeout"
	const className = "ZCL_AIBAP_TIMEOUT"

	requireReachable(t, sys)
	mustSelectSystem(t, sharedServer, sys)
	requireFixture(t, sharedServer, sys, uri)

	res := callTool(t, sharedServer, "run_class", map[string]interface{}{
		"class_name": className,
	})
	if res.IsError {
		t.Fatalf("run_class(%q) returned IsError=true (still capped at 30s?): %s", className, textOf(res))
	}

	var payload struct {
		ClassName     string `json:"class_name"`
		ConsoleOutput string `json:"console_output"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
		t.Fatalf("unmarshal run_class result: %v\nraw: %s", err, textOf(res))
	}
	if !strings.Contains(payload.ConsoleOutput, "elapsed 45.00 s") {
		t.Errorf("console_output missing the 45s completion marker; got: %q", payload.ConsoleOutput)
	}
}
