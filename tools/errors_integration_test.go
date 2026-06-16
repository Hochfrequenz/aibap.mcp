//go:build integration

package tools_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// errors_integration_test.go verifies PR #407 end-to-end against the live
// systems: that the ADT error-hint rules in tools/errors.go fire correctly
// when a real SAP error flows through the MCP wrapper layer.
//
// PR #407 corrects and completes the hint rules in tools/errors.go. Its
// central claims are about LIVE behaviour — which exception Type and HTTP
// status code each error condition produces on S4U (S/4) vs HFQ (ECC):
//
//	| HTTP      | Exception Type                  | S4U | HFQ    |
//	|-----------|---------------------------------|-----|--------|
//	| 404       | ExceptionResourceNotFound       | ✅  | ✅     |
//	| 400 / 405 | ExceptionResourceAlreadyExists  | 400 | 405    |
//	| 409       | ExceptionResourceLockConflict   | ✅  | absent |
//	| 412       | ExceptionResourceInvalidEtag    | ✅  | ✅     |
//	| 406       | ExceptionResourceNotAcceptable  | ✅  | ✅     |
//	| 415       | ExceptionUnsupportedMediaType   | ✅  | ✅     |
//	| 422       | ExceptionUnprocessableEntity    | ✅  | ✅     |
//	| 405       | ExceptionNotAllowed             | ✅  | absent |
//
// The unit tests (errors_test.go) pin the matcher against synthetic
// adt.ADTError values. These integration tests prove that the synthetic
// inputs actually match what the live systems emit — i.e. that the Type and
// status code surface unchanged through adtler and the MCP error path, and
// that errorResult appends the documented hint.
//
// Coverage boundary (deliberate, see "WrapperUntriggerable" below):
// only 404 and "already exists" can be triggered reliably through the
// high-level MCP tools. The remaining table rows (406/412/415/422/409/405-
// not-allowed) require raw HTTP manipulation (custom Accept/Content-Type/
// If-Match headers, artificial lock conflicts) that the wrapper layer does
// not expose — that surface belongs to adtler's own integration suite, not
// here. Per CLAUDE.md, aibap.mcp integration tests cover the MCP wrapper
// layer only.

// adtErrorLine matches the prefix adt.ADTError.Error() produces:
//
//	"SAP ADT error 404 (ExceptionResourceNotFound): <message>"   (Type present)
//	"SAP ADT error 404: <message>"                               (Type empty)
//
// The Type group is optional so the helper degrades gracefully on legacy
// (<ExceptionText>/HTML/plain) bodies that carry no Type.
var adtErrorLine = regexp.MustCompile(`SAP ADT error (\d+)(?: \(([^)]+)\))?:`)

// parseADTError extracts the HTTP status code and exception Type from an
// errorResult text payload. Returns status 0 / type "" if the text carries
// no recognisable "SAP ADT error N" prefix (e.g. a plain Go error).
func parseADTError(text string) (status int, excType string) {
	m := adtErrorLine.FindStringSubmatch(text)
	if m == nil {
		return 0, ""
	}
	status, _ = strconv.Atoi(m[1])
	return status, m[2]
}

// TestIntegration_ErrorHint_NotFound404 verifies the 404 row of the PR #407
// table end-to-end: GET on a non-existent object URI must surface
// status 404 + Type ExceptionResourceNotFound (both systems) and the
// matcher must append the "Object not found … search_objects" hint.
//
// Read-only: triggers the error without mutating the system.
func TestIntegration_ErrorHint_NotFound404(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			// A URI with no realistic chance of collision.
			uri := "/sap/bc/adt/programs/programs/zzzz_pr407_no_such_object_" + sys

			res := callTool(t, sharedServer, "get_source", map[string]interface{}{
				"object_uri": uri,
			})
			if !res.IsError {
				t.Fatalf("expected IsError=true for non-existent object; got body: %s", textOf(res))
			}
			text := textOf(res)
			status, excType := parseADTError(text)
			t.Logf("get_source 404 probe on %s: status=%d type=%q raw=%q", sys, status, excType, text)

			if status != 404 {
				t.Errorf("expected HTTP 404, got %d; raw: %s", status, text)
			}
			// PR #407: both S4U and HFQ emit ExceptionResourceNotFound for 404.
			if excType != "ExceptionResourceNotFound" {
				t.Errorf("expected Type ExceptionResourceNotFound (PR #407 claims both systems), got %q; raw: %s", excType, text)
			}
			// errorResult must append the 404 hint.
			if !strings.Contains(text, "Hint:") {
				t.Errorf("expected a Hint to be appended for 404; raw: %s", text)
			}
			if !strings.Contains(text, "search_objects") {
				t.Errorf("404 hint should point at search_objects; raw: %s", text)
			}
		})
	}
}

// TestIntegration_ErrorHint_AlreadyExists verifies the flagship claim of
// PR #407: "already exists" returns DIFFERENT status codes on the two
// systems (400 on S4U, 405 on HFQ) but the SAME exception Type
// (ExceptionResourceAlreadyExists), so the Tier-1 Type rule produces one
// consistent hint regardless of status code or logon language.
//
// Trigger: create_object CLAS on a class name that already exists. Verified
// live (issue #406 / PR #407) that the OO-class create endpoint raises the
// genuine ExceptionResourceAlreadyExists — unlike the PROG endpoint, which
// wraps the same condition as a 500 ExceptionResourceCreationFailure (see
// TestIntegration_ErrorHint_ProgramAlreadyExists_500 below). Because the
// class already exists the create fails — nothing is created, so there is
// nothing to clean up.
//
// Target: CL_ABAP_TYPEDESCR, a standard ABAP RTTI class present on every
// system, so the test needs no Z_ADT_MCP_TEST fixture and works identically
// on S4U and HFQ. We never attempt to delete it (it is a standard SAP
// object) — the create simply must fail.
func TestIntegration_ErrorHint_AlreadyExists(t *testing.T) {
	const (
		className = "CL_ABAP_TYPEDESCR"
		uri       = "/sap/bc/adt/oo/classes/cl_abap_typedescr"
	)

	// expectedStatus encodes the PR #407 table. Systems not listed here are
	// not asserted on status (only Type + hint), so the test still adds value
	// on a third system without hard-coding a guess.
	expectedStatus := map[string]int{
		"s4u": 400,
		"hfq": 405,
	}

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			// Defensive: a standard class should always be present. If a
			// target system somehow lacks it, skip rather than risk the
			// create succeeding and littering.
			requireFixture(t, sharedServer, sys, uri)

			// Attempt to re-create the existing class in $TMP. ABAP object
			// names are globally unique, so this must fail with "already
			// exists" regardless of the target package — and because it
			// fails, no object is created and no cleanup is required.
			res := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "CLAS",
				"name":        className,
				"package":     "$TMP",
				"description": "PR #407 already-exists probe (create must fail)",
			})
			if !res.IsError {
				// Impossible for a standard class, but guard anyway. Do NOT
				// attempt to delete a standard SAP object — just fail loudly.
				t.Fatalf("create_object unexpectedly SUCCEEDED for standard class %s on %s; raw: %s", className, sys, textOf(res))
			}

			text := textOf(res)
			status, excType := parseADTError(text)
			t.Logf("create_object already-exists probe on %s: status=%d type=%q raw=%q", sys, status, excType, text)

			// PR #407 flagship: same Type on both systems.
			if excType != "ExceptionResourceAlreadyExists" {
				t.Errorf("expected Type ExceptionResourceAlreadyExists (PR #407), got %q; raw: %s", excType, text)
			}
			// ...but divergent status codes, per the verified table.
			if want, ok := expectedStatus[sys]; ok {
				if status != want {
					t.Errorf("PR #407 table: expected HTTP %d for already-exists on %s, got %d; raw: %s", want, sys, status, text)
				}
			}
			// The hint must be the consistent already-exists guidance,
			// independent of the status code divergence above.
			if !strings.Contains(text, "Hint:") {
				t.Errorf("expected a Hint to be appended for already-exists; raw: %s", text)
			}
			if !strings.Contains(strings.ToLower(text), "already exists") {
				t.Errorf("already-exists hint missing; raw: %s", text)
			}
		})
	}
}

// TestIntegration_ErrorHint_ProgramAlreadyExists_500 verifies the behaviour
// discovered while validating PR #407 live: creating a PROGRAM whose name
// already exists does NOT raise ExceptionResourceAlreadyExists (as the
// OO-class endpoint does). Instead the program-create endpoint raises a 500
// ExceptionResourceCreationFailure whose message names the collision
// (English on S4U, "existiert bereits" on HFQ).
//
// Before the fix this fell through to the generic Tier-2 {statusCode: 500}
// rule and the user got the misleading "SAP server error … check SM21/ST22"
// hint. The Tier-1 {excType: ExceptionResourceCreationFailure} rule added in
// errors.go now precedes that catch-all and produces actionable, language-
// independent guidance pointing at object_exists / search_objects. This test
// is the live regression anchor for that rule on both systems.
func TestIntegration_ErrorHint_ProgramAlreadyExists_500(t *testing.T) {
	const (
		objName = "Z_ADT_MCP_TEST_REPORT"
		uri     = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"
	)

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, uri)

			res := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "PROG",
				"name":        objName,
				"package":     "$TMP",
				"description": "PR #407 program already-exists probe (create must fail)",
			})
			if !res.IsError {
				// The fixture guard said the program exists, so a success here
				// means a duplicate local PROG was just created. Fail and clean
				// it up — a $TMP program is safe to delete.
				t.Errorf("create_object unexpectedly SUCCEEDED for existing program %s on %s; raw: %s", objName, sys, textOf(res))
				cleanup := callTool(t, sharedServer, "delete_object", map[string]interface{}{
					"object_uri": "/sap/bc/adt/programs/programs/" + strings.ToLower(objName),
				})
				t.Logf("cleanup delete_object on %s: isError=%v body=%s", sys, cleanup.IsError, textOf(cleanup))
				return
			}

			text := textOf(res)
			status, excType := parseADTError(text)
			t.Logf("create_object PROG already-exists on %s: status=%d type=%q raw=%q", sys, status, excType, text)

			// Pin the discovered reality: PROG-create wraps "already exists"
			// as a 500 ExceptionResourceCreationFailure.
			if status != 500 {
				t.Errorf("expected HTTP 500 for PROG already-exists (CreationFailure wrapping), got %d; raw: %s", status, text)
			}
			if excType != "ExceptionResourceCreationFailure" {
				t.Errorf("expected Type ExceptionResourceCreationFailure for PROG already-exists, got %q; raw: %s", excType, text)
			}
			// The SAP message DOES name the real cause...
			if !strings.Contains(strings.ToLower(text), "already exists") && !strings.Contains(strings.ToLower(text), "existiert bereits") {
				t.Errorf("expected the SAP message to mention the object already exists; raw: %s", text)
			}
			// ...and the Tier-1 CreationFailure rule (PR #407 follow-up) now
			// surfaces actionable guidance instead of the generic 500
			// "check ST22" hint. Assert the new hint, and that the misleading
			// short-dump guidance is gone.
			if !strings.Contains(text, "Hint:") {
				t.Errorf("expected a Hint to be appended for PROG creation failure; raw: %s", text)
			}
			if !strings.Contains(text, "object_exists") {
				t.Errorf("expected the CreationFailure hint to point at object_exists/search_objects; raw: %s", text)
			}
			if strings.Contains(text, "SM21") {
				t.Errorf("CreationFailure should no longer get the misleading generic-500 (SM21/ST22) hint; raw: %s", text)
			}
		})
	}
}

// TestIntegration_ErrorHint_WrapperUntriggerable documents — in executable
// form — why the remaining PR #407 table rows are NOT covered by live
// triggers here. It always skips; its purpose is to make the coverage
// boundary explicit and greppable in `-v` output so a reviewer sees the
// omission is deliberate, not forgotten.
//
// Each row below cannot be provoked through the high-level MCP tools:
//
//   - 412 ExceptionResourceInvalidEtag — patch_source / set_source re-fetch
//     the current ETag immediately before the write (see tools/patch.go:
//     srcResult.ETag), so a stale If-Match can never be injected via the
//     wrapper. Triggering 412 needs a hand-crafted If-Match, which is
//     adtler's layer.
//   - 406 ExceptionResourceNotAcceptable / 415 ExceptionUnsupportedMediaType
//     — Accept and Content-Type are chosen by adtler's content negotiation
//     (NegotiateContentType / acceptHeaderForURI). The wrapper exposes no way
//     to send a deliberately wrong header.
//   - 422 ExceptionUnprocessableEntity — semantic-validation failures depend
//     on endpoint-specific malformed payloads that the typed tool arguments
//     prevent us from constructing.
//   - 409 ExceptionResourceLockConflict / 405 ExceptionNotAllowed — require an
//     artificial concurrent save/lock conflict and a method the resource
//     rejects, respectively; neither is reachable through a single tool call.
//
// These are verified at the source level in PR #407 (read from the live
// GET_HTTP_STATUS ABAP) and pinned by the unit tests in errors_test.go. The
// HTTP/XML layer that produces them is adtler's responsibility and is tested
// there.
func TestIntegration_ErrorHint_WrapperUntriggerable(t *testing.T) {
	t.Skip("PR #407 rows 406/412/415/422/409/405-not-allowed are not triggerable through the MCP wrapper layer — covered by errors_test.go unit tests and adtler. See doc comment for the per-row rationale.")
}
