package tools_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// This file covers the elicitation path at the level the existing tests
// deliberately skip: through a real *server.MCPServer and a real client
// session, rather than through a stubElicitor injected directly into a
// handler.
//
// Why that distinction matters (issue #475). The stub tests in
// elicitation_test.go pin what ConfirmDestructive does with a given
// Elicitor response. What none of them pin is the step before that: which
// response a given kind of client actually produces. That mapping lives in
// mcp-go, not in this repo, and getting it wrong is what #475 turned up —
// the helper's doc comment claims a client without elicitation support hits
// the ErrElicitationNotSupported branch and proceeds, but
// (*MCPServer).RequestElicitation never inspects the declared capability. It
// only type-asserts the session to server.SessionWithElicitation, and
// mcp-go's stdioSession satisfies that unconditionally. So on this
// server's transport the request goes out regardless of what the client
// declared, and whatever the client answers decides the outcome.
//
// recordingSession models exactly that: it always implements
// SessionWithElicitation, and its declared capabilities are set
// independently. Tests can therefore state "client that never declared
// elicitation" and "client that did" as separate cases and observe that
// the server currently treats them identically.

// recordingSession is a client session that behaves like mcp-go's
// stdioSession for the purposes of elicitation: it implements
// server.SessionWithElicitation no matter which capabilities the client
// declared during initialize. Every elicitation request is recorded, and
// reply/replyErr decide what the "client" answers.
type recordingSession struct {
	caps        mcp.ClientCapabilities
	info        mcp.Implementation
	notify      chan mcp.JSONRPCNotification
	initialized bool

	elicitations []mcp.ElicitationRequest
	reply        *mcp.ElicitationResult
	replyErr     error
}

func newRecordingSession(caps mcp.ClientCapabilities, reply *mcp.ElicitationResult) *recordingSession {
	return &recordingSession{
		caps:   caps,
		notify: make(chan mcp.JSONRPCNotification, 8),
		reply:  reply,
	}
}

func (s *recordingSession) Initialize()       { s.initialized = true }
func (s *recordingSession) Initialized() bool { return s.initialized }
func (s *recordingSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return s.notify
}
func (s *recordingSession) SessionID() string { return "test-session" }

func (s *recordingSession) GetClientInfo() mcp.Implementation             { return s.info }
func (s *recordingSession) SetClientInfo(info mcp.Implementation)         { s.info = info }
func (s *recordingSession) GetClientCapabilities() mcp.ClientCapabilities { return s.caps }
func (s *recordingSession) SetClientCapabilities(caps mcp.ClientCapabilities) {
	s.caps = caps
}

func (s *recordingSession) RequestElicitation(
	_ context.Context,
	req mcp.ElicitationRequest,
) (*mcp.ElicitationResult, error) {
	s.elicitations = append(s.elicitations, req)
	return s.reply, s.replyErr
}

// confirmationRequests counts the recorded requests that are destructive-op
// confirmations, identified by the boolean "confirm" property
// ConfirmDestructive puts in its requested schema. Counting those rather than
// all elicitations keeps the guard honest if a tool ever elicits for an
// unrelated reason — a missing transport number, say.
func (s *recordingSession) confirmationRequests() int {
	n := 0
	for _, req := range s.elicitations {
		schema, ok := req.Params.RequestedSchema.(map[string]any)
		if !ok {
			continue
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := props["confirm"]; ok {
			n++
		}
	}
	return n
}

// Compile-time proof that recordingSession covers the three interfaces the
// server plumbing cares about. If mcp-go widens any of them, this breaks
// here rather than as a confusing runtime type-assertion miss.
var (
	_ server.ClientSession          = (*recordingSession)(nil)
	_ server.SessionWithClientInfo  = (*recordingSession)(nil)
	_ server.SessionWithElicitation = (*recordingSession)(nil)
)

// declineReply is what a client sends when it refuses without necessarily
// having shown anything to a user. This is the reply observed from a real
// client in #475.
func declineReply() *mcp.ElicitationResult {
	return &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}
}

func acceptReply() *mcp.ElicitationResult {
	return &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}
}

// elicitationCapableCaps is what a client that supports elicitation sends in
// the initialize handshake.
func elicitationCapableCaps() mcp.ClientCapabilities {
	return mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapability{Form: &struct{}{}}}
}

// newSessionServer wires the server the way main.go does — the
// *server.MCPServer is its own Elicitor — and returns a context carrying
// the session, as HandleMessage would receive it on a live transport.
func newSessionServer(
	client adt.Client,
	fallback tools.BlackMagicClient,
	sess *recordingSession,
) (*server.MCPServer, context.Context) {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(
		s, client, &mockSelector{}, adt.NewLockMap(),
		tools.ParseToolGroups([]string{"all"}), fallback, s,
	)
	return s, s.WithContext(context.Background(), sess)
}

// TestElicitation_ClientWithoutDeclaredCapability_IsStillAsked is the
// regression anchor for issue #475.
//
// The client here never declared the elicitation capability. Per MCP
// 2025-06-18 a server must not send elicitation/create to such a client, and
// ConfirmDestructive's doc comment assumes it does not — it documents
// ErrElicitationNotSupported as the branch that gets taken. This test shows
// the request is sent anyway, because nothing between ConfirmDestructive and
// the session checks GetClientCapabilities().Elicitation.
//
// The load-bearing assertion is that the request was sent. The outcome
// assertions below it are secondary, because the outcome depends on what the
// particular client answers (this one declines, another returns a JSON-RPC
// error, a third never replies and the call hangs) — whereas sending the
// request at all is the server-side defect regardless of the answer. When a
// capability gate lands, this test is the one that has to change, and its
// failure message says so.
func TestElicitation_ClientWithoutDeclaredCapability_IsStillAsked(t *testing.T) {
	sess := newRecordingSession(mcp.ClientCapabilities{}, declineReply())
	s, ctx := newSessionServer(&mockClient{}, nil, sess)

	result := callToolCtx(ctx, t, s, "run_class", map[string]interface{}{
		"class_name": "ZCL_ADT_MCP_CLASSRUN_TST",
	})

	if got := sess.confirmationRequests(); got != 1 {
		t.Fatalf(
			"expected exactly 1 confirmation request to a client that never declared the "+
				"elicitation capability (current behaviour, see #475), got %d. If a "+
				"SupportsElicitation gate has landed, this expectation is now 0 — update this "+
				"test and the ConfirmDestructive doc comment together.",
			got,
		)
	}
	if !result.IsError {
		t.Fatal("expected run_class to be blocked after the client declined")
	}
	if text := resultText(t, result); !strings.Contains(text, "run_class aborted") {
		t.Errorf("expected the abort to name the tool, got: %s", text)
	}
}

// TestElicitation_DeclineReasonClaimsAUserActed pins the wording problem
// reported on #475: the abort reason attributes the decision to a person,
// but the server cannot know whether a person was involved. No user is in
// the loop here — the session answers on its own — and the reason still
// reads "user declined the confirmation".
//
// The assertion is on that exact string, so it fails when the wording is
// changed. That failure is the point: whoever reworks it should see this
// test, confirm the new phrasing no longer asserts something the server
// cannot know, and update the expectation deliberately.
func TestElicitation_DeclineReasonClaimsAUserActed(t *testing.T) {
	sess := newRecordingSession(mcp.ClientCapabilities{}, declineReply())
	s, ctx := newSessionServer(&mockClient{}, nil, sess)

	result := callToolCtx(ctx, t, s, "delete_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	})

	if !result.IsError {
		t.Fatal("expected delete_object to be blocked after the client declined")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "delete_object aborted") {
		t.Errorf("expected the abort to name the tool, got: %s", text)
	}
	if !strings.Contains(text, "user declined the confirmation") {
		t.Errorf(
			"expected the current reason text %q, got: %s. If the wording was changed to stop "+
				"claiming a user acted (see #475), update this expectation to the new phrasing.",
			"user declined the confirmation", text,
		)
	}
}

// TestElicitation_ClientWithCapability_AcceptProceeds is the positive
// counterpart: a client that declared the capability and accepts lets the
// operation through to the SAP call.
func TestElicitation_ClientWithCapability_AcceptProceeds(t *testing.T) {
	ran := ""
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			ran = className
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "CLASSRUN_OK"}, nil
		},
	}
	sess := newRecordingSession(elicitationCapableCaps(), acceptReply())
	s, ctx := newSessionServer(mock, nil, sess)

	result := callToolCtx(ctx, t, s, "run_class", map[string]interface{}{
		"class_name": "ZCL_ADT_MCP_CLASSRUN_TST",
	})

	if result.IsError {
		t.Fatalf("expected run_class to proceed after accept, got error: %s", resultText(t, result))
	}
	if ran != "ZCL_ADT_MCP_CLASSRUN_TST" {
		t.Errorf("expected RunClass to be called with the class name, got %q", ran)
	}
	if got := sess.confirmationRequests(); got != 1 {
		t.Errorf("expected exactly 1 confirmation request, got %d", got)
	}
}

// guardedTools is the set of tools that must route through
// ConfirmDestructive, mapped to arguments that actually reach the guard.
// Handlers that validate before confirming (run_class does an existence
// pre-check, update_customizing rejects a nil fallback first) need arguments
// that survive those steps.
//
// run_query is guarded only on the branch where 'purpose' is missing or not a
// recognised development-tooling value, so it is driven without a purpose on
// purpose.
var guardedTools = map[string]map[string]interface{}{
	"delete_object": {
		"object_uri": "/sap/bc/adt/programs/programs/ZDEAD",
	},
	"rename": {
		"source_uri": "/sap/bc/adt/programs/programs/zold/source/main#start=5,7",
		"new_name":   "ZNEW",
	},
	"run_class": {
		"class_name": "ZCL_ADT_MCP_CLASSRUN_TST",
	},
	"run_query": {
		"sql": "SELECT NAME FROM TRDIR",
	},
	"release_transport": {
		"transport": "TESTK900001",
	},
	"delete_transport": {
		"transport": "TESTK900001",
	},
	"remove_from_transport": {
		"task_number":      "TESTK900002",
		"parent_transport": "TESTK900001",
		"pgmid":            "R3TR",
		"object_type":      "PROG",
		"object_name":      "ZDEAD",
		"wb_type":          "PROG/P",
		"position":         "000001",
	},
	"rollback_transport": {
		"transport": "TESTK900001",
	},
	"update_customizing": {
		"table": "V_T077D",
		"entries": []map[string]interface{}{
			{"keys": map[string]interface{}{"BUKRS": "1000"}, "values": map[string]interface{}{"BUTXT": "Test"}},
		},
	},
}

// TestGuardedToolsAskForConfirmation is the completeness guard the elicitation
// work shipped without: it walks the tools the server exposes, invokes each one
// against a session that records elicitation requests, and asserts the set of
// tools that asked for confirmation is exactly guardedTools.
//
// Both directions matter:
//
//   - a tool in guardedTools that does not ask means the ConfirmDestructive
//     call was lost in a refactor;
//   - a tool that asks but is not in guardedTools means a new destructive tool
//     was added without anyone recording the decision here.
//
// Neither direction is covered by the per-tool elicitation tests, which only
// ever exercise tools someone already remembered to write a test for.
//
// Two limits, both deliberate:
//
//   - tools listed in knownOptOuts (the debug_* handlers, which panic on
//     mockClient) are skipped in both directions, so one of those turning
//     guarded stays invisible here;
//   - the negative direction relies on synthesizeArgs' canned values reaching
//     the guard. A future guarded tool that validates arguments before
//     confirming — the shape of run_class's existence pre-check — would abort
//     early and pass silently. Adding it to guardedTools with arguments that do
//     reach the guard is what the positive direction then enforces.
func TestGuardedToolsAskForConfirmation(t *testing.T) {
	probe, _ := newSessionServer(&mockClient{}, &confirmProbeFallback{}, newRecordingSession(mcp.ClientCapabilities{}, nil))
	registered := listRegisteredTools(t, probe)
	known := make(map[string]bool, len(registered))
	for _, tool := range registered {
		known[tool.Name] = true
	}
	// Staleness: an entry naming a tool that no longer exists silently stops
	// guarding anything. Same guard structuredContentShape applies to
	// knownOptOuts.
	for _, name := range sortedGuardedToolNames(guardedTools) {
		if !known[name] {
			t.Errorf("guardedTools[%q]: tool no longer registered — remove the entry", name)
		}
		if _, optedOut := knownOptOuts[name]; optedOut {
			t.Errorf("guardedTools[%q] is also in knownOptOuts — it cannot be verified; resolve the conflict", name)
		}
	}

	for _, tool := range registered {
		tool := tool
		if reason, ok := knownOptOuts[tool.Name]; ok {
			t.Run(tool.Name+"_opted_out", func(t *testing.T) {
				t.Skipf("opted out: %s", reason)
			})
			continue
		}
		wantGuarded := false
		args := synthesizeArgs(tool.InputSchema)
		if override, ok := guardedTools[tool.Name]; ok {
			wantGuarded = true
			args = override
		}

		t.Run(tool.Name, func(t *testing.T) {
			// Accept, so a guarded tool runs its full body and an unguarded
			// tool cannot be recorded as "asked" merely because it aborted.
			sess := newRecordingSession(elicitationCapableCaps(), acceptReply())
			s, ctx := newSessionServer(&mockClient{}, &confirmProbeFallback{}, sess)

			callToolCtx(ctx, t, s, tool.Name, args)

			asked := sess.confirmationRequests() > 0
			switch {
			case wantGuarded && !asked:
				t.Fatalf(
					"%s is listed in guardedTools but never called ConfirmDestructive. Either the "+
						"guard was dropped, or the arguments in guardedTools no longer reach it.",
					tool.Name,
				)
			case !wantGuarded && asked:
				t.Fatalf(
					"%s asked for confirmation but is not listed in guardedTools. If this tool is "+
						"newly destructive, add it to guardedTools (and to the documentation "+
						"covering which tools confirm, see #475).",
					tool.Name,
				)
			}
		})
	}
}

// TestGuardedToolDescriptionsCarryTheConfirmationNote pins the documentation
// half of #475: the fact that a confirmation is requested — and that its
// outcome depends on the client, not necessarily on a person — has to travel
// with the tool, because a caller reading tools/list sees nothing else.
//
// Both directions again: a guarded tool without the note is a tool whose
// caller cannot know it confirms, and an unguarded tool carrying the note
// promises a prompt that never comes. guardedTools is the shared source of
// truth, so a new destructive tool trips this test and
// TestGuardedToolsAskForConfirmation together.
func TestGuardedToolDescriptionsCarryTheConfirmationNote(t *testing.T) {
	probe, _ := newSessionServer(&mockClient{}, &confirmProbeFallback{}, newRecordingSession(mcp.ClientCapabilities{}, nil))

	for _, tool := range listRegisteredTools(t, probe) {
		_, wantNote := guardedTools[tool.Name]
		hasNote := strings.Contains(tool.Description, tools.DestructiveConfirmationNote)
		switch {
		case wantNote && !hasNote:
			t.Errorf(
				"%s routes through ConfirmDestructive but its description does not carry "+
					"tools.DestructiveConfirmationNote — append it (see #475).",
				tool.Name,
			)
		case !wantNote && hasNote:
			t.Errorf(
				"%s carries the confirmation note but is not in guardedTools — either it does "+
					"not actually confirm (drop the note) or the guard is missing (add the entry).",
				tool.Name,
			)
		}
	}
}

// confirmProbeFallback is a no-op BlackMagicClient. update_customizing
// refuses outright when no fallback is configured, which would hide whether
// it confirms; every method succeeds silently so the handler reaches its
// guard.
type confirmProbeFallback struct{}

func (c *confirmProbeFallback) ReleaseTransportFallback(context.Context, string) error { return nil }
func (c *confirmProbeFallback) CreateTransportFallback(
	_ context.Context, _, _, _, _ string,
) (string, error) {
	return "TESTK900001", nil
}
func (c *confirmProbeFallback) UpdateCustomizing(
	_ context.Context, _ string, _ []tools.CustomizingEntry, _ string,
) error {
	return nil
}
func (c *confirmProbeFallback) CreateObjectFallback(
	_ context.Context, _, _, _, _, _ string,
) error {
	return nil
}

// listedTool is the slice of a tools/list entry the reflective tests need.
type listedTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

// listRegisteredTools enumerates the tools a server exposes via tools/list.
// Reflective, so a tool added later is covered without touching the callers.
// Shared with TestStructuredContentIsObject so the two reflective guards
// cannot drift in how they enumerate tools.
func listRegisteredTools(t *testing.T, s *server.MCPServer) []listedTool {
	t.Helper()
	resp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var envelope struct {
		Result struct {
			Tools []listedTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v\nraw: %s", err, string(raw))
	}
	if len(envelope.Result.Tools) == 0 {
		t.Fatal("tools/list returned zero tools — test server misconfigured")
	}
	return envelope.Result.Tools
}

// resultText concatenates the text content of a tool result, so assertions
// do not depend on how many content blocks a handler emitted.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// sortedGuardedToolNames returns the guardedTools keys in a deterministic
// order so test output does not shuffle between runs.
func sortedGuardedToolNames(m map[string]map[string]interface{}) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
