package tools_test

import (
	"testing"
)

// debugStepToolName is shared across this file and debugger_confirmation_test.go.
const debugStepToolName = "debug_step"

// TestDebugStepActionEnum pins issue #501 Bug 2's actual tool-surface change
// (the only user-visible change in that fix): "continue" — the ADT method
// name that never worked, rejected with ExceptionInvalidData: Unknown
// method — must not appear in debug_step's action enum, and its
// replacement plus the two additional release actions must.
//
// This asserts against the declared input schema only; it does not invoke
// the debug_step handler (which panics on the mockClient used by
// TestStructuredContentIsObject — see that test's knownOptOuts comment for
// why). Schema listing itself does not touch adtler, so it's safe.
func TestDebugStepActionEnum(t *testing.T) {
	s := newTestServer(&mockClient{})
	tools := listRegisteredTools(t, s)

	var step *listedTool
	for i := range tools {
		if tools[i].Name == debugStepToolName {
			step = &tools[i]
			break
		}
	}
	if step == nil {
		t.Fatal("debug_step not registered")
	}

	props, _ := step.InputSchema["properties"].(map[string]any)
	actionSchema, _ := props["action"].(map[string]any)
	enumVals, _ := actionSchema["enum"].([]any)
	if len(enumVals) == 0 {
		t.Fatalf("debug_step action has no enum: %+v", actionSchema)
	}

	got := make(map[string]bool, len(enumVals))
	for _, v := range enumVals {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("debug_step action enum value is not a string: %#v", v)
		}
		got[s] = true
	}

	if got["continue"] {
		t.Error(`debug_step action enum still contains "continue" — ADT rejects it with ExceptionInvalidData: Unknown method; the real resume action is "stepContinue" (issue #501)`)
	}
	for _, want := range []string{"stepInto", "stepOver", "stepReturn", "stepContinue", "terminateDebuggee", "detachDebugger"} {
		if !got[want] {
			t.Errorf("debug_step action enum missing %q; got %v", want, enumVals)
		}
	}
}

// TestDebugToolsDeclareOutputSchema pins that the four handlers fixed by
// issue #501 (previously wrapping non-JSON ADT bodies in json.RawMessage
// and passing them to NewToolResultJSON) each declare a typed
// WithOutputSchema. The old behavior wasn't a uniform failure mode: XML/
// most text bodies aren't valid JSON at all and failed json.Marshal
// outright, but a text/plain variable value that happened to look like a
// bare JSON scalar (e.g. "42", "true") would marshal successfully and
// produce a scalar structuredContent instead — the #351 bug class, not a
// marshal error; see issue #501's own "single digit" / "structuredContent
// expected record" symptom. Either way the typed structs fix it. This test
// does not — and cannot, without a live SAP debuggee — assert that the
// handler actually returns a value matching the declared schema; see
// tools/debugger_test.go for the builder-level coverage of that, and the
// 2026-09-18 live verification linked from issue #501's PR for the real
// end-to-end proof.
func TestDebugToolsDeclareOutputSchema(t *testing.T) {
	s := newTestServer(&mockClient{})
	tools := listRegisteredTools(t, s)

	byName := make(map[string]listedTool, len(tools))
	for _, tl := range tools {
		byName[tl.Name] = tl
	}

	for _, name := range []string{debugStepToolName, "debug_get_variable", "debug_get_stack", "debug_set_watchpoint"} {
		tl, ok := byName[name]
		if !ok {
			t.Errorf("%s not registered", name)
			continue
		}
		if len(tl.OutputSchema) == 0 {
			t.Errorf("%s does not declare an outputSchema", name)
		}
	}
}
