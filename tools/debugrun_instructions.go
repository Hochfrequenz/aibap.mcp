package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

// okCodeProgramURI is the program whose existence tells whether a system
// knows the OK code okCodeCommand (inferred from two releases, see the spec's
// "Unknown"). The check is an ADT repository lookup, not a table read.
const (
	okCodeProgramURI = "/sap/bc/adt/programs/programs/rs_adtdbg_activate_by_okcode"
	okCodeCommand    = "/H_REACTIVATE_EXTD_DBG"
)

type okCodeAvailability int

const (
	okCodeUnknown okCodeAvailability = iota
	okCodeAvailable
	okCodeMissing
)

// okCodeCache remembers per system whether the OK code is available. Only
// definite answers are cached; a failed lookup is retried next time.
type okCodeCache struct {
	mu       sync.Mutex
	bySystem map[string]okCodeAvailability
}

func (c *okCodeCache) lookup(ctx context.Context, system string, objectInfo func(context.Context, string) error) okCodeAvailability {
	c.mu.Lock()
	if v, ok := c.bySystem[system]; ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()

	err := objectInfo(ctx, okCodeProgramURI)
	var v okCodeAvailability
	switch {
	case err == nil:
		v = okCodeAvailable
	case adt.ClassifyError(err) == adt.ErrorNotFound:
		v = okCodeMissing
	default:
		return okCodeUnknown
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bySystem == nil {
		c.bySystem = make(map[string]okCodeAvailability)
	}
	c.bySystem[system] = v
	return v
}

// manualInstructions tell someone else to start the run (trigger "manual",
// and the fallback when the server's own trigger failed).
func manualInstructions(user string, until time.Time) *DebugInstructions {
	return &DebugInstructions{
		User: user,
		Steps: []string{fmt.Sprintf(
			"Start the run now, as user %s, before %s: call the HTTP service or RFC function module, or run the program.",
			user, until.UTC().Format(time.RFC3339))},
		Notes: []string{
			"Use a new connection, or the first request of a stateful session.",
			"The run must be made as user " + user + ".",
			"Breakpoints in system programs are never hit.",
		},
	}
}

// guiInstructions tell a person or an agent how to start a SAP GUI dialog run
// that the user's external debugging applies to.
func guiInstructions(user string, t DebugTarget, ok okCodeAvailability) *DebugInstructions {
	okCode := fmt.Sprintf("enter the OK code %s KIND=USER USER=%s in the command field and press Enter", okCodeCommand, user)
	// Live on SAP_BASIS 750 a run was caught with only the transaction and
	// the IDE user filled; with the transaction alone it was not.
	tcode := fmt.Sprintf("enter /nSADT_START_TCODE in the command field, fill the transaction field (D_AIE_TCODE) with %s "+
		"and the IDE user field (D_IDE_USER) with %s (both are required: without the IDE user the run is not caught), "+
		"switch Eclipse navigation off if it is on, and press F8",
		startTransaction(t), user)
	var enable string
	switch ok {
	case okCodeAvailable:
		enable = "Enable the SAP GUI session for external debugging: " + okCode + "."
	case okCodeMissing:
		enable = "Enable the SAP GUI session for external debugging: " + tcode + "."
	default:
		enable = "Enable the SAP GUI session for external debugging: first " + okCode +
			". If the system does not know that OK code, " + tcode + " instead."
	}
	notes := []string{
		"Do every step in the same SAP GUI window: another window of the same user is not enabled for external debugging.",
		"The run must be made as user " + user + ".",
		"While the debugger is attached, the SAP GUI window stays busy; it continues after detachDebugger.",
		"Breakpoints in system programs are never hit.",
	}
	if t.Type == targetFunctionModule || t.Type == targetClassMethod {
		notes = append(notes, "The test environment converts input values to upper case unless its upper/lower case option is set.")
	}
	return &DebugInstructions{
		User:  user,
		Steps: []string{enable, startTargetStep(t, ok), "End the session with debug_step action detachDebugger, never stepContinue."},
		Notes: notes,
	}
}

// startTransaction is the D_AIE_TCODE that SADT_START_TCODE starts.
func startTransaction(t DebugTarget) string {
	switch t.Type {
	case "transaction":
		return strings.ToUpper(t.Name)
	case targetFunctionModule:
		return "SE37"
	case targetClassMethod:
		return "SE24"
	}
	return "SE38"
}

// openTransaction says how the person reaches tx: after the OK code they
// open it themselves; SADT_START_TCODE has already opened it.
func openTransaction(tx string, ok okCodeAvailability) string {
	opened := "continue in " + tx + ", which SADT_START_TCODE opened"
	switch ok {
	case okCodeAvailable:
		return "In the same window, enter /n" + tx + " in the command field"
	case okCodeMissing:
		return "In the same window, " + opened
	}
	return "In the same window, " + opened + " (after the OK code, enter /n" + tx + " in the command field instead)"
}

func startTargetStep(t DebugTarget, ok okCodeAvailability) string {
	in := formatInputs(t.Inputs)
	name := strings.ToUpper(t.Name)
	open := openTransaction(startTransaction(t), ok)
	switch t.Type {
	case "transaction":
		if in != "" {
			return open + " and enter " + in + "."
		}
		return open + "."
	case targetFunctionModule:
		s := open + ", enter function module " + name + " and press F8 (test environment)"
		if in != "" {
			s += ", enter " + in + " (a popup may open for the value)"
		}
		return s + ", then press F8."
	case targetClassMethod:
		class, method, _ := strings.Cut(name, "=>")
		s := open + ", enter class " + class + " and press F8 (test environment), then click the Execute Method icon in the line of method " + method
		if in != "" {
			s += ", enter " + in
		}
		return s + " and press F8."
	}
	s := open + ", enter program " + name + " and press F8"
	if in != "" {
		return s + "; on the selection screen enter " + in + ", then press F8."
	}
	return s + "; if a selection screen appears, fill it and press F8."
}

func formatInputs(in map[string]string) string {
	if len(in) == 0 {
		return ""
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+" = "+in[k])
	}
	return strings.Join(parts, ", ")
}
