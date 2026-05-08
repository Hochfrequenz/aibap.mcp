//go:build integration

// Throwaway reproducer harness for the adtler v0.1.4 → v0.1.5 bump.
// Walks the #319 tracking issue checklist and exercises each blocker
// against the live target system(s). Delete after the bump PR merges.
//
// Run:
//   go test -tags integration -v -count=1 -run BumpVerify ./tools/...
//
// Requires VPN + ~/.config/sap-mcp/systems.json + Z_ADT_MCP_TEST package
// installed on the targeted systems.

package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// #288 — run_atc_check returns HTTP 500 on R/3 even with check_variant=DEFAULT.
// adtler#12 (PR #50): adopt the canonical 3-step flow (worklist POST → run →
// fetch results) so R/3 stops 500-ing.
func TestBumpVerify_ATC_R3_NoMore500(t *testing.T) {
	const sys = "hfq"
	const objectURI = "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle"

	requireReachable(t, sys)
	if _, err := registry.Select(sys); err != nil {
		t.Fatalf("registry.Select(%q): %v", sys, err)
	}

	ctx := context.Background()

	// First confirm get_atc_customizing still answers — this is the
	// "natural sequence" that #319 calls out as adtler#44.
	cust, err := registry.GetATCCustomizing(ctx)
	if err != nil {
		t.Fatalf("GetATCCustomizing on %s: %v", sys, err)
	}
	t.Logf("R/3 system check variant = %q", cust.SystemCheckVariant)

	// Then run the ATC check. Pre-bump: HTTP 500 with empty body.
	// Post-bump: a (possibly empty) finding list, no error.
	res, err := registry.RunATCCheck(ctx, []string{objectURI}, "")
	if err != nil {
		t.Fatalf("RunATCCheck on R/3 still failing — adtler#12 fix did not land: %v", err)
	}
	t.Logf("R/3 ATC: worklist=%s, %d findings", res.WorklistID, len(res.Findings))

	// Same call with explicit check_variant — the case the original
	// reproducer in #288 also exercises.
	res, err = registry.RunATCCheck(ctx, []string{objectURI}, cust.SystemCheckVariant)
	if err != nil {
		t.Fatalf("RunATCCheck on R/3 with check_variant=%q still failing: %v",
			cust.SystemCheckVariant, err)
	}
	t.Logf("R/3 ATC (variant=%s): worklist=%s, %d findings",
		cust.SystemCheckVariant, res.WorklistID, len(res.Findings))
}

// Sanity check: ATC on S/4 still works (was already green pre-bump).
// Catches any regression introduced by the 3-step flow.
func TestBumpVerify_ATC_S4_StillWorks(t *testing.T) {
	const sys = "s4u"
	const objectURI = "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle"

	requireReachable(t, sys)
	if _, err := registry.Select(sys); err != nil {
		t.Fatalf("registry.Select(%q): %v", sys, err)
	}

	ctx := context.Background()
	res, err := registry.RunATCCheck(ctx, []string{objectURI}, "")
	if err != nil {
		t.Fatalf("RunATCCheck on S/4: %v", err)
	}
	t.Logf("S/4 ATC: worklist=%s, %d findings", res.WorklistID, len(res.Findings))
}

// adtler#6 (PR #53) — GetCompletions now returns real proposals; pre-fix it
// returned nil for every input. Bonus verification, not on the #319 checklist.
func TestBumpVerify_GetCompletions_S4(t *testing.T) {
	const sys = "s4u"
	const objectURI = "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle"

	requireReachable(t, sys)
	if _, err := registry.Select(sys); err != nil {
		t.Fatalf("registry.Select(%q): %v", sys, err)
	}

	// Canary pattern from adtler's TestGetCompletions_Integration:
	// `WRITE ` mid-file at column 6 reliably triggers ~20 proposals on
	// both R/3 and S/4 when the URI fragment + asXML parsing both work.
	source := "REPORT z_adt_mcp_fullcycle.\nWRITE "
	items, err := registry.GetCompletions(context.Background(), objectURI, source, 2, 6)
	if err != nil {
		t.Fatalf("GetCompletions: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("GetCompletions returned 0 proposals — adtler#6 fix did not land")
	}
	t.Logf("S/4 GetCompletions on cursor after 'WRITE ': %d proposals (first: %q)", len(items), items[0].Text)
}

// #243 — set_text_elements MCP tool blocked on adtler-side write API.
// adtler PR #3 added SetTextElements to the DocuClient interface. The MCP
// wrapper itself is on PR #243's feature branch and will be rebased onto
// main after this bump merges; here we verify only that the adtler-level
// write API works end-to-end so PR #243 has a green path forward.
//
// Strategy: lock the test program, write empty text elements (no-op, since
// the fixture has none anyway), unlock. If any of these fail the bump must
// not ship — PR #243 would still be blocked.
func TestBumpVerify_SetTextElements_S4(t *testing.T) {
	const sys = "s4u"
	const objectURI = "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle"
	// S/4 binds textelement writes to a *separate* enqueue resource than
	// the program lock — the lockHandle must be acquired on the
	// textelements URI, not the program URI. See adtler
	// settextelements_transport_integration_test.go.
	const textElementsURI = "/sap/bc/adt/textelements/programs/z_adt_mcp_fullcycle"

	requireReachable(t, sys)
	if _, err := registry.Select(sys); err != nil {
		t.Fatalf("registry.Select(%q): %v", sys, err)
	}

	ctx := context.Background()

	lockHandle, err := registry.LockObject(ctx, textElementsURI)
	if err != nil {
		t.Fatalf("LockObject(textelements): %v", err)
	}
	defer func() {
		if uerr := registry.UnlockObject(ctx, textElementsURI, lockHandle); uerr != nil {
			t.Errorf("UnlockObject (cleanup): %v", uerr)
		}
	}()

	// Z_ADT_MCP_TEST is transport-managed on s4u. Fresh modifiable
	// workbench transport created out-of-band for this verification —
	// adapt or recreate before running.
	const transport = "S4UK903032"
	injected := adt.TextSymbol{Key: "001", Text: "bump v0.1.5 verify", MaxLength: 30}
	err = registry.SetTextElements(ctx, objectURI,
		[]adt.TextSymbol{injected}, nil, lockHandle, transport)
	if err != nil {
		t.Fatalf("SetTextElements: %v", err)
	}

	after, err := registry.GetTextElements(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetTextElements (verify): %v", err)
	}
	found := false
	for _, s := range after.Symbols {
		if s.Key == injected.Key && s.Text == injected.Text {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("injected text symbol %+v not present after write; got %+v", injected, after.Symbols)
	}
	t.Logf("S/4 SetTextElements roundtrip succeeded — adtler#3 API reachable, %d symbol(s) read back", len(after.Symbols))
}

// adtler#8 (PR #54) — NavigateToDefinition now reads the source body and
// returns the actual definition URI. Pre-fix it just echoed the cursor URI back.
//
// Smoke test only: passes a synthetic source body referencing a SAP standard
// class and asserts the returned URI is *different* from the input. The MCP
// wrapper's signature change is exercised by go test ./...; this just confirms
// end-to-end the new body-passing protocol speaks to the live SAP handler.
func TestBumpVerify_NavigateToDefinition_S4(t *testing.T) {
	const sys = "s4u"
	const sourceURI = "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle/source/main#start=2,5"

	requireReachable(t, sys)
	if _, err := registry.Select(sys); err != nil {
		t.Fatalf("registry.Select(%q): %v", sys, err)
	}

	// Synthetic source: line 1 REPORT, line 2 references cl_abap_unit_assert
	// at column 5..29 → "cl_abap_unit_assert".
	source := "REPORT z_adt_mcp_fullcycle.\n    cl_abap_unit_assert=>fail( )."
	target, err := registry.NavigateToDefinition(context.Background(), sourceURI, source)
	if err != nil {
		t.Fatalf("NavigateToDefinition: %v", err)
	}
	if target == "" {
		t.Fatalf("NavigateToDefinition returned empty target — adtler#8 fix did not land")
	}
	if strings.HasPrefix(target, "/sap/bc/adt/programs/programs/z_adt_mcp_fullcycle") {
		t.Fatalf("NavigateToDefinition still echoes the source URI back (got %q) — adtler#8 fix did not land", target)
	}
	t.Logf("S/4 NavigateToDefinition resolved cl_abap_unit_assert to %q", target)
}
