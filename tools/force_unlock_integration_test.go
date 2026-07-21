//go:build integration

package tools_test

import (
	"encoding/json"
	"testing"
)

// TestIntegration_ForceUnlock verifies the #383 recovery path end-to-end
// against a live SAP system: a real ENQUEUE is acquired via lock_object, then
// force_unlock terminates the stateful session (releasing the lock server-side)
// and clears the active system's cached handle. Re-locking afterward proves the
// connection re-authenticates cleanly after the logoff.
//
// Low-risk: only lock/unlock on the fixture class, no source mutation.
func TestIntegration_ForceUnlock(t *testing.T) {
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_test_units"
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			// Acquire a real ENQUEUE and cache its handle in the lock map.
			lockRes := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uri,
			})
			if lockRes.IsError {
				t.Fatalf("lock_object returned error: %s", textOf(lockRes))
			}

			// Force-unlock: terminate the session and clear cached handles.
			fuRes := callTool(t, sharedServer, "force_unlock", map[string]interface{}{})
			if fuRes.IsError {
				t.Fatalf("force_unlock returned error: %s", textOf(fuRes))
			}
			var fu struct {
				System       string `json:"system"`
				SessionReset bool   `json:"session_reset"`
				LocksCleared int    `json:"locks_cleared"`
			}
			if err := json.Unmarshal([]byte(textOf(fuRes)), &fu); err != nil {
				t.Fatalf("parse force_unlock result %q: %v", textOf(fuRes), err)
			}
			if !fu.SessionReset {
				t.Errorf("session_reset = false, want true")
			}
			if fu.System != sys {
				t.Errorf("system = %q, want %q", fu.System, sys)
			}
			if fu.LocksCleared < 1 {
				t.Errorf("locks_cleared = %d, want >= 1 (the lock_object handle should have been cleared)", fu.LocksCleared)
			}

			// The session was dropped; the next call must re-authenticate
			// transparently. Re-locking the (now released) object must succeed —
			// if the ENQUEUE were still held this would fail 403/423.
			relockRes := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uri,
			})
			if relockRes.IsError {
				t.Fatalf("re-lock after force_unlock returned error (lock not released or re-auth failed): %s", textOf(relockRes))
			}

			// Clean up: release the fresh lock we just took.
			if ures := callTool(t, sharedServer, "unlock_object", map[string]interface{}{
				"object_uri": uri,
			}); ures.IsError {
				t.Logf("cleanup unlock_object returned error (non-fatal): %s", textOf(ures))
			}
		})
	}
}
