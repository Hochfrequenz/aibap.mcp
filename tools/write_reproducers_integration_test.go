//go:build integration && transport

// MCP-layer end-to-end regression guards for aibap.mcp#436 (class-include write
// 412) and #383 (DDLS source write 403), verifying the adtler v0.3.8 fixes
// through the real tool handlers against a live SAP system.
//
// These CREATE transports and MUTATE source (they leave artifacts), so they are
// gated behind the extra `transport` build tag and NEVER run in the normal
// integration suite. Run explicitly:
//
//	MCP_INTEGRATION_SYSTEMS="HF S/4 Mandant 100" \
//	  go test -tags 'integration transport' -run TestIntegration_Reproduce ./tools/...
//
// They reuse the shared harness in integration_test.go (sharedServer,
// mustSelectSystem, requireReachable, requireFixture, callTool, textOf).
package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkTransport creates a workbench transport via the create_transport tool and
// returns its number, skipping the subtest if creation is unavailable.
func mkTransport(t *testing.T, desc string) string {
	t.Helper()
	res := callTool(t, sharedServer, "create_transport", map[string]interface{}{
		"category":    "K",
		"description": desc,
		"package":     "Z_ADT_MCP_TEST",
	})
	if res.IsError {
		t.Skipf("create_transport unavailable — cannot run write reproducer: %s", textOf(res))
	}
	var tr struct {
		TransportNumber string `json:"transport_number"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &tr); err != nil || tr.TransportNumber == "" {
		t.Skipf("create_transport returned no transport number: %s", textOf(res))
	}
	return tr.TransportNumber
}

// TestIntegration_Reproduce436_IncludeETag reproduces #436 through the MCP
// tools: lock a class, read an include's (GET-derived) ETag, then write it back
// via set_include_source WITHOUT an explicit lock_handle. Before adtler v0.3.8
// this returned 412 (the GET ETag is not the class-level write-precondition
// ETag). The wrapper now resolves the lock handle (#401) and adtler omits
// If-Match when locked (#436) → the write succeeds.
func TestIntegration_Reproduce436_IncludeETag(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/zcl_adt_mcp_test_units"
	const include = "testclasses"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, classURI)

			tr := mkTransport(t, "aibap #436 include-etag reproducer")

			if r := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": classURI}); r.IsError {
				t.Fatalf("lock_object: %s", textOf(r))
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": classURI})
			})

			getR := callTool(t, sharedServer, "get_include_source", map[string]interface{}{
				"object_uri": classURI, "include": include,
			})
			if getR.IsError {
				t.Fatalf("get_include_source: %s", textOf(getR))
			}
			var got struct {
				Source string `json:"source"`
				ETag   string `json:"etag"`
			}
			if err := json.Unmarshal([]byte(textOf(getR)), &got); err != nil {
				t.Fatalf("parse get_include_source: %v\nraw: %s", err, textOf(getR))
			}
			if got.ETag == "" {
				t.Fatalf("get_include_source returned empty ETag — cannot exercise #436")
			}
			t.Logf("GET-derived include ETag: %s", got.ETag)

			// The call that returned 412 before v0.3.8. No lock_handle on purpose:
			// the wrapper must resolve it from the session lock map (#401).
			setR := callTool(t, sharedServer, "set_include_source", map[string]interface{}{
				"object_uri": classURI,
				"include":    include,
				"source":     got.Source,
				"etag":       got.ETag,
				"transport":  tr,
			})
			if setR.IsError {
				t.Fatalf("REGRESSION #436 — set_include_source failed: %s", textOf(setR))
			}
			t.Logf("#436 OK on %s: include write with GET-derived ETag + resolved lock succeeded (was 412)", sys)
		})
	}
}

// TestIntegration_Reproduce383_DDLS reproduces the DDLS half of #383 through the
// MCP tools: create a CDS view, then write its source via set_source_from_file
// (which auto-locks + resolves the ETag). Before adtler v0.3.8 this returned 403
// "currently editing" because the lock handle was delivered as a header; DDLS
// needs it as the ?lockHandle= query parameter. Skips on systems without DDLS
// (ECC).
func TestIntegration_Reproduce383_DDLS(t *testing.T) {
	const name = "Z_ADT_MCP_CDSREPRO"
	const uri = "/sap/bc/adt/ddic/ddl/sources/z_adt_mcp_cdsrepro"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			tr := mkTransport(t, "aibap #383 DDLS reproducer")

			createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "DDLS",
				"name":        name,
				"package":     "Z_ADT_MCP_TEST",
				"description": "aibap #383 DDLS reproducer",
				"transport":   tr,
			})
			if createR.IsError {
				msg := textOf(createR)
				if strings.Contains(strings.ToLower(msg), "not available") {
					t.Skipf("DDLS creation not available on %s (ECC?): %s", sys, msg)
				}
				// May already exist from a prior run — reuse if so.
				exR := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
				var ex struct {
					Exists bool `json:"exists"`
				}
				_ = json.Unmarshal([]byte(textOf(exR)), &ex)
				if !ex.Exists {
					t.Fatalf("create_object(DDLS): %s", msg)
				}
				t.Logf("DDLS %s already exists, reusing", name)
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": uri, "transport": tr})
			})

			// Valid CDS source (double-quoted + \n per CLAUDE.md — no Go backticks).
			cds := "@EndUserText.label: 'aibap 383 reproducer'\n" +
				"define root view entity Z_ADT_MCP_CDSREPRO\n  as select from t000\n{\n  key mandt as Client\n}\n"
			file := filepath.Join(t.TempDir(), "cds.txt")
			if err := os.WriteFile(file, []byte(cds), 0o644); err != nil {
				t.Fatalf("write temp file: %v", err)
			}

			// The call that returned 403 before v0.3.8. set_source_from_file
			// auto-locks and resolves the ETag through the wrapper.
			setR := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": uri,
				"file_path":  file,
				"transport":  tr,
			})
			if setR.IsError {
				t.Fatalf("REGRESSION #383 — set_source_from_file (DDLS) failed: %s", textOf(setR))
			}
			t.Logf("#383 DDLS OK on %s: DDLS source write succeeded (was 403)", sys)
		})
	}
}
