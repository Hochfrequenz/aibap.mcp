package tools

import "context"

// DebugTarget names what a SAP GUI dialog run starts (debug_run trigger.kind
// "gui").
type DebugTarget struct {
	// Type is report, transaction, function_module or class_method.
	Type string `json:"type"`
	// Name is the program, transaction code, function module or CLASS=>METHOD.
	Name string `json:"name"`
	// Inputs are free-form values for the selection screen or parameters.
	Inputs map[string]string `json:"inputs,omitempty"`
}

// DebugTriggerer starts a run so that the external breakpoints of user are hit.
// Optional: checked by type assertion on the BlackMagicClient, so existing
// implementations keep compiling. Called only for trigger.kind "gui", after the
// listener is running, with a context that the run's cleanup does not cancel.
type DebugTriggerer interface {
	TriggerDebugRun(ctx context.Context, system, user string, t DebugTarget) error
}

const targetClassMethod = "class_method"

var validDebugTargetTypes = []string{"report", "transaction", "function_module", targetClassMethod}

func validDebugTargetType(s string) bool {
	for _, v := range validDebugTargetTypes {
		if s == v {
			return true
		}
	}
	return false
}
