package tools

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ConsentMode selects how this server asks the MCP client's permission system
// to treat the tools whose effects it cannot undo.
//
// Background (#507): the client's permission prompt is what stands between a
// caller and a destructive call, and accepting it once with "Yes, and don't ask
// again" writes an allow rule that auto-approves every later call to that tool
// — on every system, with every argument, for as long as the rule stays in the
// settings file. The rule cannot be narrowed: Claude Code skips any `mcp__`
// rule containing parentheses, and none of these tools takes a system argument
// to match on. Until this flag existed, the only thing that still surfaced such
// a call was an elicitation this server sent on its own, which no allow rule
// can pre-answer — a side effect of how elicitation works rather than a stated
// design property, and one that cost a second dialog for every call.
//
// Strict mode replaces that accident with an explicit statement, for the tools
// where a wrong call cannot be taken back.
type ConsentMode string

const (
	// ConsentStrict annotates the irreversible tools with
	// MetaRequiresUserInteraction. It is the default and the zero value, so a
	// build that passes no option protects rather than exposes.
	ConsentStrict ConsentMode = "strict"

	// ConsentPrompt annotates nothing and leaves every tool on the client's
	// normal permission flow, "don't ask again" included. It is for callers
	// who bring their own safety net — a working copy under abapGit, a
	// throwaway sandbox system — and would rather answer fewer dialogs, and
	// for unattended callers that strict mode would otherwise deny outright.
	ConsentPrompt ConsentMode = "prompt"
)

// MetaRequiresUserInteraction is the Claude Code `_meta` key that marks a tool
// as requiring explicit approval on every call.
//
// Per https://code.claude.com/docs/en/mcp ("Require approval for a specific
// tool"), a tool carrying it shows its permission prompt on every call — even
// in acceptEdits, auto and bypassPermissions modes — and an allow rule that
// already matches the tool does not skip that prompt. In dontAsk mode the call
// is denied instead, as it is for an allow answer from a
// --permission-prompt-tool in headless mode. An Agent SDK host is the
// exception the docs call out: its canUseTool callback does receive these
// calls and may approve them, because such a host is expected to put the
// question to a person.
//
// Two consequences worth stating plainly, because they are the price of the
// mode: the annotation cannot be scoped to a system, namespace or package, and
// no allow rule reverses it. A caller who wants the prompt to be answerable
// once launches the server with --consent=prompt.
//
// Version caveats. The key requires Claude Code v2.1.199 or later; earlier
// versions ignore it and apply their normal permission flow. Between v2.1.199
// and v2.1.245 the prompt for an annotated tool still displayed a "Yes, and
// don't ask again" option; per the v2.1.246 changelog entry the allow rule it
// wrote was ignored, so the prompt kept reappearing, but a user on those
// versions sees an option that does nothing. From v2.1.246 the option is no
// longer offered. Other MCP clients may treat the key differently or not at
// all.
const MetaRequiresUserInteraction = "anthropic/requiresUserInteraction"

// irreversibleTools lists the tools annotated in ConsentStrict mode: those
// whose effect this server offers no way to undo.
//
// Each entry costs every caller a dialog that no allow rule can suppress, so
// an entry needs its argument written down with it:
//
//   - delete_object — the object is gone; SAP has no undo and the source is
//     not under version control on the server side.
//   - delete_transport — its own tool description says "This is irreversible",
//     and nothing here can recreate the request with its original number.
//   - release_transport — a released request cannot be un-released; the change
//     is on its way to the next system.
//   - rollback_transport — restores an earlier state by overwriting the
//     current one, so a wrong call destroys work the same way a delete does.
//   - run_class — executes arbitrary ABAP under the configured user. What it
//     does is not part of the call, so no confirmation text can describe its
//     effect and no caller can bound it in advance.
//   - update_customizing — an entry with op "delete" removes a customizing
//     row through SAP GUI automation. Customizing tables have no version
//     database, so a removed row cannot be reconstructed from this server.
//
// The three guarded-looking tools left out are left out on the merits, not by
// oversight:
//
//   - run_query runs SELECT statements only; it changes nothing. Its purpose
//     gate is a scope check under the SAP API Policy, not a consent step.
//   - rename rewrites source, which the SAP version database still holds; a
//     second rename puts the old name back.
//   - remove_from_transport changes an object's link to a transport, not the
//     object, and add_to_transport restores the link. Its own description
//     warns that a stale position can remove the wrong entry, which is a
//     correctness problem in that tool rather than an irreversible one.
//
// TestStrictConsentAnnotatesTheIrreversibleTools pins this set from the wire
// side.
var irreversibleTools = map[string]bool{
	"delete_object":      true,
	"delete_transport":   true,
	"release_transport":  true,
	"rollback_transport": true,
	"run_class":          true,
	"update_customizing": true,
}

// ParseConsentMode converts the --consent flag value to a ConsentMode. An empty
// string selects the default, ConsentStrict. Matching is exact: a flag value
// this function does not recognise is an error rather than a silent fallback,
// because falling back would quietly hand back the permanent consent the mode
// exists to withhold.
func ParseConsentMode(s string) (ConsentMode, error) {
	switch ConsentMode(s) {
	case "":
		return ConsentStrict, nil
	case ConsentStrict:
		return ConsentStrict, nil
	case ConsentPrompt:
		return ConsentPrompt, nil
	default:
		return "", fmt.Errorf("unknown consent mode %q: expected %q or %q", s, ConsentStrict, ConsentPrompt)
	}
}

// annotate sets MetaRequiresUserInteraction on tool when the mode and the tool
// call for it. The value is the JSON boolean true; Claude Code ignores any
// other value.
//
// mcp.Tool holds Meta by pointer, so an existing one is copied rather than
// written through: AddTool receives the Tool by value, and a registration that
// ever shares a Meta between tools would otherwise find this key on all of
// them.
func (m ConsentMode) annotate(tool *mcp.Tool) {
	if m == ConsentPrompt || !irreversibleTools[tool.Name] {
		return
	}
	next := mcp.Meta{AdditionalFields: map[string]any{}}
	if tool.Meta != nil {
		next.ProgressToken = tool.Meta.ProgressToken
		for k, v := range tool.Meta.AdditionalFields {
			next.AdditionalFields[k] = v
		}
	}
	next.AdditionalFields[MetaRequiresUserInteraction] = true
	tool.Meta = &next
}
