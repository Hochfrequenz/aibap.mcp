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

	result := callTool(t, s, "debug_step", map[string]interface{}{
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

			callTool(t, s, "debug_step", map[string]interface{}{
				"action": action,
				"user":   "TESTUSER",
			})
		})
	}
}
