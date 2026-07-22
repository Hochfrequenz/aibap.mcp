//go:build integration

package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_LockRollbackOnWriteFailure is a live smoke test of the #383
// rollback path: it triggers a real auto-locked write failure and confirms the
// rollback runs against SAP without error and the object is subsequently usable.
//
// It triggers a real failure without mutating the fixture: writing a
// transportable object's own current source back WITHOUT a transport fails with
// 400 "corrNr could not be found" AFTER the auto-lock — exactly the leak path.
// On systems where the object is local ($TMP) the write succeeds (a harmless
// same-source no-op) and the subtest skips, since there is no failure to roll back.
//
// NOTE on strength: the follow-up lock_object runs in the SAME MCP session, so
// if the rollback did NOT release, ADT may still return a (re-entrant) handle
// rather than 403 — this test can't distinguish that. The authoritative release
// guarantee is the unit test asserting UnlockObject is called with the exact
// auto-acquired handle, plus adtler#58 (a same-session dequeue with the real
// handle releases the enqueue). A cross-session assertion would need a second
// client, which this single-session harness doesn't provide.
func TestIntegration_LockRollbackOnWriteFailure(t *testing.T) {
	const uri = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			// Capture current source so the write is content-neutral.
			getR := callTool(t, sharedServer, "get_source", map[string]interface{}{"object_uri": uri})
			if getR.IsError {
				t.Fatalf("get_source: %s", textOf(getR))
			}
			var src struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal([]byte(textOf(getR)), &src); err != nil || src.Source == "" {
				t.Fatalf("parse get_source: %v", err)
			}
			file := filepath.Join(t.TempDir(), "src.abap")
			if err := os.WriteFile(file, []byte(src.Source), 0o644); err != nil {
				t.Fatalf("write temp: %v", err)
			}

			// Auto-locked write with NO transport. On a transportable object this
			// fails at the corrNr check AFTER auto-locking (the leak path).
			writeR := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": uri,
				"file_path":  file,
			})
			if !writeR.IsError {
				// Local ($TMP) object: write succeeded (harmless same-source no-op).
				// Best-effort unlock in case it auto-locked, then skip.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				t.Skipf("write did not fail on %s (object is local?) — cannot exercise rollback here", sys)
			}
			t.Logf("auto-locked write failed as expected: %s", textOf(writeR))

			// The proof: if the rollback released the enqueue, the object is
			// re-lockable. If the lock leaked, lock_object returns 403
			// "currently editing".
			lockR := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": uri})
			if lockR.IsError {
				t.Fatalf("object not usable after failed write (rollback path regressed): %s", textOf(lockR))
			}
			// Clean up the lock we just took to verify.
			_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
			t.Logf("#383 rollback smoke OK on %s: failed write rolled back cleanly, object re-lockable (same session)", sys)
		})
	}
}
