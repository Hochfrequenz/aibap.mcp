//go:build integration

// Throwaway reproducer harness for the adtler v0.5.0 -> v0.5.1 bump.
// Delete after the bump PR merges.
//
// Run:
//
//	go test -tags integration -v -count=1 -run BumpVerify ./tools/...
//
// It collects the reproducers of the two tracking-issue (#508) entries that
// adtler v0.5.1 is expected to resolve:
//
//   - #500 — activate_object reported {"Success": true} while the object
//     stayed inactive (adtler#144/#145: ActivateObjects now re-reads
//     GetInactiveObjects after an apparent success and overrides the result)
//   - #409 — get_object_dependencies rejected UIAC/UIAD (adtler#139/#140)
//
// The #500 test WRITES to the target system: it creates a transport and a
// throwaway class in Z_ADT_MCP_TEST, writes deliberately broken source, and
// deletes the class again in cleanup. The transport stays behind (nothing
// here releases it).
//
// Object names used in this file are either throwaway Z* names created here
// or SAP-delivered ones; nothing internal is hardcoded — the UIAC/UIAD
// candidates are discovered from TADIR at runtime.
package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bumpReqRe matches a transport request number, so a blocked create can be
// retried under the request that still owns the object entry.
var bumpReqRe = regexp.MustCompile("[A-Z][A-Z0-9]{2}K[0-9]{6}")

// mkBumpTransport creates a workbench transport for Z_ADT_MCP_TEST and
// returns its number, skipping the subtest if creation is unavailable. It
// duplicates mkTransport from write_reproducers_integration_test.go, which
// carries the extra `transport` build tag and so is not compiled here.
func mkBumpTransport(t *testing.T, desc string) string {
	t.Helper()
	res := callTool(t, sharedServer, "create_transport", map[string]interface{}{
		"category":    "K",
		"description": desc,
		"package":     "Z_ADT_MCP_TEST",
	})
	if res.IsError {
		t.Skipf("create_transport unavailable — cannot run bump reproducer: %s", textOf(res))
	}
	var tr struct {
		TransportNumber string `json:"transport_number"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &tr); err != nil || tr.TransportNumber == "" {
		t.Skipf("create_transport returned no transport number: %s", textOf(res))
	}
	return tr.TransportNumber
}

// listedInactive reports whether get_inactive_objects currently lists an
// object whose URI mentions the given object name. Matching is on the name
// rather than the full URI because the inactive list points at individual
// include URIs (CLAS/OC, CLAS/OM/…) rather than the bare class URI.
func listedInactive(t *testing.T, objectName string) (bool, string) {
	t.Helper()
	res := callTool(t, sharedServer, "get_inactive_objects", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("get_inactive_objects: %s", textOf(res))
	}
	raw := textOf(res)
	var payload struct {
		Count   int `json:"count"`
		Objects []struct {
			URI  string
			Name string
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse get_inactive_objects: %v\nraw: %s", err, raw)
	}
	needle := strings.ToLower(objectName)
	for _, o := range payload.Objects {
		if strings.Contains(strings.ToLower(o.URI), needle) || strings.EqualFold(o.Name, objectName) {
			return true, o.URI
		}
	}
	return false, ""
}

// activationResult is the wire shape of adt.ActivationResult (no JSON tags
// upstream, so the field names are the JSON keys).
type activationResult struct {
	Success  bool
	Messages []struct {
		ObjectURI string
		Type      string
		Text      string
	}
}

// logMessages prints every activation message so the PR can quote what the
// caller is actually told, rather than only the boolean.
func logMessages(t *testing.T, phase string, r activationResult) {
	t.Helper()
	for i, m := range r.Messages {
		t.Logf("%s message[%d]: type=%s uri=%s text=%q", phase, i, m.Type, m.ObjectURI, m.Text)
	}
}

func activate(t *testing.T, objectURI string) activationResult {
	t.Helper()
	res := callTool(t, sharedServer, "activate_object", map[string]interface{}{
		"object_uri": objectURI,
	})
	if res.IsError {
		t.Fatalf("activate_object returned IsError: %s", textOf(res))
	}
	var out activationResult
	if err := json.Unmarshal([]byte(textOf(res)), &out); err != nil {
		t.Fatalf("parse activate_object: %v\nraw: %s", err, textOf(res))
	}
	return out
}

func writeSource(t *testing.T, objectURI, transport, source string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "src.abap")
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	res := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
		"object_uri": objectURI,
		"file_path":  file,
		"transport":  transport,
	})
	if res.IsError {
		t.Fatalf("set_source_from_file: %s", textOf(res))
	}
}

// TestBumpVerify_500_ActivationTellsTheTruth is the #500 reproducer.
//
// Phase 1 drives an activation that SAP must refuse (a class whose
// implementation calls something that does not exist). Before adtler v0.5.1
// this returned {"Success": true, "Messages": null} on any 2xx — on ECC the
// activation response body is empty, so there was nothing to parse an error
// out of. v0.5.1 re-reads get_inactive_objects and overrides Success. The
// assertion is the invariant, not the ECC-specific symptom: an activation
// result may not claim success while the object is still listed as inactive.
//
// Phase 2 repairs the source and activates again — the no-regression half:
// a genuine activation must still report Success:true and must leave the
// object off the inactive list.
func TestBumpVerify_500_ActivationTellsTheTruth(t *testing.T) {
	const name = "ZCL_ADT_MCP_BUMP551"
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_bump551"

	broken := "CLASS " + name + " DEFINITION\n" +
		"  PUBLIC FINAL CREATE PUBLIC .\n" +
		"  PUBLIC SECTION.\n" +
		"    METHODS ping RETURNING VALUE(rv_text) TYPE string.\n" +
		"ENDCLASS.\n\n" +
		"CLASS " + name + " IMPLEMENTATION.\n" +
		"  METHOD ping.\n" +
		"    rv_text = zcl_this_class_does_not_exist=>nope( ).\n" +
		"  ENDMETHOD.\n" +
		"ENDCLASS.\n"

	repaired := "CLASS " + name + " DEFINITION\n" +
		"  PUBLIC FINAL CREATE PUBLIC .\n" +
		"  PUBLIC SECTION.\n" +
		"    METHODS ping RETURNING VALUE(rv_text) TYPE string.\n" +
		"ENDCLASS.\n\n" +
		"CLASS " + name + " IMPLEMENTATION.\n" +
		"  METHOD ping.\n" +
		"    rv_text = 'pong'.\n" +
		"  ENDMETHOD.\n" +
		"ENDCLASS.\n"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			tr := mkBumpTransport(t, "aibap #500 activation verification")

			createR := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "CLAS",
				"name":        name,
				"package":     "Z_ADT_MCP_TEST",
				"description": "aibap #500 bump reproducer",
				"transport":   tr,
			})
			if createR.IsError {
				// A previous run of this test deleted the class but left its
				// entry in that run's (unreleased) transport, so SAP refuses
				// to re-create it under a different request. Retry with the
				// request the error names.
				blocking := bumpReqRe.FindString(textOf(createR))
				if blocking == "" {
					t.Fatalf("create_object(CLAS %s): %s", name, textOf(createR))
				}
				t.Logf("create_object refused; retrying under the request that still owns the object entry")
				tr = blocking
				createR = callTool(t, sharedServer, "create_object", map[string]interface{}{
					"object_type": "CLAS",
					"name":        name,
					"package":     "Z_ADT_MCP_TEST",
					"description": "aibap #500 bump reproducer",
					"transport":   tr,
				})
				if createR.IsError {
					t.Fatalf("create_object(CLAS %s) retry: %s", name, textOf(createR))
				}
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": uri})
				_ = callTool(t, sharedServer, "delete_object", map[string]interface{}{
					"object_uri": uri, "transport": tr,
				})
			})

			// Phase 1: activation that must not succeed.
			writeSource(t, uri, tr, broken)
			got := activate(t, uri)
			logMessages(t, "phase 1", got)
			inactive, inactiveURI := listedInactive(t, name)
			t.Logf("phase 1 (broken source) on %s: Success=%v messages=%d stillInactive=%v (%s)",
				sys, got.Success, len(got.Messages), inactive, inactiveURI)

			if inactive && got.Success {
				t.Fatalf("#500 NOT fixed on %s: activate_object reported Success:true while %s is still listed inactive (%s)",
					sys, name, inactiveURI)
			}
			if !inactive {
				t.Logf("NOTE on %s: SAP activated the broken class anyway (or dropped it from the inactive list) — "+
					"phase 1 could not exercise the silent-no-op path here; the invariant still holds", sys)
			}
			if !got.Success && len(got.Messages) == 0 {
				t.Errorf("activate_object reported failure without a single message — the caller is told nothing")
			}

			// Phase 2: no-regression — a genuine activation still reports success.
			writeSource(t, uri, tr, repaired)
			ok := activate(t, uri)
			logMessages(t, "phase 2", ok)
			stillInactive, stillURI := listedInactive(t, name)
			t.Logf("phase 2 (repaired source) on %s: Success=%v messages=%d stillInactive=%v (%s)",
				sys, ok.Success, len(ok.Messages), stillInactive, stillURI)

			if stillInactive {
				// The SAP-side half of #500: on this system ADT activation of
				// this class is a no-op even for valid source, so there is no
				// genuine activation to regression-test. adtler v0.5.1 does not
				// fix that — it only stops the result from claiming success.
				// That claim is what this branch checks.
				if ok.Success {
					t.Fatalf("#500 NOT fixed on %s: activate_object reported Success:true for %s while it is still listed inactive (%s)",
						sys, name, stillURI)
				}
				t.Logf("SAP-SIDE #500 STILL OPEN on %s: %s stays inactive after activate_object even with valid source "+
					"(package Z_ADT_MCP_TEST, transportable, plain Z* name — no namespace involved). "+
					"The bump makes the tool report this instead of claiming success; it does not make the activation work.",
					sys, name)
				return
			}
			if !ok.Success {
				t.Fatalf("REGRESSION on %s: activate_object reported Success:false for a class that is not inactive: %+v",
					sys, ok.Messages)
			}
		})
	}
}

// queryOneColumn runs a single-column SELECT through run_query and returns
// the values. Used to discover UIAC/UIAD object names from TADIR instead of
// hardcoding names from our own landscape.
func queryOneColumn(t *testing.T, sql string, maxRows int) []string {
	t.Helper()
	res := callTool(t, sharedServer, "run_query", map[string]interface{}{
		"purpose":  "development_metadata",
		"sql":      sql,
		"max_rows": float64(maxRows),
	})
	if res.IsError {
		t.Skipf("run_query failed, cannot discover objects: %s", textOf(res))
	}
	var out struct {
		Rows [][]string
	}
	if err := json.Unmarshal([]byte(textOf(res)), &out); err != nil {
		t.Fatalf("parse run_query: %v\nraw: %s", err, textOf(res))
	}
	vals := make([]string, 0, len(out.Rows))
	for _, r := range out.Rows {
		if len(r) > 0 && strings.TrimSpace(r[0]) != "" {
			vals = append(vals, strings.TrimSpace(r[0]))
		}
	}
	return vals
}

type dependencyResult struct {
	ObjectType   string `json:"object_type"`
	ObjectName   string `json:"object_name"`
	Count        int    `json:"count"`
	Dependencies []struct {
		Name    string `json:"name"`
		UseType string `json:"use_type"`
	} `json:"dependencies"`
	Warnings []string `json:"warnings"`
}

func dependencies(t *testing.T, objectType, objectName string) (dependencyResult, string, bool) {
	t.Helper()
	res := callTool(t, sharedServer, "get_object_dependencies", map[string]interface{}{
		"object_type": objectType,
		"object_name": objectName,
	})
	if res.IsError {
		return dependencyResult{}, textOf(res), false
	}
	var out dependencyResult
	if err := json.Unmarshal([]byte(textOf(res)), &out); err != nil {
		t.Fatalf("parse get_object_dependencies: %v\nraw: %s", err, textOf(res))
	}
	return out, textOf(res), true
}

// TestBumpVerify_409_FioriCatalogDependencies is the #409 reproducer. Before
// this bump, get_object_dependencies rejected UIAC and UIAD with
// "unsupported object type" — aibap.mcp#512 already advertises both types,
// so until v0.5.1 the tool promised something the pinned adtler could not
// do. Object names come from TADIR at runtime; a system that carries no
// UIAC objects (the ECC one, per #409) must still answer the call without
// an error, which is its own acceptance criterion.
func TestBumpVerify_409_FioriCatalogDependencies(t *testing.T) {
	// Fallback catalog if TADIR has no UIAC row: SAP-delivered, and already
	// named in the get_object_dependencies tool description.
	const fallbackCatalog = "SAP_TC_FIN_CO_BE_APPS"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			catalogs := queryOneColumn(t,
				"SELECT OBJ_NAME FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'UIAC' ORDER BY OBJ_NAME", 5)
			catalog := fallbackCatalog
			if len(catalogs) > 0 {
				catalog = catalogs[0]
			} else {
				t.Logf("no UIAC objects in TADIR on %s — using %s to check the empty-but-not-an-error case",
					sys, fallbackCatalog)
			}

			catRes, raw, ok := dependencies(t, "UIAC", catalog)
			if !ok {
				t.Fatalf("#409 NOT fixed on %s: get_object_dependencies(UIAC) errored: %s", sys, raw)
			}
			t.Logf("UIAC on %s: count=%d warnings=%v", sys, catRes.Count, catRes.Warnings)
			// #409 acceptance criterion: each app entry of a catalog carries
			// use type UI_APP.
			for _, d := range catRes.Dependencies {
				if d.UseType != "UI_APP" {
					t.Errorf("UIAC app entry on %s has use_type %q, want UI_APP", sys, d.UseType)
				}
			}

			// Pick a UIAD: an app entry of the catalog if there is one,
			// otherwise any UIAD in TADIR (the ECC system has those).
			var app string
			for _, d := range catRes.Dependencies {
				if d.Name != "" {
					app = d.Name
					break
				}
			}
			if app == "" {
				if apps := queryOneColumn(t,
					"SELECT OBJ_NAME FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'UIAD' ORDER BY OBJ_NAME", 5); len(apps) > 0 {
					app = apps[0]
				}
			}
			if app == "" {
				t.Logf("no UIAD object available on %s — UIAC half verified, UIAD half not exercised", sys)
				return
			}

			appRes, rawApp, ok := dependencies(t, "UIAD", app)
			if !ok {
				t.Fatalf("#409 NOT fixed on %s: get_object_dependencies(UIAD) errored: %s", sys, rawApp)
			}
			t.Logf("UIAD on %s: count=%d warnings=%v", sys, appRes.Count, appRes.Warnings)
			for _, d := range appRes.Dependencies {
				t.Logf("  launch target: use_type=%s", d.UseType)
			}
			// #409 acceptance criterion: an app entry with no launch target
			// (app type R, a URL app) answers with an empty list plus a
			// warning, not an error. The error case is already covered above.
			if appRes.Count == 0 && len(appRes.Warnings) == 0 {
				t.Errorf("UIAD on %s resolved to nothing and said nothing — expected a warning", sys)
			}
		})
	}
}

// TestBumpVerify_ZZCleanup removes leftovers of a previous failed run of
// TestBumpVerify_500_ActivationTellsTheTruth. Run it explicitly when the
// #500 test aborts before its own cleanup:
//
//	BUMP_CLEANUP_TRANSPORT=<request> go test -tags integration -v -count=1 -run TestBumpVerify_ZZCleanup ./tools/...
func TestBumpVerify_ZZCleanup(t *testing.T) {
	const name = "ZCL_ADT_MCP_BUMP551"
	const uri = "/sap/bc/adt/oo/classes/zcl_adt_mcp_bump551"
	tr := os.Getenv("BUMP_CLEANUP_TRANSPORT")
	if tr == "" {
		t.Skip("BUMP_CLEANUP_TRANSPORT not set")
	}
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			ex := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
			t.Logf("object_exists: %s", textOf(ex))
			fu := callTool(t, sharedServer, "force_unlock", map[string]interface{}{"object_uri": uri})
			t.Logf("force_unlock: isError=%v %s", fu.IsError, textOf(fu))
			del := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": uri, "transport": tr})
			t.Logf("delete_object: isError=%v %s", del.IsError, textOf(del))
			ex2 := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
			t.Logf("object_exists after: %s", textOf(ex2))
		})
	}
}
