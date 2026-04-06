package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerTransportTools(s toolAdder, client adt.TransportClient, fallback BlackMagicClient) {
	s.AddTool(mcp.NewTool("get_transport_requests",
		mcp.WithTitleAnnotation("Get Transport Requests"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("List CTS transport requests on the configured SAP system. Status: D=modifiable, L=released."),
		mcp.WithString("user", mcp.Description("Filter by owner username")),
		mcp.WithString("status", mcp.Description("Filter by status: D (modifiable) or L (released)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := req.GetString("user", "")
		status := req.GetString("status", "")
		transports, err := client.GetTransportRequests(ctx, user, status)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(transports)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("add_to_transport",
		mcp.WithTitleAnnotation("Add to Transport"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Record an ABAP object into a CTS transport task. The transport parameter should be a task number (not the parent transport). Use get_transport_requests to find available transports."),
		mcp.WithString(paramObjectURI, mcp.Required(), mcp.Description(descADTObjectURI)),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number, e.g. DEVK900123")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		transport := req.GetString("transport", "")
		if err := client.AddToTransport(ctx, uri, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object added to transport successfully"), nil
	})

	s.AddTool(mcp.NewTool("create_transport_task",
		mcp.WithTitleAnnotation("Create Transport Task"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Create a task (Aufgabe) under an existing transport request for the current user. "+
				"Use this when you need your own task to add new objects to a shared transport. "+
				"Note: for deleting or modifying objects already locked in a transport, you do NOT need to create a task first — "+
				"pass the parent transport number directly to delete_object or set_source and SAP records the change automatically.",
		),
		mcp.WithString("parent_transport", mcp.Required(), mcp.Description("Parent transport request number, e.g. S4UK902339")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Short description for the task")),
		mcp.WithString("owner", mcp.Description("SAP username for the task owner. Defaults to the authenticated user if omitted. Use this to create tasks for other team members.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parent := req.GetString("parent_transport", "")
		desc := req.GetString("description", "")
		owner := req.GetString("owner", "")
		taskNumber, err := client.CreateTransportTask(ctx, parent, owner, desc)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"task_number":      taskNumber,
			"parent_transport": parent,
			"description":      desc,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("create_transport",
		mcp.WithTitleAnnotation("Create Transport Request"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Create a new CTS transport request. Returns the transport number. "+
				"Categories: K=workbench (development objects), W=customizing, T=transport of copies, "+
				"C=relocation without package change, O=relocation with package change, E=relocation of complete package. "+
				"Package and target are optional — omit them to create an unassigned request. "+
				"To find the correct target, query: SELECT SYSNAME, TRANSLAYER FROM TCESYST WHERE VERSION = '0002'. "+
				"To find a package's transport layer: SELECT DEVCLASS, PDEVCLASS FROM TDEVC WHERE DEVCLASS = 'Z_MY_PKG'.",
		),
		mcp.WithString("category", mcp.Required(), mcp.Description("Transport category: K (workbench), W (customizing), T (transport of copies)")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Short description for the transport")),
		mcp.WithString("target", mcp.Description("Target system (e.g. DUM, PRD). Query TCESYST to find available targets.")),
		mcp.WithString("package", mcp.Description("Development class / package name. Optional — omit for unassigned requests.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cat := req.GetString("category", "")
		desc := req.GetString("description", "")
		target := req.GetString("target", "")
		pkg := req.GetString("package", "")
		number, err := client.CreateTransport(ctx, cat, target, desc, pkg)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"transport_number": number,
			"description":      desc,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("release_transport",
		mcp.WithTitleAnnotation("Release Transport"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Release a transport request or task. Released transports are queued for import "+
				"into the target system and cannot be modified afterwards. "+
				"All tasks must be released before the parent request — if include_tasks is true, "+
				"tasks are released automatically first. "+
				"NOTE: On ECC systems, release via ADT may silently fail (returns 200 but status stays modifiable). "+
				"If release fails on ECC, use the sap-desktop MCP server to release via SE09 instead.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request or task number to release")),
		mcp.WithBoolean("include_tasks", mcp.Description("If true, automatically release all tasks before releasing the request (default: false)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		includeTasks := req.GetBool("include_tasks", false)
		var err error
		if includeTasks {
			err = client.ReleaseTransportWithTasks(ctx, transport)
		} else {
			err = client.ReleaseTransport(ctx, transport)
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Transport " + transport + " released"), nil
	})

	s.AddTool(mcp.NewTool("delete_transport",
		mcp.WithTitleAnnotation("Delete Transport"),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Delete a transport request or task. Works for both requests and tasks. "+
				"The transport must be modifiable (not released). "+
				"Deleting a request with tasks deletes all tasks too.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request or task number to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		if err := client.DeleteTransport(ctx, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Transport " + transport + " deleted"), nil
	})

	s.AddTool(mcp.NewTool("get_transport_objects",
		mcp.WithTitleAnnotation("Get Transport Objects"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"List all objects recorded in a transport request (deduplicated across request and tasks). "+
				"Use this to see what a transport contains before releasing or rolling back.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport := req.GetString("transport", "")
		objects, err := client.GetTransportObjects(ctx, transport)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(objects)
		return mcp.NewToolResultText(string(out)), nil
	})
}
