package tools

import (
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The workflow guide is not tied to a tool group: the server instructions
// point every session at it, so it must be listed and readable whatever
// groups are enabled — even none.
func TestWorkflowGuideResource_AlwaysRegistered(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups map[string]bool
	}{
		{name: "default groups", groups: DefaultGroups()},
		{name: "no groups", groups: map[string]bool{}},
		{name: "debug only", groups: map[string]bool{"debug": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := server.NewMCPServer("test", "0")
			RegisterAllWithLockMap(s, nil, nil, nil, tc.groups, nil)

			if listed := listedResourceURIs(t, s); !slices.Contains(listed, WorkflowGuideURI) {
				t.Fatalf("resources/list does not contain %s (listed: %v)", WorkflowGuideURI, listed)
			}
			if text := readResourceText(t, s, WorkflowGuideURI); text != defaultWorkflowGuide {
				t.Errorf("resources/read returned %d bytes, want the embedded default (%d bytes)", len(text), len(defaultWorkflowGuide))
			}
		})
	}
}

// A downstream build replaces the text under the same URI, so the pointer in
// the server instructions stays valid in both builds.
func TestWorkflowGuideResource_Override(t *testing.T) {
	const override = "# Team workflow\n\nOur own rules.\n"
	s := server.NewMCPServer("test", "0")
	RegisterAllWithLockMap(s, nil, nil, nil, DefaultGroups(), nil, WithWorkflowGuide(override))

	if text := readResourceText(t, s, WorkflowGuideURI); text != override {
		t.Errorf("resources/read returned %q, want the override", text)
	}
	if n := strings.Count(strings.Join(listedResourceURIs(t, s), "\n"), WorkflowGuideURI); n != 1 {
		t.Errorf("resources/list contains %s %d times, want exactly once", WorkflowGuideURI, n)
	}
}

// An empty override is the "not set" value of the downstream variable and
// must fall back to the public default rather than serve an empty guide.
func TestWorkflowGuideResource_EmptyOverrideKeepsDefault(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	RegisterAllWithLockMap(s, nil, nil, nil, DefaultGroups(), nil, WithWorkflowGuide(""))

	if text := readResourceText(t, s, WorkflowGuideURI); text != defaultWorkflowGuide {
		t.Errorf("empty override: resources/read returned %d bytes, want the default (%d bytes)", len(text), len(defaultWorkflowGuide))
	}
}

// Sanity check that the embed is the real guide: it carries the decision tree
// roots, the round trip through git, and the tool limits it promises. Not a
// check that every sentence is right.
func TestWorkflowGuideContent(t *testing.T) {
	for _, want := range []string{
		"self-contained",
		"embedded",
		"export_package",
		"rollback_transport",
		"release_transport",
		"transport of copies",
		"abapgit_pull",
		"SAP-first",
		"files-first",
	} {
		if !strings.Contains(strings.ToLower(defaultWorkflowGuide), strings.ToLower(want)) {
			t.Errorf("workflow guide does not mention %q", want)
		}
	}
}
