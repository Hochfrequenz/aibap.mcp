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
	"regexp"
	"strings"
	"testing"
)

// ctsReqRe mirrors adtler's request-ID matcher; the test uses it to read the
// blocking request the 409 names so it can drive the documented recovery.
var ctsReqRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]{2}K[0-9]{6}\b`)

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
				if err := json.Unmarshal([]byte(textOf(exR)), &ex); err != nil {
					t.Fatalf("create_object(DDLS) failed (%s) and object_exists returned unparseable output: %v\nraw: %s", msg, err, textOf(exR))
				}
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

// TestIntegration_Reproduce442_LockedInTransport reproduces #442 through the MCP
// tools: create a DDLS registered in transport A, then write it targeting a
// DIFFERENT transport B. SAP rejects with 409 ExceptionResourceLockConflict
// ("Object ... is already locked in request A") — a CTS object-directory
// registration, a distinct lock domain from the runtime ENQUEUE. adtler v0.3.10
// classifies this as ErrorObjectLockedInTransport and parses the blocking
// request; the wrapper turns it into a hint that names A and tells the caller to
// retry with transport=A. This guard asserts the hint (regression for the whole
// classify→hint path) and that the documented recovery (writing to the named
// request) actually succeeds.
//
// It CREATES a transportable object and MUTATES source, so it is gated behind
// the `transport` tag. Cleanup unlocks in-session before deleting to avoid
// leaving an orphaned "currently editing" lock (a failed cleanup only logs, per
// the reproducers above). Skips on systems without DDLS (ECC).
func TestIntegration_Reproduce442_LockedInTransport(t *testing.T) {
	const name = "Z_ADT_MCP_CTS442"
	const uri = "/sap/bc/adt/ddic/ddl/sources/z_adt_mcp_cts442"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			trA := mkTransport(t, "aibap #442 reproducer (registration transport)")
			trB := mkTransport(t, "aibap #442 reproducer (foreign transport)")

			createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "DDLS", "name": name, "package": "Z_ADT_MCP_TEST",
				"description": "aibap #442 reproducer", "transport": trA,
			})
			if createR.IsError {
				msg := textOf(createR)
				if strings.Contains(strings.ToLower(msg), "not available") {
					t.Skipf("DDLS creation not available on %s (ECC?): %s", sys, msg)
				}
				exR := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
				var ex struct {
					Exists bool `json:"exists"`
				}
				if err := json.Unmarshal([]byte(textOf(exR)), &ex); err != nil {
					t.Fatalf("create_object(DDLS) failed (%s) and object_exists returned unparseable output: %v\nraw: %s", msg, err, textOf(exR))
				}
				if !ex.Exists {
					t.Fatalf("create_object(DDLS): %s", msg)
				}
				t.Logf("DDLS %s already exists, reusing", name)
			}
			t.Cleanup(func() {
				// Unlock in-session FIRST — set_source_from_file auto-locks and a
				// left-behind lock orphans on process exit (unreachable by a later
				// session; see #449). Then delete against A.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				if d := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": uri, "transport": trA}); d.IsError {
					t.Logf("WARNING: cleanup could not delete %s (may need SM12): %s", name, textOf(d))
				}
			})

			cds := "@EndUserText.label: 'aibap 442 reproducer'\n" +
				"define root view entity Z_ADT_MCP_CTS442\n  as select from t000\n{\n  key mandt as Client\n}\n"
			file := filepath.Join(t.TempDir(), "cds.txt")
			if err := os.WriteFile(file, []byte(cds), 0o644); err != nil {
				t.Fatalf("write temp: %v", err)
			}

			// Provoke: write targeting B while the object is registered in A.
			setB := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": uri, "file_path": file, "transport": trB,
			})
			if !setB.IsError {
				// Object was already active / registered in B from a prior run —
				// the conflict cannot be exercised. Unlock and skip.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				t.Skipf("write to foreign transport %s did not conflict on %s — cannot exercise #442 (object already registered there or activated)", trB, sys)
			}
			out := textOf(setB)
			t.Logf("write-to-foreign-transport rejected as expected: %s", out)

			// The blocking request the 409 names (last CTS ID in the message).
			ids := ctsReqRe.FindAllString(out, -1)
			if len(ids) == 0 {
				t.Fatalf("409 message named no transport request — cannot verify #442 hint: %s", out)
			}
			blocking := ids[len(ids)-1]

			// The hint must name the blocking request and steer to retrying with
			// it — not to the useless unlock_object/SM12 path.
			if !strings.Contains(out, "transport="+blocking) {
				t.Errorf("REGRESSION #442 — hint should tell caller to retry with transport=%s, got: %s", blocking, out)
			}
			if strings.Contains(out, "Save conflict") {
				t.Errorf("REGRESSION #442 — got the generic lock-conflict hint instead of the transport-named one: %s", out)
			}

			// Documented recovery: writing to the named request succeeds.
			setRecover := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": uri, "file_path": file, "transport": blocking,
			})
			if setRecover.IsError {
				t.Fatalf("recovery write to the named request %s failed: %s", blocking, textOf(setRecover))
			}
			// Release the auto-lock in-session so cleanup can delete cleanly.
			_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
			t.Logf("#442 OK on %s: 409 named %s, hint steered to transport=%s, recovery write to %s succeeded", sys, blocking, blocking, blocking)
		})
	}
}
