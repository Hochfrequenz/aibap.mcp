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

// buildDebugStepResult, buildDebugVariableResult, buildDebugStackResult, and
// buildDebugWatchpointResult wrap the non-JSON bodies returned by the ADT
// debugger endpoints in a typed struct (#501).

func buildDebugStepResult(data []byte) DebugStepResult {
	return DebugStepResult{Raw: string(data)}
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
		mcp.WithDescription("Stop the current debug run: stop the listener, remove its breakpoints, detach a halted debuggee, and drop the debug session."),
		mcp.WithString(paramUser, mcp.Description("Ignored: debug_stop always stops the current run.")),
		mcp.WithOutputSchema[DebugListenerStopResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := sessions.stop().err(); err != nil {
			return errorResult(fmt.Errorf("debug_stop: the run is stopped, but the cleanup was incomplete: %w", err)), nil
		}
		return mcp.NewToolResultJSON(DebugListenerStopResult{Stopped: true})
	})
}

func registerDebugBreakpointTools(s toolAdder, sessions *debugSessions) {
	s.AddTool(mcp.NewTool("debug_set_breakpoint",
		mcp.WithTitleAnnotation("Set Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a line breakpoint for external debugging outside a run. While a debug_run is active, pass every breakpoint to debug_run instead."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main"),
		),
		mcp.WithNumber("line", mcp.Required(), mcp.Description("Line number for the breakpoint")),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("ADT object type, e.g. PROG/P")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("ABAP object name, e.g. ZREPORT")),
		withDebugUser(),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		objectType := req.GetString("object_type", "")
		objectName := req.GetString("object_name", "")
		for _, field := range []struct{ name, value string }{
			{paramObjectURI, uri}, {"object_type", objectType}, {"object_name", objectName},
		} {
			if err := requireDebuggerStringParam("debug_set_breakpoint", field.name, field.value); err != nil {
				return errorResult(err), nil
			}
		}
		user, err := sessions.resolveUser(req.GetString(paramUser, ""))
		if err != nil {
			return errorResult(err), nil
		}
		bp, err := sessions.setBreakpointWithoutRun(ctx, user, adt.LineBreakpoint{
			ObjectURI: uri, Line: req.GetInt("line", 0), ObjectType: objectType, ObjectName: objectName,
		})
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(bp)
	})

	s.AddTool(mcp.NewTool("debug_remove_breakpoint",
		mcp.WithTitleAnnotation("Remove Breakpoint"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Remove a breakpoint by ID. Note: all breakpoints of a run are also removed when debug_stop is called."),
		mcp.WithString("breakpoint_id", mcp.Required(), mcp.Description("Breakpoint ID returned by debug_set_breakpoint")),
		withDebugUser(),
		mcp.WithOutputSchema[BreakpointRemoveResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = req.GetString("breakpoint_id", "")
		return mcp.NewToolResultJSON(BreakpointRemoveResult{
			Removed: false,
			Message: "Breakpoint removal not yet implemented — breakpoints are cleared on debug_stop",
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
		mcp.WithDescription("Execute a debug step action while debug_run's state is attached. stepContinue resumes the suspended debuggee; "+
			"terminateDebuggee kills the running debuggee process outright, while detachDebugger stops debugging and lets the debuggee "+
			"continue running to completion normally. If a step action runs the debuggee past its last statement, the result is "+
			"reported as debuggee_ended=true instead of an error — a normal, successful outcome."),
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
		if out.state.Status == runEnded && out.state.EndReason == endCompleted {
			return mcp.NewToolResultJSON(DebugStepResult{DebuggeeEnded: true})
		}
		return mcp.NewToolResultJSON(buildDebugStepResult(out.raw))
	})

	s.AddTool(mcp.NewTool("debug_get_variable",
		mcp.WithTitleAnnotation("Get Debug Variable"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read a variable value from the halted debuggee."),
		mcp.WithString("variable_name", mcp.Required(), mcp.Description("ABAP variable name, e.g. LV_RESULT")),
		withDebugUser(),
		mcp.WithOutputSchema[DebugVariableResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("variable_name", "")
		if err := requireDebuggerStringParam("debug_get_variable", "variable_name", name); err != nil {
			return errorResult(err), nil
		}
		data, errRes := sessions.call(req.GetString(paramUser, ""), func(d *adt.DebugSession) ([]byte, error) {
			return d.GetVariable(ctx, name)
		})
		if errRes != nil {
			return errRes, nil
		}
		return mcp.NewToolResultJSON(buildDebugVariableResult(name, data))
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
