package tools_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// metaKeyRequiresUserInteraction is the Claude Code _meta key under test,
// spelled out here rather than imported from the package under test so a typo
// in the production constant fails the test instead of travelling with it.
// Documented at https://code.claude.com/docs/en/mcp under "Require approval
// for a specific tool".
const metaKeyRequiresUserInteraction = "anthropic/requiresUserInteraction"

// wantInteractionRequired is the set of tools whose effects this server cannot
// undo, and which therefore carry the annotation in strict consent mode. Kept
// as a literal so widening the set is a deliberate edit to a test rather than
// a side effect of a registration change.
//
// rename, remove_from_transport and run_query are absent on purpose — see the
// exclusion notes on tools.irreversibleTools. They keep the client's normal
// permission flow, including "don't ask again", in both modes.
var wantInteractionRequired = []string{
	"delete_object",
	"delete_transport",
	"release_transport",
	"rollback_transport",
	"run_class",
	"update_customizing",
}

func newConsentTestServer(t *testing.T, opts ...tools.RegisterOption) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(
		s, &mockClient{}, &mockSelector{}, adt.NewLockMap(),
		tools.ParseToolGroups([]string{"all"}), &confirmProbeFallback{}, opts...,
	)
	return s
}

// interactionRequiredTools reports which tools carry the annotation on the
// wire, reading tools/list rather than the Go values that produced it — the
// client only ever sees the wire form, and the docs require the JSON boolean
// true specifically.
func interactionRequiredTools(t *testing.T, s *server.MCPServer) []string {
	t.Helper()
	var names []string
	for _, tool := range listRegisteredTools(t, s) {
		raw, present := tool.Meta[metaKeyRequiresUserInteraction]
		if !present {
			continue
		}
		if raw != true {
			t.Errorf(
				"%s sets %s to %#v; Claude Code ignores any value that is not the JSON "+
					"boolean true, so the annotation would silently do nothing",
				tool.Name, metaKeyRequiresUserInteraction, raw,
			)
			continue
		}
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func TestParseConsentMode(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    tools.ConsentMode
		wantErr bool
	}{
		{in: "", want: tools.ConsentStrict},
		{in: "strict", want: tools.ConsentStrict},
		{in: "prompt", want: tools.ConsentPrompt},
		{in: "Strict", wantErr: true},
		{in: "yolo", wantErr: true},
	} {
		t.Run("input_"+tc.in, func(t *testing.T) {
			got, err := tools.ParseConsentMode(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseConsentMode(%q) = %q, want an error", tc.in, got)
				}
				for _, mode := range []string{"strict", "prompt"} {
					if !strings.Contains(err.Error(), mode) {
						t.Errorf("error for %q should name the valid mode %q, got: %v", tc.in, mode, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConsentMode(%q) returned error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseConsentMode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStrictConsentAnnotatesTheIrreversibleTools pins the default. The
// annotation is what stops one "Yes, and don't ask again" click from granting
// consent for every later call to these four tools (issue #507).
func TestStrictConsentAnnotatesTheIrreversibleTools(t *testing.T) {
	got := interactionRequiredTools(t, newConsentTestServer(t))

	want := append([]string(nil), wantInteractionRequired...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf(
			"strict consent annotates %v, want %v — a tool added to or removed from the "+
				"annotated set needs the same edit in wantInteractionRequired and in the README",
			got, want,
		)
	}
}

// TestStrictConsentIsTheDefault guards the direction of the default: a build
// that passes no consent option must protect, not expose.
func TestStrictConsentIsTheDefault(t *testing.T) {
	explicit := interactionRequiredTools(t, newConsentTestServer(t, tools.WithConsentMode(tools.ConsentStrict)))
	implicit := interactionRequiredTools(t, newConsentTestServer(t))

	if strings.Join(explicit, ",") != strings.Join(implicit, ",") {
		t.Errorf(
			"registering without a consent option annotates %v but explicit strict annotates "+
				"%v; the zero value must mean strict",
			implicit, explicit,
		)
	}
}

// TestPromptConsentAnnotatesNothing is the other half: --consent=prompt is the
// mode a user picks to get "don't ask again" back, so no tool may keep the
// annotation that suppresses it.
func TestPromptConsentAnnotatesNothing(t *testing.T) {
	got := interactionRequiredTools(t, newConsentTestServer(t, tools.WithConsentMode(tools.ConsentPrompt)))

	if len(got) != 0 {
		t.Errorf(
			"prompt consent still annotates %v; those tools would keep prompting on every "+
				"call with no way to remember the answer, which is the mode's whole purpose",
			got,
		)
	}
}

// TestAnnotatedToolsDeclareThemselvesDestructive keeps the annotation honest
// against the tool's own MCP annotations, which are the server's other public
// statement about what a tool does. Forcing an unsuppressible prompt on a tool
// this server advertises as read-only or non-destructive would cost every
// caller a dialog the tool's own metadata says is unnecessary, and the two
// statements would contradict each other on the wire.
//
// The reverse is deliberately not asserted: several tools declare
// destructiveHint true and are still recoverable, so they stay out of
// irreversibleTools. See the exclusion notes there.
func TestAnnotatedToolsDeclareThemselvesDestructive(t *testing.T) {
	s := newConsentTestServer(t)
	byName := make(map[string]listedTool)
	for _, tool := range listRegisteredTools(t, s) {
		byName[tool.Name] = tool
	}

	for _, name := range interactionRequiredTools(t, s) {
		tool := byName[name]
		if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
			t.Errorf(
				"%s is annotated %s but does not declare destructiveHint true; a tool that "+
					"forces a prompt on every call must say it is destructive",
				name, metaKeyRequiresUserInteraction,
			)
		}
		if tool.Annotations.ReadOnlyHint == nil || *tool.Annotations.ReadOnlyHint {
			t.Errorf(
				"%s is annotated %s but declares readOnlyHint true (or omits it); a read-only "+
					"tool has nothing to withhold consent for",
				name, metaKeyRequiresUserInteraction,
			)
		}
	}
}
