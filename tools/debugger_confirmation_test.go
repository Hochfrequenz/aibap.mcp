package tools_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestDebugStepTerminateDebuggee_ElicitationDeclined pins that
// debug_step(terminateDebuggee) — the one destructive action among
// debug_step's six (it kills the debuggee's ABAP work process, discarding
// whatever that session was doing) — asks for confirmation via
// ConfirmDestructive before it does anything, and that a decline aborts
// before adtler's DebugSession is even constructed.
//
// That last part is why this test is possible at all: adtler's
// NewDebugSession panics unconditionally against any Client that isn't
// *httpClient/*ClientRegistry (see resolveHTTPClient), which is why every
// debug_* tool sits in knownOptOuts for the reflective
// TestStructuredContentIsObject / TestGuardedToolsAskForConfirmation guards
// and can't be added to guardedTools without tripping their
// already-in-knownOptOuts staleness check. Gating the confirmation before
// getSession(user) is called keeps the decline path — and only the decline
// path — testable against mockClient: a decline that reached adtler would
// panic here exactly like every other debug_* call does, so this test
// failing to panic is itself evidence the gate ran first.
//
// The accept path (confirmation approved, Step actually called) can't be
// unit-tested the same way — it still needs a real adtler DebugSession.
// That path is covered live instead: see issue #501's 2026-09-18
// end-to-end verification, which exercised debug_step (stepInto,
// stepContinue) against a real attached debuggee.
func TestDebugStepTerminateDebuggee_ElicitationDeclined(t *testing.T) {
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(&mockClient{}, nil, el)

	result := callTool(t, s, debugStepToolName, map[string]interface{}{
		"action": "terminateDebuggee",
		"user":   "TESTUSER",
	})

	if !result.IsError {
		t.Fatal("expected error result on decline")
	}
	if el.called != 1 {
		t.Errorf("expected 1 confirmation request, got %d", el.called)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "debug_step(terminateDebuggee) aborted") {
		t.Errorf("expected abort message, got: %s", text)
	}
	if !strings.Contains(el.lastMessage, "TESTUSER") {
		t.Errorf("expected confirmation message to name the affected user, got: %s", el.lastMessage)
	}
}

// TestDebugStepOtherActions_DoNotAskForConfirmation pins the other half:
// the five non-destructive actions must not route through
// ConfirmDestructive at all. Each of these proceeds to getSession(), which
// panics against mockClient — that panic (recovered here) is itself proof
// the action reached adtler rather than stopping at a confirmation gate;
// if any of these accidentally started asking for confirmation first, this
// test would see 0 confirmation requests before the panic, not after.
func TestDebugStepOtherActions_DoNotAskForConfirmation(t *testing.T) {
	for _, action := range []string{"stepInto", "stepOver", "stepReturn", "stepContinue", "detachDebugger"} {
		t.Run(action, func(t *testing.T) {
			el := &stubElicitor{result: &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true}},
			}}
			s := newTestServerWithFallbackElicitor(&mockClient{}, nil, el)

			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected a panic from adtler's NewDebugSession (mockClient is not *httpClient/*ClientRegistry) — "+
						"got none, which would mean %s stopped before reaching adtler", action)
				}
				if el.called != 0 {
					t.Errorf("%s: expected 0 confirmation requests, got %d", action, el.called)
				}
			}()

			callTool(t, s, debugStepToolName, map[string]interface{}{
				"action": action,
				"user":   "TESTUSER",
			})
		})
	}
}

// TestDebugStepUnrecognisedAction_RejectedBeforeReachingAdtler guards the
// gate-bypass risk flagged in independent review: this server does not call
// mcp-go's server.WithInputSchemaValidation, so the declared mcp.Enum on
// debug_step's "action" parameter is advisory only — nothing stops a caller
// from sending a string that isn't one of the six canonical values.
// Without the handler's own validDebugStepActionSet check, an unrecognised
// spelling would fall through the exact-match
// `action == "terminateDebuggee"` gate uncaught and reach adtler (and from
// there SAP) with no confirmation, on the unverified assumption that SAP's
// kernel rejects every non-exact spelling of a real method name the same
// way it rejected "continue" in issue #501. This test asserts the
// rejection happens in the handler, before getSession() — proven the same
// way the other tests in this file are: no panic means it never touched
// adtler, and 0 confirmation requests means it never reached the
// terminateDebuggee gate either.
func TestDebugStepUnrecognisedAction_RejectedBeforeReachingAdtler(t *testing.T) {
	cases := []string{
		"bogus",
		"TerminateDebuggee",          // wrong case
		"terminateDebuggee ",         // trailing whitespace
		"terminateDebuggee&method=x", // injection attempt against adtler's unescaped "?method=%s"
		"",
	}
	for _, action := range cases {
		t.Run(action, func(t *testing.T) {
			el := &stubElicitor{result: &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true}},
			}}
			s := newTestServerWithFallbackElicitor(&mockClient{}, nil, el)

			result := callTool(t, s, debugStepToolName, map[string]interface{}{
				"action": action,
				"user":   "TESTUSER",
			})

			if !result.IsError {
				t.Fatalf("action %q: expected error result, got success — this means it reached adtler or was accepted as a real action", action)
			}
			if el.called != 0 {
				t.Errorf("action %q: expected 0 confirmation requests, got %d — the unrecognised action should be rejected before any gate", action, el.called)
			}
			if !strings.Contains(resultText(t, result), "unrecognised action") {
				t.Errorf("action %q: expected an 'unrecognised action' error, got: %s", action, resultText(t, result))
			}
		})
	}
}

// TestDebugStepDescription_NilElicitorDoesNotPromiseConfirmation guards the
// false-safety-claim risk flagged in independent review: debug_step's
// description can't use the shared tools.DestructiveConfirmationNote /
// guardedTools mechanism (see the comment on
// TestDebugStepTerminateDebuggee_ElicitationDeclined for why), which means
// it also isn't covered by TestNilElicitorBuildPromisesNoConfirmation —
// that test only walks guardedTools. A hand-written sentence claiming
// terminateDebuggee "asks the MCP client to confirm" would be exactly as
// false in a nil-Elicitor build (RegisterAll; ConfirmDestructive returns
// proceed=true without asking) as the bug #475 fixed for the shared note.
// This test is debug_step's own equivalent of that guard.
func TestDebugStepDescription_NilElicitorDoesNotPromiseConfirmation(t *testing.T) {
	s := newTestServerWithFallbackElicitor(&mockClient{}, nil, nil)
	found := false
	for _, tl := range listRegisteredTools(t, s) {
		if tl.Name != debugStepToolName {
			continue
		}
		found = true
		if strings.Contains(tl.Description, "asks the MCP client to confirm") {
			t.Errorf("debug_step description promises a confirmation in a nil-Elicitor build, where "+
				"ConfirmDestructive proceeds without asking: %s", tl.Description)
		}
		if !strings.Contains(tl.Description, "proceeds without asking for confirmation") {
			t.Errorf("debug_step description should say plainly that no confirmation happens in this build: %s", tl.Description)
		}
	}
	if !found {
		t.Fatal("debug_step not registered")
	}
}

// TestDebugStepDescription_WiredElicitorPromisesConfirmation is the other
// half: when an Elicitor is actually wired in, the description should say
// so — mirrors TestNilElicitorBuildPromisesNoConfirmation's counterpart for
// the shared note.
func TestDebugStepDescription_WiredElicitorPromisesConfirmation(t *testing.T) {
	el := &stubElicitor{}
	s := newTestServerWithFallbackElicitor(&mockClient{}, nil, el)
	found := false
	for _, tl := range listRegisteredTools(t, s) {
		if tl.Name != debugStepToolName {
			continue
		}
		found = true
		if !strings.Contains(tl.Description, "asks the MCP client to confirm") {
			t.Errorf("debug_step description should promise a confirmation when an Elicitor is wired in: %s", tl.Description)
		}
	}
	if !found {
		t.Fatal("debug_step not registered")
	}
}
