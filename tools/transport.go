package tools

import (
	"context"
	"encoding/json"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerTransportTools(s toolAdder, client adt.TransportClient) {
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
			"Create a task (Aufgabe) under an existing transport request. "+
				"Use this when you need to record changes under a shared transport, e.g. to delete or modify objects "+
				"locked in another user's transport. The task is created for the currently authenticated SAP user.",
		),
		mcp.WithString("parent_transport", mcp.Required(), mcp.Description("Parent transport request number, e.g. S4UK902339")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Short description for the task")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parent := req.GetString("parent_transport", "")
		desc := req.GetString("description", "")
		taskNumber, err := client.CreateTransportTask(ctx, parent, desc)
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
}
