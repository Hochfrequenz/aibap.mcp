//go:build integration

// Throwaway reproducer harness for the adtler v0.5.1 -> v0.5.2 bump.
// Delete after the bump PR merges.
//
// Run:
//
//	go test -tags integration -v -count=1 -run BumpVerify_520 ./tools/...
//
// It runs the reproducer of the one tracking-issue (#508) entry that adtler
// v0.5.2 resolves:
//
//   - #520 — create_package always failed on S/4 with 400
//     ExceptionResourceBadRequest ("Accept header missing"), because adtler's
//     CreatePackage sent a Content-Type and no Accept (adtler#149/#151).
//
// This test WRITES to the target system: it creates a package, and that
// package CANNOT be removed again — delete_object on a package fails with 412
// (adtler#150). The name is therefore deliberate and permanent, and is recorded
// in the pull request so the next developer or agent can reuse it rather than
// create another.
//
// Re-running is safe but proves less: once the package exists, SAP refuses the
// POST as a duplicate. That refusal is NOT evidence the fix works — SAP checks
// the Accept header last, so a duplicate answers ExceptionResourceAlreadyExists
// with or without the header (measured on S/4 on 2026-09-21, see adtler#151).
// Only the first run on a system observes the fix; afterwards this test
// degrades to checking that the package is still readable.
package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/config"
)

// bump552Package is created permanently on every system this runs against.
const bump552Package = "$ZHFABAP_PKG520"

// bump552URI is the ADT URI of that package: lower-cased, with the leading
// "$" percent-encoded.
const bump552URI = "/sap/bc/adt/packages/%24zhfabap_pkg520"

// responsibleFor returns the logon user configured for a system, which SAP
// requires in adtcore:responsible — an empty value comes back as an opaque
// 400 ExceptionInvalidData ("Check of condition failed").
func responsibleFor(t *testing.T, sys string) string {
	t.Helper()
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		t.Skipf("cannot read config to resolve the logon user: %v", err)
	}
	entry, ok := cfg.Systems[sys]
	if !ok || strings.TrimSpace(entry.User) == "" {
		t.Skipf("system %q has no logon user configured", sys)
	}
	return entry.User
}

func TestBumpVerify_520_CreatePackageReachesSAP(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "create_package", map[string]interface{}{
				"name":        bump552Package,
				"description": "aibap.mcp #520 bump reproducer",
				"responsible": responsibleFor(t, sys),
			})
			out := textOf(res)

			// The exact failure this bump exists to remove. It must not come
			// back, on any system, whatever else happens.
			if strings.Contains(out, "ExceptionResourceBadRequest") {
				t.Fatalf("[%s] create_package still rejected at the HTTP layer: %s — "+
					"this is the adtler#149 failure, so the branch is not consuming "+
					"the release that fixes it", sys, out)
			}

			switch {
			case res.IsError && strings.Contains(out, "ExceptionResourceAlreadyExists"):
				// Expected on re-runs: the package survives from an earlier
				// run because it cannot be deleted (adtler#150).
				t.Logf("[%s] %s already exists — the POST reached SAP, but this run "+
					"cannot observe the fix itself (Accept is checked last)",
					sys, bump552Package)
			case res.IsError:
				t.Fatalf("[%s] create_package failed: %s", sys, out)
			default:
				var created struct {
					Name    string `json:"name"`
					Created bool   `json:"created"`
				}
				if err := json.Unmarshal([]byte(out), &created); err != nil {
					t.Fatalf("[%s] create_package returned unparsable output %q: %v", sys, out, err)
				}
				if !created.Created {
					t.Fatalf("[%s] create_package reported created=false: %s", sys, out)
				}
				t.Logf("[%s] created %s — this run observed the fix", sys, created.Name)
			}

			// Either way the package must now read back as a package.
			info := callTool(t, sharedServer, "get_object_info", map[string]interface{}{
				"object_uri": bump552URI,
			})
			infoOut := textOf(info)
			if info.IsError {
				t.Fatalf("[%s] get_object_info on %s failed: %s", sys, bump552Package, infoOut)
			}
			if !strings.Contains(infoOut, "DEVC/K") {
				t.Fatalf("[%s] %s does not read back as DEVC/K: %s", sys, bump552Package, infoOut)
			}
			t.Logf("[%s] %s reads back as DEVC/K", sys, bump552Package)
		})
	}
}
