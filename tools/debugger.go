package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// validDebugStepActions is the single source of truth for debug_step's
// action enum: the mcp.Enum() declaration and the handler's own validation
// both derive from it, so they can't drift apart. The handler validates
// independently of the declared JSON Schema because this server does not
// call mcp-go's server.WithInputSchemaValidation.
var validDebugStepActions = []string{"stepInto", "stepOver", "stepReturn", "stepContinue", "terminateDebuggee", "detachDebugger"}

var validDebugStepActionSet = func() map[string]bool {
	m := make(map[string]bool, len(validDebugStepActions))
	for _, a := range validDebugStepActions {
		m[a] = true
	}
	return m
}()

const (
	defaultWaitSeconds = 45
	maxWaitSeconds     = 300
)

func requireDebuggerStringParam(toolName, paramName, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %q is required", toolName, paramName)
	}
	return nil
}

// buildDebugSessionsResult converts the raw GetDebuggeeSessions response into a
// typed result. The body is SAP ASX XML (not JSON), and it is empty when there
// are no active debuggee sessions (#433).
func buildDebugSessionsResult(data []byte) DebugSessionsResult {
	if len(bytes.TrimSpace(data)) == 0 {
		return DebugSessionsResult{HasSessions: false}
	}
	return DebugSessionsResult{HasSessions: true, Raw: string(data)}
}

// buildDebugVariableResult, buildDebugStackResult, and
// buildDebugWatchpointResult wrap the non-JSON bodies returned by the ADT
// debugger endpoints in a typed struct (#501).

// stepResultFromState shapes the run state after a step as debug_step's result.
func stepResultFromState(st DebugRunState) DebugStepResult {
	return DebugStepResult{
		Version:       st.Version,
		Status:        st.Status,
		EndReason:     st.EndReason,
		DebuggeeEnded: st.Status == runEnded && st.EndReason == endCompleted,
		Position:      st.Position,
		Hint:          st.Hint,
	}
}

// stepResultForError reports whether err is (or wraps) *adt.DebuggeeEndedError:
// the step ran the debuggee to completion, a success (#529).
func stepResultForError(err error) (DebugStepResult, bool) {
	var ended *adt.DebuggeeEndedError
	if errors.As(err, &ended) {
		return DebugStepResult{DebuggeeEnded: true}, true
	}
	return DebugStepResult{}, false
}

func buildDebugVariableResult(name string, data []byte) DebugVariableResult {
	return DebugVariableResult{VariableName: name, Value: string(data)}
}

func buildDebugStackResult(data []byte) DebugStackResult {
	return DebugStackResult{Raw: string(data)}
}

func buildDebugWatchpointResult(data []byte) DebugWatchpointResult {
	return DebugWatchpointResult{Raw: string(data)}
}

// withDebugUser declares the optional user argument the debug tools take.
func withDebugUser() mcp.ToolOption {
	return mcp.WithString(paramUser,
		mcp.Description("SAP user whose run is debugged. Default: the logon user configured for the active system; required when that system has none (OAuth2)."),
	)
}

const debugRunDescription = "Debug one ABAP run. Sets all breakpoints in one request, listens in the background for a run of user, " +
	"and attaches to the first run that hits a breakpoint. Returns the run state at once (manual, gui), or after up to 15 s " +
	"when the server starts the run itself (unit_tests, or gui in a build that can drive SAP GUI). " +
	"Then call debug_wait with since_version = the returned version until status is attached; inspect with debug_get_stack, " +
	"debug_get_variable and debug_step; end the debuggee with debug_step detachDebugger (not stepContinue); call debug_stop when done. " +
	"When the state carries instructions, follow them: the run must be made as user before listening_until. " +
	"Breakpoints in system programs are never hit. A new debug_run first stops the previous run."

const debugWaitDescription = "Wait for the debug run started by debug_run to change. With since_version (the version you saw last) " +
	"it returns as soon as the run's version is greater, or after timeout_seconds (default 45) with the unchanged state; " +
	"without since_version it returns the current state at once. Cancelling or timing out debug_wait does not stop the run. " +
	"rearm: true starts the next listening window of a manual or gui run after status timeout or ended, within the run's remaining budget; a unit_tests run needs a new debug_run."

func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector, fallback BlackMagicClient, settings registerSettings) {
	// The debug tools share one session and one run; debugSessions decides
	// when they are created, reused or replaced (#562, #558).
	sessions := newDebugSessions(client, selector, settings.systemUser)
	// A build may start gui runs itself (#558); checked by type assertion so
	// existing BlackMagicClient implementations keep compiling.
	if t, ok := fallback.(DebugTriggerer); ok {
		sessions.triggerer = t
	}
	if settings.shutdown != nil {
		settings.shutdown.add(sessions.shutdown)
	}
	registerDebugRunTools(s, sessions)
	registerDebugBreakpointTools(s, sessions)
	registerDebugInspectTools(s, sessions)
}

func registerDebugRunTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_run",
		mcp.WithTitleAnnotation("Debug a Run"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(debugRunDescription),
		mcp.WithArray("breakpoints",
			mcp.Required(),
			// No minItems/maxItems/minimum: some clients reject those keywords
			// (TestToolSchemasMatchOpenCodeProfile); parseDebugRunArgs enforces them.
			mcp.Description(fmt.Sprintf("1 to %d line breakpoints, all set in one request: [{object_uri, line}]. object_uri is a source URI, e.g. "+
				"/sap/bc/adt/programs/programs/zreport/source/main or /sap/bc/adt/oo/classes/zcl_x/includes/testclasses", maxRunBreakpoints)),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					paramObjectURI: map[string]any{"type": "string", "description": "Source URI (…/source/main or …/includes/…)"},
					"line":         map[string]any{"type": "integer", "description": "Source line, 1 or greater"},
				},
				"required": []string{paramObjectURI, "line"},
			}),
		),
		withDebugUser(),
		mcp.WithObject("trigger",
			mcp.Required(),
			mcp.Description("How the run starts. kind unit_tests: the server runs the ABAP Unit tests of object_uri "+
				"(default: the object of the first breakpoint) as the logon user. kind manual: someone else starts the run "+
				"(an HTTP or RFC call, a program); follow instructions. kind gui: a SAP GUI dialog run of target; follow "+
				"instructions unless this server build starts it itself."),
			mcp.Properties(map[string]any{
				"kind":         map[string]any{"type": "string", "enum": []string{triggerUnitTests, triggerManual, triggerGUI}},
				paramObjectURI: map[string]any{"type": "string", "description": "unit_tests only: object whose tests run"},
				"target": map[string]any{
					"type":        "object",
					"description": "gui only: what the dialog run starts",
					"properties": map[string]any{
						"type":   map[string]any{"type": "string", "enum": validDebugTargetTypes},
						"name":   map[string]any{"type": "string", "description": "program, transaction code, function module, or CLASS=>METHOD"},
						"inputs": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "selection-screen or parameter values"},
					},
					"required": []string{"type", "name"},
				},
			}),
		),
		mcp.WithNumber("timeout_seconds", mcp.Description("Listener budget of the run in seconds (default 300, max 600)")),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseDebugRunArgs(req.GetArguments())
		if err != nil {
			return errorResult(err), nil
		}
		st, err := sessions.startRun(ctx, args)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(st)
	})

	s.AddTool(mcp.NewTool("debug_wait",
		mcp.WithTitleAnnotation("Wait for the Debug Run"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(debugWaitDescription),
		mcp.WithNumber("since_version", mcp.Description("The version of the last run state you saw; debug_wait returns once the run has a newer one")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Longest wait in seconds (default 45, max 300)")),
		mcp.WithBoolean("rearm", mcp.Description("Listen again (manual and gui runs, after timeout or ended)")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		run, err := sessions.currentRun(req.GetString(paramUser, ""))
		if err != nil {
			return errorResult(err), nil
		}
		if req.GetBool("rearm", false) {
			if err := sessions.rearm(run); err != nil {
				return errorResult(err), nil
			}
		}
		args := req.GetArguments()
		timeout := defaultWaitSeconds
		if v, ok := args["timeout_seconds"]; ok && v != nil {
			n, ok := intArg(v)
			if !ok || n < 1 || n > maxWaitSeconds {
				return errorResult(fmt.Errorf("debug_wait: \"timeout_seconds\" must be a whole number from 1 to %d", maxWaitSeconds)), nil
			}
			timeout = n
		}
		var st DebugRunState
		var fatal error
		if v, ok := args["since_version"]; ok && v != nil {
			since, ok := intArg(v)
			if !ok {
				return errorResult(errors.New("debug_wait: \"since_version\" must be a whole number (the version of a run state)")), nil
			}
			st, fatal = run.waitUntil(ctx, time.Duration(timeout)*time.Second, func(s DebugRunState) bool { return s.Version > int64(since) })
		} else {
			st, fatal = run.snapshot()
		}
		if fatal != nil {
			return errorResult(fatal), nil
		}
		return mcp.NewToolResultJSON(st)
	})

	s.AddTool(mcp.NewTool("debug_stop",
		mcp.WithTitleAnnotation("Stop Debugging"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Stop the current debug run: stop the listener, remove its breakpoints, detach a halted debuggee, and drop "+
			"the debug session. Reports the removed breakpoints and anything that could not be removed or detached."),
		mcp.WithString(paramUser, mcp.Description("Ignored: debug_stop always stops the current run.")),
		mcp.WithOutputSchema[DebugStopResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultJSON(buildStopResult(sessions.stop()))
	})
}

func registerDebugBreakpointTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_set_breakpoint",
		mcp.WithTitleAnnotation("Set Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Add a line breakpoint to the active debug run and return the run state. While the debuggee is halted "+
			"(status attached) the breakpoint is set in the attached debugger. Otherwise every external breakpoint of the run is set "+
			"again together with the new one, in one request. Fails without an active run (start one with debug_run) and while an "+
			"attach is in progress (call debug_wait)."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("Source URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main or …/includes/…"),
		),
		mcp.WithNumber("line", mcp.Required(), mcp.Description("Source line")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugRunState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := strings.TrimSpace(req.GetString(paramObjectURI, ""))
		if _, ok := sourceObjectURI(uri); !ok {
			return errorResult(fmt.Errorf("debug_set_breakpoint: object_uri %q is not a source URI (…/source/main or …/includes/…)", uri)), nil
		}
		line, ok := intArg(req.GetArguments()["line"])
		if !ok || line < 1 {
			return errorResult(errors.New("debug_set_breakpoint: line must be a whole number of at least 1")), nil
		}
		typ, name := deriveBreakpointObject(uri)
		st, err := sessions.addBreakpoint(ctx, req.GetString(paramUser, ""), adt.LineBreakpoint{ObjectURI: uri, Line: line, ObjectType: typ, ObjectName: name})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(st)
	})

	s.AddTool(mcp.NewTool("debug_remove_breakpoint",
		mcp.WithTitleAnnotation("Remove Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Remove one breakpoint of the active debug run by the id in its run state, in the scope it was set in. "+
			"A debugger-scope breakpoint can only be removed while the debuggee is attached. "+
			"An external breakpoint cannot be removed while the debuggee is attached or attaching (on SAP_BASIS 750 the request detaches the debugger): remove it after detachDebugger. "+
			"debug_stop removes all of them."),
		mcp.WithString("breakpoint_id", mcp.Required(), mcp.Description("Breakpoint id from the run state's breakpoints")),
		withDebugUser(),
		mcp.WithOutputSchema[BreakpointRemoveResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := strings.TrimSpace(req.GetString("breakpoint_id", ""))
		if err := requireDebuggerStringParam("debug_remove_breakpoint", "breakpoint_id", id); err != nil {
			return errorResult(err), nil
		}
		bp, err := sessions.removeBreakpoint(ctx, req.GetString(paramUser, ""), id)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(BreakpointRemoveResult{
			Removed: true,
			Message: fmt.Sprintf("removed %s (%s line %d, scope %s)", bp.ID, bp.ObjectURI, bp.Line, bp.Scope),
		})
	})
}

func registerDebugInspectTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_get_sessions",
		mcp.WithTitleAnnotation("Get Debug Sessions"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("List active debuggee sessions. Works in any state of a debug run."),
		withDebugUser(),
		mcp.WithOutputSchema[DebugSessionsResult](),
	), debugUserOnlyHandler(sessions.callSession, (*adt.DebugSession).GetDebuggeeSessions, buildDebugSessionsResult))

	s.AddTool(mcp.NewTool("debug_step",
		mcp.WithTitleAnnotation("Debug Step"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Execute a debug step action while debug_run's state is attached, and return the run's status and the new position. "+
			"stepInto, stepOver, stepReturn and stepContinue (to a next breakpoint) halt again: status stays attached with the new position. "+
			"End a session with detachDebugger: the debuggee runs on to its end and the run ends with end_reason detached; "+
			"terminateDebuggee kills it (end_reason terminated). Use detachDebugger, never stepContinue, to finish: past the end of a run "+
			"stepContinue answers SAP_BASIS 750 with AdiFailed / CX_TPDAPI_DEBUGGEE_ENDED and SAP_BASIS 816 with 400 ExceptionInvalidData; "+
			"both are reported as debuggee_ended=true (end_reason completed) when no debuggee remains, and after a SAP GUI trigger "+
			"stepContinue can hang. A failed step leaves the debuggee attached: detach it, or debug_stop."),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Step action: stepInto, stepOver, stepReturn, stepContinue, terminateDebuggee, or detachDebugger"),
			mcp.Enum(validDebugStepActions...),
		),
		withDebugUser(),
		mcp.WithOutputSchema[DebugStepResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := req.GetString("action", "")
		if !validDebugStepActionSet[action] {
			return errorResult(fmt.Errorf(
				"debug_step: unrecognised action %q — must be one of: %s",
				action, strings.Join(validDebugStepActions, ", "),
			)), nil
		}
		out, err := sessions.step(ctx, req.GetString(paramUser, ""), action)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(stepResultFromState(out.state))
	})

	s.AddTool(mcp.NewTool("debug_get_variable",
		mcp.WithTitleAnnotation("Get Debug Variable"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read a variable of the halted debuggee (debug_run status attached). This is for debugging a halted program, "+
			"not for retrieving data. Scalars return their value. expand: true returns the components of a structure, the attributes "+
			"of an object reference, or the dereferenced value of a data reference (REF->*). offset/limit return rows of an internal "+
			"table: offset is 1-based (default 1), limit defaults to 20, with a hard cap of 100 rows per call."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name, e.g. LV_RESULT, LS_ROW-TEXT, LO_OBJ->ATTR")),
		mcp.WithBoolean("expand", mcp.Description("Return the children of a structure, object reference or data reference")),
		mcp.WithNumber("offset", mcp.Description("Internal tables: first row, 1-based (default 1)")),
		mcp.WithNumber("limit", mcp.Description("Internal tables: number of rows (default 20, max 100)")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugVariableResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q, err := parseVariableQuery(req.GetArguments())
		if err != nil {
			return errorResult(err), nil
		}
		var res DebugVariableResult
		_, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			var err error
			res, err = readVariable(ctx, d, q)
			return nil, err
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(res)
	})

	s.AddTool(mcp.NewTool("debug_get_stack",
		mcp.WithTitleAnnotation("Get Debug Stack"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Get the call stack of the halted debuggee."),
		withDebugUser(),
		mcp.WithOutputSchema[DebugStackResult](),
	), debugUserOnlyHandler(sessions.call, (*adt.DebugSession).GetStack, buildDebugStackResult))

	s.AddTool(mcp.NewTool("debug_set_watchpoint",
		mcp.WithTitleAnnotation("Set Watchpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a watchpoint on a variable of the halted debuggee to break when its value changes."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name to watch")),
		mcp.WithString("condition", mcp.Description("Optional condition expression")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugWatchpointResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		variableName := req.GetString("variable_name", "")
		condition := req.GetString("condition", "")
		if err := requireDebuggerStringParam("debug_set_watchpoint", "variable_name", variableName); err != nil {
			return errorResult(err), nil
		}
		data, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return d.SetWatchpoint(ctx, variableName, condition)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(buildDebugWatchpointResult(data))
	})
}
