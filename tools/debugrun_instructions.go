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
	tcode := fmt.Sprintf("start transaction SADT_START_TCODE with D_AIE_TCODE = %s, D_IDE_USER = %s and D_REQUEST_USER = %s, with Eclipse navigation switched off",
		startTransaction(t), user, user)
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
		"Breakpoints in system programs are never hit.",
	}
	if t.Type != "report" {
		notes = append(notes, "Untested: starting a "+strings.ReplaceAll(t.Type, "_", " ")+" this way has not been verified on a live system yet.")
	}
	return &DebugInstructions{
		User:  user,
		Steps: []string{enable, startTargetStep(t), "End the session with debug_step action detachDebugger, never stepContinue."},
		Notes: notes,
	}
}

// startTransaction is the D_AIE_TCODE that SADT_START_TCODE starts.
func startTransaction(t DebugTarget) string {
	switch t.Type {
	case "transaction":
		return strings.ToUpper(t.Name)
	case "function_module":
		return "SE37"
	case targetClassMethod:
		return "SE24"
	}
	return "SE38"
}

func startTargetStep(t DebugTarget) string {
	in := formatInputs(t.Inputs)
	name := strings.ToUpper(t.Name)
	switch t.Type {
	case "transaction":
		s := fmt.Sprintf("In the same window, start transaction %s (enter /n%s; with SADT_START_TCODE it starts directly from D_AIE_TCODE)", name, name)
		if in != "" {
			s += " and enter " + in
		}
		return s + "."
	case "function_module":
		s := fmt.Sprintf("In the same window, open SE37, enter function module %s and press F8 (test environment)", name)
		if in != "" {
			s += ", enter " + in
		}
		return s + ", then press F8."
	case targetClassMethod:
		class, method, _ := strings.Cut(name, "=>")
		s := fmt.Sprintf("In the same window, open SE24, enter class %s and press F8 (test environment), choose method %s", class, method)
		if in != "" {
			s += ", enter " + in
		}
		return s + ", then execute."
	}
	s := fmt.Sprintf("In the same window, open SE38, enter program %s and press F8", name)
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
