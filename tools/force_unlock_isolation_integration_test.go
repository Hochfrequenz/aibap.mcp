//go:build integration

package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/config"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// TestIntegration_ForceUnlock_SessionScoped verifies the property Jonatan
// asked about on PR #431: force_unlock must release ONLY the locks held by the
// calling MCP session, never another session's (and therefore never another
// user's) ENQUEUEs.
//
// force_unlock terminates the process's stateful SAP session via ICF logoff,
// and SAP ties every ENQUEUE to the session that acquired it. So the property
// to prove is: a lock held by a *different, independent* SAP session survives
// force_unlock. We use a second session of the SAME user (a second adt.Client
// over the same config entry has its own cookie jar → its own stateful SAP
// session → its own enqueue owner), which is a stronger guarantee than a second
// user and needs nothing extra in systems.json: if the same user's other
// session survives, a different user's certainly does.
//
// Self-guarding: before the reset we confirm that session 2's lock genuinely
// blocks session 1. If a given system turns out not to be session-exclusive
// (session 1 can lock the object despite session 2 holding it), the test skips
// with a clear reason instead of asserting something meaningless.
func TestIntegration_ForceUnlock_SessionScoped(t *testing.T) {
	const uriA = "/sap/bc/adt/oo/classes/zcl_adt_mcp_test_units"       // session 1's own lock
	const uriB = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report" // foreign lock (session 2)

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)

			// Session 1 = the shared server used by every other integration test.
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uriA)
			requireFixture(t, sharedServer, sys, uriB)

			// Session 2 = a second, fully independent MCP server / SAP session
			// (own client, own cookie jar, own lock map) over the same config.
			server2 := buildSecondServer(t)
			mustSelectSystem(t, server2, sys)

			// 1. Session 2 acquires a real ENQUEUE on object B.
			if res := callTool(t, server2, "lock_object", map[string]interface{}{
				"object_uri": uriB,
			}); res.IsError {
				t.Skipf("session 2 could not lock %s (%s) — cannot set up the foreign-lock scenario on %s", uriB, textOf(res), sys)
			}
			// Ensure B is released even if a later assertion fails.
			defer func() {
				if res := callTool(t, server2, "unlock_object", map[string]interface{}{
					"object_uri": uriB,
				}); res.IsError {
					t.Logf("cleanup: session 2 unlock of %s returned error (non-fatal): %s", uriB, textOf(res))
				}
			}()

			// 2. Baseline: session 1 must NOT be able to lock B while session 2
			//    holds it. If it can, this system isn't session-exclusive for
			//    this object and the isolation assertion below can't discriminate
			//    — skip rather than false-green.
			if res := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uriB,
			}); !res.IsError {
				// Session 1 unexpectedly got the lock; release it and skip.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uriB})
				t.Skipf("session 1 acquired %s despite session 2 holding it — enqueue not session-exclusive on %s; isolation test not applicable", uriB, sys)
			}

			// 3. Session 1 acquires its own lock on object A.
			if res := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uriA,
			}); res.IsError {
				t.Fatalf("session 1 lock_object(%s) failed: %s", uriA, textOf(res))
			}
			// Release A no matter what follows. Registered immediately after the
			// lock so a force_unlock failure below can't leave A stuck on the
			// shared session for the rest of the suite. force_unlock (step 4) or
			// the re-lock (step 5) may already have released/re-taken A, so a
			// no-op unlock here is fine and only logged.
			defer func() {
				if res := callTool(t, sharedServer, "unlock_object", map[string]interface{}{
					"object_uri": uriA,
				}); res.IsError {
					t.Logf("cleanup: session 1 unlock of %s returned error (non-fatal): %s", uriA, textOf(res))
				}
			}()

			// 4. Session 1 force_unlock — terminates session 1 only.
			fuRes := callTool(t, sharedServer, "force_unlock", map[string]interface{}{})
			if fuRes.IsError {
				t.Fatalf("force_unlock returned error: %s", textOf(fuRes))
			}
			var fu struct {
				SessionReset bool `json:"session_reset"`
			}
			if err := json.Unmarshal([]byte(textOf(fuRes)), &fu); err != nil {
				t.Fatalf("parse force_unlock result %q: %v", textOf(fuRes), err)
			}
			if !fu.SessionReset {
				t.Fatalf("session_reset = false, want true")
			}

			// 5. Session 1's OWN lock is released and re-auth works: re-locking
			//    A must succeed (existing coverage, re-asserted here).
			if res := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uriA,
			}); res.IsError {
				t.Fatalf("re-lock of own object %s after force_unlock failed (lock not released or re-auth broke): %s", uriA, textOf(res))
			}

			// 6. THE ISOLATION ASSERTION: object B is still held by session 2.
			//    If force_unlock had released locks system-wide, B would now be
			//    free and this lock attempt would succeed.
			//    Assumes session 2's stateful session hasn't idle-timed-out
			//    between steps 1 and 6 (SAP stateful sessions expire after a few
			//    minutes of inactivity); the test runs in well under a second, so
			//    that window is not a practical concern.
			if res := callTool(t, sharedServer, "lock_object", map[string]interface{}{
				"object_uri": uriB,
			}); !res.IsError {
				// It leaked: session 1 grabbed B. Release it so cleanup is sane.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uriB})
				t.Fatalf("force_unlock released a foreign session's lock on %s — it is NOT session-scoped", uriB)
			}
		})
	}
}

// buildSecondServer stands up a second MCP server backed by an independent set
// of adt.Clients (own cookie jars → own stateful SAP sessions) over the same
// systems.json used by the shared server. Used to simulate a concurrent,
// unrelated SAP session for isolation testing.
func buildSecondServer(t *testing.T) *server.MCPServer {
	t.Helper()

	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		t.Fatalf("buildSecondServer: config.Load failed: %v", err)
	}
	clients, err := adt.NewClientsFromConfig(&cfg.Config, "aibap.mcp")
	if err != nil {
		t.Fatalf("buildSecondServer: NewClientsFromConfig failed: %v", err)
	}

	// Restrict to the systems this run actually targets, mirroring TestMain.
	registryClients := map[string]adt.Client{}
	var defaultSys string
	for _, sys := range integrationSystems {
		if c, ok := clients[sys]; ok {
			registryClients[sys] = c
			if defaultSys == "" || reachable[sys] {
				defaultSys = sys
			}
		}
	}
	if len(registryClients) == 0 {
		t.Fatalf("buildSecondServer: none of %v present in config", integrationSystems)
	}

	reg, err := adt.NewClientRegistry(registryClients, defaultSys)
	if err != nil {
		t.Fatalf("buildSecondServer: NewClientRegistry failed: %v", err)
	}

	s := server.NewMCPServer("aibap.mcp-integration-session2", "0.0.0")
	tools.RegisterAllWithLockMap(
		s,
		reg,
		reg, // ClientRegistry implements SystemSelector
		adt.NewLockMap(),
		tools.ParseToolGroups([]string{"all"}),
		nil, nil,
	)
	return s
}
