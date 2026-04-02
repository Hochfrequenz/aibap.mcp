package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerDebuggerTools(s toolAdder, client adt.Client, selector SystemSelector) {
	// Shared debug session — created lazily on first use.
	var dbg *adt.DebugSession

	getSession := func(user string) *adt.DebugSession {
		if dbg == nil {
			dbg = adt.NewDebugSession(client, user)
		}
		return dbg
	}

	s.AddTool(mcp.NewTool("debug_set_breakpoint",
		mcp.WithTitleAnnotation("Set Breakpoint"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a line breakpoint in an ABAP object for external debugging."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number for the breakpoint"),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("ADT object type, e.g. PROG/P"),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("ABAP object name, e.g. ZREPORT"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		line := req.GetInt("line", 0)
		objectType := req.GetString("object_type", "")
		objectName := req.GetString("object_name", "")
		user := req.GetString("user", "")

		bp, err := getSession(user).SetBreakpoint(ctx, uri, line, objectType, objectName)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(bp)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("debug_remove_breakpoint",
		mcp.WithTitleAnnotation("Remove Breakpoint"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Not yet implemented. Breakpoints are cleared automatically when debug_stop is called."),
		mcp.WithString("breakpoint_id",
			mcp.Required(),
			mcp.Description("Breakpoint ID returned by debug_set_breakpoint"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = req.GetString("breakpoint_id", "")
		// TODO: implement RemoveBreakpoint in DebugSession
		return mcp.NewToolResultText("Breakpoint removal not yet implemented — breakpoints are cleared on debug_stop"), nil
	})

	s.AddTool(mcp.NewTool("debug_start",
		mcp.WithTitleAnnotation("Start Debug Session"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a breakpoint and start the debug listener. Blocks until the breakpoint is hit or timeout expires. To trigger the breakpoint, run unit tests (run_unit_tests) in a separate call. External breakpoints only trigger in HTTP/ICF sessions, not SAP GUI."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/zreport/source/main"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number for the breakpoint"),
		),
		mcp.WithString("object_type",
			mcp.Required(),
			mcp.Description("ADT object type, e.g. PROG/P"),
		),
		mcp.WithString("object_name",
			mcp.Required(),
			mcp.Description("ABAP object name, e.g. ZREPORT"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Listener timeout in seconds (default: 60)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		line := req.GetInt("line", 0)
		objectType := req.GetString("object_type", "")
		objectName := req.GetString("object_name", "")
		user := req.GetString("user", "")
		timeout := req.GetInt("timeout_seconds", 60)

		session := getSession(user)

		bp, err := session.SetBreakpoint(ctx, uri, line, objectType, objectName)
		if err != nil {
			return errorResult(err), nil
		}
		if bp.ErrorMessage != "" {
			return errorResult(fmt.Errorf("breakpoint error: %s", bp.ErrorMessage)), nil
		}

		result, err := session.StartListener(ctx, timeout)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"breakpoint_id": bp.ID,
			"status":        result.Status,
			"debuggee_id":   result.DebuggeeID,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("debug_stop",
		mcp.WithTitleAnnotation("Stop Debug Session"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Stop the debug listener and clean up all breakpoints."),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		if err := getSession(user).StopListener(ctx); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Debug listener stopped and breakpoints cleared"), nil
	})

	s.AddTool(mcp.NewTool("debug_get_sessions",
		mcp.WithTitleAnnotation("Get Debug Sessions"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("List active debuggee sessions."),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		data, err := getSession(user).GetDebuggeeSessions(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("debug_attach",
		mcp.WithTitleAnnotation("Attach to Debug Session"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Attach to an active debuggee session."),
		mcp.WithString("debuggee_id",
			mcp.Required(),
			mcp.Description("Debuggee session ID from debug_get_sessions"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		debuggeeID := req.GetString("debuggee_id", "")
		user := req.GetString("user", "")
		if err := getSession(user).Attach(ctx, debuggeeID); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Attached to debuggee %s", debuggeeID)), nil
	})

	s.AddTool(mcp.NewTool("debug_step",
		mcp.WithTitleAnnotation("Debug Step"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Execute a debug step action (stepInto, stepOver, stepReturn, or continue). Requires an active debug session via debug_start + debug_attach."),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Step action: stepInto, stepOver, stepReturn, or continue"),
			mcp.Enum("stepInto", "stepOver", "stepReturn", "continue"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action := req.GetString("action", "")
		user := req.GetString("user", "")
		data, err := getSession(user).Step(ctx, action)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("debug_get_variable",
		mcp.WithTitleAnnotation("Get Debug Variable"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Read a variable value from the active debug session."),
		mcp.WithString("variable_name",
			mcp.Required(),
			mcp.Description("ABAP variable name, e.g. LV_RESULT"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("variable_name", "")
		user := req.GetString("user", "")
		data, err := getSession(user).GetVariable(ctx, name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("debug_get_stack",
		mcp.WithTitleAnnotation("Get Debug Stack"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Get the current call stack from the active debug session."),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		data, err := getSession(user).GetStack(ctx)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	s.AddTool(mcp.NewTool("debug_set_watchpoint",
		mcp.WithTitleAnnotation("Set Watchpoint"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Set a watchpoint on a variable to break when its value changes."),
		mcp.WithString("variable_name",
			mcp.Required(),
			mcp.Description("ABAP variable name to watch"),
		),
		mcp.WithString("condition",
			mcp.Description("Optional condition expression"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("SAP username for the debug session"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		variableName := req.GetString("variable_name", "")
		condition := req.GetString("condition", "")
		user := req.GetString("user", "")
		data, err := getSession(user).SetWatchpoint(ctx, variableName, condition)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})
}
