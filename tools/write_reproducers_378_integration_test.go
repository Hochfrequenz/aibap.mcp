//go:build integration && transport

// MCP-layer end-to-end regression guards for aibap.mcp#378: the four
// write-error hint classes added to tools/errors.go (corrNr-missing,
// EU/510 own-stale-lock, invalid lock handle, CTS save-conflict), verified
// through the real tool handlers against a live SAP system.
//
// These CREATE transports and, in two cases, MUTATE source (they leave
// artifacts on cleanup failure), so they are gated behind the extra
// `transport` build tag like the #436/#383/#442 reproducers in
// write_reproducers_integration_test.go, which they share a package and
// helpers with (mkTransport, ctsReqRe, parseADTError from
// errors_integration_test.go). Run explicitly:
//
//	MCP_INTEGRATION_SYSTEMS="<alias>" \
//	  go test -tags 'integration transport' -run TestIntegration_Reproduce378 ./tools/...
package tools_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestIntegration_Reproduce378_CorrNrMissing reproduces #378 finding 1:
// patch_source without a transport on a non-$TMP object returns 400
// ExceptionParameterNotFound naming corrNr. The write itself fails at SAP, so
// nothing is mutated; patch_source's auto-lock is released on failure via the
// existing releaseOnFailure path, so this needs no transport of its own and
// no cleanup.
func TestIntegration_Reproduce378_CorrNrMissing(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "patch_source", map[string]interface{}{
				"object_uri": uri,
				"operations": []map[string]interface{}{
					{"type": "insert", "after_line": 0, "content": "* aibap #378 corrNr probe — write must fail before this is ever saved"},
				},
			})
			if !res.IsError {
				t.Fatalf("REGRESSION #378 — patch_source without transport unexpectedly SUCCEEDED on non-$TMP object %s; raw: %s", uri, textOf(res))
			}
			out := textOf(res)
			t.Logf("corrNr-missing probe on %s: %s", sys, out)

			if !strings.Contains(out, "ExceptionParameterNotFound") {
				t.Skipf("did not reproduce ExceptionParameterNotFound on %s (system/package config may differ): %s", sys, out)
			}
			if !strings.Contains(out, "create_transport") {
				t.Errorf("REGRESSION #378 — corrNr-missing hint should name create_transport, got: %s", out)
			}
		})
	}
}

// TestIntegration_Reproduce378_EU510OwnStaleLock reproduces #378 finding 2
// (S/4 only): locking the same object twice in one session returns 403
// ExceptionResourceNoAccess ("currently editing") naming the CALLING user,
// not a real concurrent editor. Only locks/unlocks — no source is touched.
func TestIntegration_Reproduce378_EU510OwnStaleLock(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			r1 := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": uri})
			if r1.IsError {
				t.Fatalf("first lock_object failed: %s", textOf(r1))
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
			})

			// Provoke: lock the same object again in the same session.
			r2 := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": uri})
			if !r2.IsError {
				t.Skipf("second lock_object did not conflict on %s — cannot exercise #378 finding 2 here", sys)
			}
			out := textOf(r2)
			t.Logf("EU/510 own-stale-lock probe on %s: %s", sys, out)

			if !strings.Contains(out, "ExceptionResourceNoAccess") {
				t.Skipf("did not reproduce ExceptionResourceNoAccess on %s (#378 marks this S/4-only): %s", sys, out)
			}
			if !strings.Contains(out, "unlock_object") {
				t.Errorf("REGRESSION #378 — EU/510 hint should point at unlock_object, got: %s", out)
			}
			if strings.Contains(out, "S_DEVELOP") {
				t.Errorf("REGRESSION #378 — EU/510 should not get the generic S_DEVELOP authorization hint, got: %s", out)
			}
		})
	}
}

// TestIntegration_Reproduce378_InvalidLockHandle attempts to reproduce #378
// finding 3 (423 ExceptionResourceInvalidLockHandle on a bogus lock_handle).
// Live behavior observed while writing this test is NOT deterministic for a
// plain PROG source write: across repeated runs against the SAME fixture, a
// bogus lock_handle has been observed to (a) succeed outright on both the
// S/4 system and the ECC system, (b) be correctly rejected with 423
// ExceptionResourceInvalidLockHandle on both, and (c) be masked by a 500
// ExceptionResourceSaveFailure transport conflict (#378 finding 4) when the
// fixture happens to already be registered in a different transport than the
// fresh one this run created. This looks state-dependent (e.g. whether the
// object was JUST created vs. reused from an earlier run) rather than a
// clean per-system split — #378's own framing ("S/4 validates, ECC doesn't",
// citing adtler#377) does not fully hold for this endpoint. A likely
// mechanical contributor: adt.(*httpClient).SetSource (adtler source.go)
// sends the lock handle as a header first and, on 423/403/400, RETRIES via
// the ?lockHandle= query parameter — so an observed "success" may be the
// retry's delivery path succeeding where the header path correctly 423'd,
// rather than genuine non-validation. Not fully root-caused here. The test
// tolerates all three outcomes and only asserts the hint content when the
// 423 actually happens — a "success" outcome is reported via t.Skipf, not a
// silent pass, since it reproduces the exact condition #377 warns about. The
// hint itself is also pinned unconditionally by the unit test in
// errors_test.go against the literal reproducer body from the issue.
func TestIntegration_Reproduce378_InvalidLockHandle(t *testing.T) {
	// _V2 suffix: an earlier iteration used Z_ADT_MCP_378_LOCK, then
	// delete_object'd it in cleanup — which deletes the repository object but
	// NOT its CTS transport-directory entry (see the doc comment below), and
	// on the ECC system additionally left a real orphaned ENQUEUE that not even
	// force_unlock could clear (foreign to any live session, see #449). That
	// name is now permanently poisoned on both systems (create_object 403s
	// "currently editing" even though the object doesn't exist). Using a
	// fresh name here rather than fighting SM12.
	const name = "Z_ADT_MCP_378_LOCK_V2"
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_378_lock_v2"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			tr := mkTransport(t, "aibap #378 invalid-lock-handle reproducer")

			createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "PROG", "name": name, "package": "Z_ADT_MCP_TEST",
				"description": "aibap #378 invalid-lock-handle reproducer", "transport": tr,
			})
			if createR.IsError {
				msg := textOf(createR)
				exR := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
				var ex struct {
					Exists bool `json:"exists"`
				}
				if err := json.Unmarshal([]byte(textOf(exR)), &ex); err != nil || !ex.Exists {
					t.Fatalf("create_object(PROG) failed (%s) and object does not already exist", msg)
				}
				t.Logf("%s already exists, reusing", name)
			}
			// Deliberately NOT deleting this object in cleanup: delete_object does
			// not clear the object's CTS transport-directory entry (see
			// transport.go's remove_from_transport doc comment), so a delete
			// here would leave the object name registered in a now-deleted
			// object's stale transport — the NEXT run's create_object would then
			// fail with "already locked in request <stale TR>" (discovered while
			// writing this test). Treat it as a permanent, reused-across-runs
			// fixture instead, like the DDLS reproducers above.

			// Provoke: patch_source with an explicit, bogus lock_handle. This
			// bypasses our own auto-lock entirely (explicit handle wins), so
			// SAP is the only thing that can reject it.
			res := callTool(t, sharedServer, "patch_source", map[string]interface{}{
				"object_uri": uri,
				"operations": []map[string]interface{}{
					{"type": "insert", "after_line": 0, "content": "* aibap #378 invalid-lock-handle probe"},
				},
				"transport":   tr,
				"lock_handle": "DEADBEEF",
			})
			out := textOf(res)
			if !res.IsError {
				// The write went through despite the bogus handle, so SAP may
				// have registered a real ENQUEUE under it that our lock map
				// never tracked (we bypassed it with the explicit handle) — an
				// untracked unlock_object would no-op ("no lock tracked") and
				// leave the entry orphaned for any FUTURE process run, exactly
				// like the _V1 name this test used to use before it got
				// poisoned that way. Acquire a real lock and release it
				// properly instead of relying on t.Cleanup here.
				if relock := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": uri}); relock.IsError {
					t.Logf("WARNING: could not re-lock %s after bogus-handle write succeeded on %s — may leave a stale lock across process runs: %s", uri, sys, textOf(relock))
				} else {
					_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				}
				// A silent t.Logf + return would report this subtest as a plain
				// PASS even though a bogus lock_handle just got accepted — the
				// very condition #377 warns about. Skip loudly instead so this
				// outcome is visible in test output, not swallowed as green.
				t.Skipf("patch_source with bogus lock_handle SUCCEEDED on %s this run (see doc comment: likely adtler's header->query-param retry on 423/403/400 taking a delivery path this endpoint doesn't validate, not settled per-system behavior) — cannot exercise the 423 branch this run", sys)
			}
			// This branch never acquired a real lock (explicit handle bypassed
			// lockMap entirely) and SAP rejected the write outright, so there is
			// nothing to unlock.
			status, excType := parseADTError(out)
			t.Logf("invalid-lock-handle probe on %s: status=%d type=%q raw=%q", sys, status, excType, out)

			switch excType {
			case "ExceptionResourceInvalidLockHandle":
				if !strings.Contains(out, "lock_object") {
					t.Errorf("REGRESSION #378 — invalid-lock-handle hint should point at lock_object, got: %s", out)
				}
				if strings.Contains(out, "Call `unlock_object`") {
					t.Errorf("REGRESSION #378 — invalid-lock-handle hint should NOT tell caller to unlock_object (there is no real lock to drop), got: %s", out)
				}
			case "ExceptionResourceSaveFailure":
				// Observed live: whether a mismatched-transport conflict or an
				// invalid-lock-handle rejection wins appears to depend on prior
				// state of this fixture (e.g. which transport it's currently
				// registered in from an earlier run) rather than being
				// deterministic per call. That's #378 finding 4, already
				// covered by TestIntegration_Reproduce378_TransportSaveConflict
				// — not a failure here, just a miss for THIS finding this run.
				t.Skipf("got the save-conflict condition (#378 finding 4) instead of invalid-lock-handle on %s this run: %s", sys, out)
			default:
				t.Skipf("did not reproduce ExceptionResourceInvalidLockHandle on %s (got type=%q): %s", sys, excType, out)
			}
		})
	}
}

// TestIntegration_Reproduce378_TransportSaveConflict reproduces #378 finding
// 4: writing a PROG that is already registered in transport A, targeting a
// DIFFERENT transport B, can surface as a bare 500 ExceptionResourceSaveFailure
// (CTS_WBO_API/020) rather than the 409 ExceptionResourceLockConflict that
// TestIntegration_Reproduce442_LockedInTransport already covers for DDLS. If
// this object type/system combination produces the 409 instead, that path is
// already regression-tested there, so this skips rather than duplicating it.
func TestIntegration_Reproduce378_TransportSaveConflict(t *testing.T) {
	const name = "Z_ADT_MCP_378_SAVE_V2" // see the _V2 note on TestIntegration_Reproduce378_InvalidLockHandle
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_378_save_v2"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			// trA registers the object (via create_object, which alone puts a
			// R3TR PROG entry in trA's CTS directory — no separate write needed).
			// On a re-run the object already exists, already registered in
			// whichever transport an earlier run left open; either way the
			// probe below provokes the same conflict against whatever request
			// currently holds the CTS entry.
			trA := mkTransport(t, "aibap #378 save-conflict reproducer (registration transport)")
			trB := mkTransport(t, "aibap #378 save-conflict reproducer (foreign transport)")

			createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "PROG", "name": name, "package": "Z_ADT_MCP_TEST",
				"description": "aibap #378 save-conflict reproducer", "transport": trA,
			})
			if createR.IsError {
				msg := textOf(createR)
				exR := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
				var ex struct {
					Exists bool `json:"exists"`
				}
				if err := json.Unmarshal([]byte(textOf(exR)), &ex); err != nil || !ex.Exists {
					t.Fatalf("create_object(PROG) failed (%s) and object does not already exist", msg)
				}
				t.Logf("%s already exists (registered in an earlier transport), reusing", name)
			}
			// Deliberately NOT deleting this object — see the doc comment on
			// TestIntegration_Reproduce378_InvalidLockHandle for why delete_object
			// would poison the NEXT run instead of cleaning up this one.
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
			})

			// Provoke: write targeting a DIFFERENT transport than whichever one
			// currently holds the object's CTS registration.
			second := callTool(t, sharedServer, "patch_source", map[string]interface{}{
				"object_uri": uri,
				"operations": []map[string]interface{}{
					{"type": "insert", "after_line": 0, "content": "* aibap #378 save-conflict reproducer (second write)"},
				},
				"transport": trB,
			})
			if !second.IsError {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				t.Skipf("write to foreign transport %s did not conflict on %s — cannot exercise #378 finding 4", trB, sys)
			}
			out := textOf(second)
			status, excType := parseADTError(out)
			t.Logf("save-conflict probe on %s: status=%d type=%q raw=%q", sys, status, excType, out)

			if status != 500 {
				t.Skipf("save-conflict on PROG %s returned status %d (type %q) on %s, not the 500 CTS_WBO_API/020 variant #378 finding 4 describes — see TestIntegration_Reproduce442_LockedInTransport for the 409 sibling", name, status, excType, sys)
			}

			ids := ctsReqRe.FindAllString(out, -1)
			if len(ids) == 0 {
				t.Fatalf("save-conflict message named no transport request — cannot verify #378 finding 4 hint: %s", out)
			}
			blocking := ids[len(ids)-1]

			if !strings.Contains(out, "transport="+blocking) {
				t.Errorf("REGRESSION #378 — save-conflict hint should tell caller to retry with transport=%s, got: %s", blocking, out)
			}
			if strings.Contains(out, "SM21") {
				t.Errorf("REGRESSION #378 — save-conflict should not get the generic SM21/ST22 server-error hint, got: %s", out)
			}

			// Documented recovery: writing to the named request succeeds.
			recover := callTool(t, sharedServer, "patch_source", map[string]interface{}{
				"object_uri": uri,
				"operations": []map[string]interface{}{
					{"type": "insert", "after_line": 0, "content": "* aibap #378 save-conflict reproducer (recovery write)"},
				},
				"transport": blocking,
			})
			if recover.IsError {
				t.Fatalf("recovery write to the named request %s failed: %s", blocking, textOf(recover))
			}
			_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
			t.Logf("#378 finding 4 OK on %s: save-conflict named %s, hint steered to transport=%s, recovery write succeeded", sys, blocking, blocking)
		})
	}
}
