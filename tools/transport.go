package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTransportTools(s toolAdder, client adt.TransportClient, fallback BlackMagicClient) {
	s.AddTool(mcp.NewTool("get_transport_requests",
		mcp.WithTitleAnnotation("Get Transport Requests"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"List one page of the top-level CTS transport requests on the configured SAP system, most recently "+
				"changed first. Each request carries a state (modifiable, release_in_progress, released or unknown) "+
				"and SAP's raw TRSTATUS letter. Page through the list by passing the returned next cursor as after. "+
				"Reads E070 through the data preview, so the user needs data preview authorization.",
		),
		mcp.WithString("user", mcp.Description(
			"Owner of the requests (E070-AS4USER), e.g. MMUSTERMANN. Omit for the logged-on user; pass * for every user's requests. "+
				"Requests in which the user only owns a task are not listed.")),
		mcp.WithString("status", mcp.Enum("all", "modifiable", "released"), mcp.Description(
			"Filter by state: all (default), modifiable or released. A request whose release is in progress, "+
				"or whose status is unknown, appears under all only.")),
		mcp.WithNumber("limit", mcp.Description(fmt.Sprintf(
			"Page size, default %d, at most %d.", adt.DefaultTransportPageSize, adt.MaxTransportPageSize))),
		mcp.WithString("after", mcp.Description(
			"The next cursor of the previous page; omit for the first page. Valid only with the user and status that produced it.")),
		mcp.WithOutputSchema[TransportRequestsResult](),
	), getTransportRequestsHandler(client))

	s.AddTool(mcp.NewTool("add_to_transport",
		mcp.WithTitleAnnotation("Add to Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Record an object entry (an E071 key such as R3TR PROG ZFOO or LIMU FUNC Z_FOO) in a modifiable "+
				"CTS transport request or task; SAP locks the entry for that transport, as SE09 does. "+
				"Use get_transport_requests to find a transport. "+
				"Requires the transport organizer's add-object action, which SAP_BASIS 7.50 (ECC) does not offer: "+
				"there, and for a released request, the call is refused without reaching SAP. Where the system lacks "+
				"the action, the entry goes through the configured fallback if it supports transport entries "+
				"(via_fallback=true); otherwise record the object in SE09 via the sapgui.mcp server instead.",
		),
		mcp.WithString("pgmid", mcp.Description("Program ID of the E071 key: R3TR (default) or LIMU")),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("Object type of the E071 key, e.g. PROG, CLAS, TABL, FUNC")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("Object name of the E071 key, e.g. ZFOO; sent as given")),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request or task number, e.g. DEVK900123")),
		mcp.WithOutputSchema[AddToTransportResult](),
	), addToTransportHandler(client, fallback))

	s.AddTool(mcp.NewTool("create_transport_task",
		mcp.WithTitleAnnotation("Create Transport Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Create a task (Aufgabe) under an existing transport request for the current user. "+
				"Use this when you need your own task to add new objects to a shared transport. "+
				"Note: for deleting or modifying objects already locked in a transport, you do NOT need to create a task first — "+
				"pass the parent transport number directly to delete_object or set_source and SAP records the change automatically.",
		),
		mcp.WithString("parent_transport", mcp.Required(), mcp.Description("Parent transport request number, e.g. DEVK900123")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Short description for the task")),
		mcp.WithString("owner", mcp.Description("SAP username for the task owner. Defaults to the authenticated user if omitted. Use this to create tasks for other team members.")),
		mcp.WithOutputSchema[CreateTransportTaskResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parent := req.GetString("parent_transport", "")
		desc := req.GetString("description", "")
		owner := req.GetString("owner", "")
		taskNumber, err := client.CreateTransportTask(ctx, parent, owner, desc)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(CreateTransportTaskResult{
			TaskNumber:      taskNumber,
			ParentTransport: parent,
			Description:     desc,
		})
	})

	s.AddTool(mcp.NewTool("create_transport",
		mcp.WithTitleAnnotation("Create Transport Request"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Create a new CTS transport request and return its number. Categories: "+
				"K = workbench request: package required; SAP takes the target from the package's transport layer, so target is refused. "+
				"W = customizing request and T = transport of copies: package refused; target optional "+
				"(empty: a customizing request gets the client's default target, a transport of copies none). "+
				"SAP_BASIS 7.50 (ECC) cannot create W or T over ADT; there the request goes through the "+
				"configured fallback (via_fallback=true), or, without one, the call fails and the request has to be "+
				"created in SE09 via the sapgui.mcp server. "+
				"To list transport targets: SELECT SYSNAME FROM TCESYST WHERE VERSION = '0002'.",
		),
		mcp.WithString("category", mcp.Required(), mcp.Enum("K", "W", "T"), mcp.Description("Request type: K (workbench), W (customizing) or T (transport of copies)")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Short description for the transport (at most 60 characters)")),
		mcp.WithString("target", mcp.Description("Transport target for W or T, e.g. DUM, a virtual system or <SID>.<client>. Refused for K.")),
		mcp.WithString("package", mcp.Description("Package for K (required there). Refused for W and T.")),
		mcp.WithOutputSchema[CreateTransportResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cat, errRes := requireString(req, "category")
		if errRes != nil {
			return errRes, nil
		}
		desc, errRes := requireString(req, "description")
		if errRes != nil {
			return errRes, nil
		}
		opts := adt.CreateTransportOptions{
			Type:        adt.TransportType(strings.ToUpper(strings.TrimSpace(cat))),
			Description: desc,
			Package:     req.GetString("package", ""),
			Target:      strings.ToUpper(strings.TrimSpace(req.GetString("target", ""))),
		}
		switch opts.Type {
		case adt.TransportTypeWorkbench, adt.TransportTypeCustomizing, adt.TransportTypeCopies:
		default:
			return errorResult(fmt.Errorf("unknown category %q: use K (workbench), W (customizing) or T (transport of copies)", cat)), nil
		}

		// adtler refuses inputs SAP would ignore (package for W/T, target
		// for K) and answers ErrorNotSupported, without sending anything,
		// where the system cannot create W/T over ADT (ECC, adtler#137).
		// Only that case goes to the fallback; an error SAP returned is
		// reported as is.
		number, err := client.CreateTransport(ctx, opts)
		if err != nil && adt.ClassifyError(err) == adt.ErrorNotSupported {
			if fallback == nil {
				return errorResult(fmt.Errorf("%w — no fallback is configured: create the request in SE09 via the sapgui.mcp server", err)), nil
			}
			number, fbErr := fallback.CreateTransportFallback(ctx, string(opts.Type), opts.Target, desc, opts.Package)
			if fbErr == nil && number == "" {
				fbErr = errors.New("the fallback returned no request number")
			}
			if fbErr != nil {
				return errorResult(fmt.Errorf("ADT cannot create this request here and the fallback failed: %w (ADT: %v)", fbErr, err)), nil
			}
			return mcp.NewToolResultJSON(CreateTransportResult{TransportNumber: number, Description: desc, ViaFallback: true})
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(CreateTransportResult{
			TransportNumber: number,
			Description:     desc,
		})
	})

	s.AddTool(mcp.NewTool("release_transport",
		mcp.WithTitleAnnotation("Release Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Release a transport request or task. Released transports are queued for import "+
				"into the target system and cannot be modified afterwards. "+
				"All tasks must be released before the parent request — if include_tasks is true, "+
				"tasks are released automatically first. "+
				"NOTE: On ECC systems, release via ADT may silently fail (returns 200 but status stays modifiable). "+
				"This tool detects the silent fail via a follow-up status check; when a fallback is "+
				"configured it routes the release through that path automatically. Without a fallback, "+
				"use the sap-desktop MCP server to release via SE09 instead. "+
				"The result reports the state read back after the release: released, or — with released=false — "+
				"release_in_progress (SAP is still releasing; re-read later, do not release again) or unknown "+
				"(the status read failed or returned an unknown letter; check before releasing again).",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request or task number to release")),
		mcp.WithBoolean("include_tasks", mcp.Description("If true, automatically release all tasks before releasing the request (default: false)")),
		mcp.WithOutputSchema[ReleaseTransportResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport, errRes := requireString(req, "transport")
		if errRes != nil {
			return errRes, nil
		}
		includeTasks := req.GetBool("include_tasks", false)

		// ReleaseTransport/ReleaseTransportWithTasks release and confirm via a
		// post-release status read: ADT (notably on ECC) can return 200 while
		// leaving the transport modifiable. State=Modifiable with a nil error
		// is that "silent failure", and only that state warrants the fallback:
		// ReleaseInProgress and Unknown mean the release may well have gone
		// through, so releasing again would be wrong (adtler#273, #275).
		// (adtler v0.5.5 folded the old ReleaseTransportVerified wrapper
		// directly into these two methods — see adtler#171 — so this branches
		// on include_tasks instead of passing it as a parameter.)
		var res *adt.ReleaseResult
		var releaseErr error
		if includeTasks {
			res, releaseErr = client.ReleaseTransportWithTasks(ctx, transport)
		} else {
			res, releaseErr = client.ReleaseTransport(ctx, transport)
		}
		if releaseErr == nil && res == nil {
			res = &adt.ReleaseResult{Transport: transport}
		}
		silentFail := releaseErr == nil && res.State == adt.TransportStateModifiable

		if (releaseErr != nil || silentFail) && fallback != nil {
			if fbErr := fallback.ReleaseTransportFallback(ctx, transport); fbErr != nil {
				if releaseErr != nil {
					return errorResult(fmt.Errorf("ADT release failed and fallback also failed: %w (ADT: %v)", fbErr, releaseErr)), nil
				}
				return errorResult(fmt.Errorf("ADT returned 200 but transport %s stayed modifiable; fallback also failed: %w", transport, fbErr)), nil
			}
			return mcp.NewToolResultJSON(ReleaseTransportResult{Transport: transport, Released: true, ViaFallback: true})
		}

		if releaseErr != nil {
			return errorResult(releaseErr), nil
		}
		if silentFail {
			return errorResult(fmt.Errorf("transport %s released via ADT (returned 200) but status remained modifiable; configure a fallback or release via SE09 in the sap-desktop MCP", transport)), nil
		}
		return mcp.NewToolResultJSON(releaseTransportResultOf(transport, res))
	})

	s.AddTool(mcp.NewTool("delete_transport",
		mcp.WithTitleAnnotation("Delete Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Delete a transport request or task. Works for both requests and tasks. "+
				"Two preconditions: the transport must be modifiable (not released), and it must "+
				"hold no object entries — SAP rejects one that still has them with "+
				"ADT_TM_COMMON_EXCEPTION \"... contains locked objects\". Note that deleting a "+
				"repository object does not remove its transport entry: to empty a transport, "+
				"remove every entry get_transport_objects reports using remove_from_transport. The current "+
				"get_transport_objects result does not expose the owning task number required for that call. "+
				"Deleting a request also deletes its tasks, provided they are empty.",
		),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request or task number to delete")),
		mcp.WithOutputSchema[DeleteTransportResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport, errRes := requireString(req, "transport")
		if errRes != nil {
			return errRes, nil
		}
		if err := client.DeleteTransport(ctx, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(DeleteTransportResult{Transport: transport, Deleted: true})
	})

	s.AddTool(mcp.NewTool("remove_from_transport",
		mcp.WithTitleAnnotation("Remove from Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Remove an object entry from a transport task. Use get_transport_objects to find the "+
				"pgmid, type, name, wb_type, and position of the object to remove. "+
				"The task_number is the task that holds the object (use get_transport_objects on the parent "+
				"transport to find which task owns it). The parent_transport is the request number. "+
				"Requires AS ABAP 7.53 SP00 (ABAP Platform 1809) or later: older systems, including "+
				"ECC on SAP_BASIS 7.50, have no remove-object operation in ADT at all, and this tool "+
				"cannot remove it over ADT there. Where the system is detected as lacking the action, the entry is removed through the configured "+
				"fallback if it supports transport entries (via_fallback=true); otherwise remove it in SE09 via the "+
				"sapgui.mcp server instead.",
		),
		mcp.WithString("task_number", mcp.Required(), mcp.Description("Task number that holds the object, e.g. DEVK900124")),
		mcp.WithString("parent_transport", mcp.Required(), mcp.Description("Parent transport request number, e.g. DEVK900123")),
		mcp.WithString("pgmid", mcp.Required(), mcp.Description("Program ID, typically R3TR")),
		mcp.WithString("object_type", mcp.Required(), mcp.Description("Object type, e.g. PROG, CLAS, TABL")),
		mcp.WithString("object_name", mcp.Required(), mcp.Description("Object name, e.g. Z_MY_PROGRAM")),
		mcp.WithString("wb_type", mcp.Required(), mcp.Description("Workbench type, e.g. PROG/P, CLAS/OC")),
		mcp.WithString("position", mcp.Required(), mcp.Description(
			"Object position in transport (from get_transport_objects), e.g. 000001. Required, and must be "+
				"current: on systems that support removal, SAP's own handler selects the entry to delete by "+
				"pgmid/type/name/position alone — it does not also check which transport the entry belongs to. "+
				"A wrong or stale position can match and remove an unrelated entry in a different request, or "+
				"match nothing and report success while removing nothing. Always take the value from a "+
				"get_transport_objects call made just before this one, not a cached one.",
		)),
		mcp.WithOutputSchema[RemoveFromTransportResult](),
	), removeFromTransportHandler(client, fallback))

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
		mcp.WithOutputSchema[TransportObjectsResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		transport, errRes := requireString(req, "transport")
		if errRes != nil {
			return errRes, nil
		}
		objects, err := client.GetTransportObjects(ctx, transport)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(TransportObjectsResult{
			Transport: transport,
			Count:     len(objects),
			Objects:   objects,
		})
	})
}

// transportRequestsResultOf wraps an adtler page into the tool result. The
// transports slice is never nil, so the wire shape is always an array.
func transportRequestsResultOf(page *adt.TransportRequestPage) TransportRequestsResult {
	res := TransportRequestsResult{Transports: []TransportRequestInfo{}}
	if page != nil {
		for _, r := range page.Requests {
			res.Transports = append(res.Transports, TransportRequestInfo{
				Number:      r.Number,
				Owner:       r.Owner,
				Description: r.Description,
				State:       r.State.String(),
				RawStatus:   r.RawStatus,
			})
		}
		res.Next = page.Next
	}
	res.Count = len(res.Transports)
	return res
}

// releaseTransportResultOf reports the state adtler read back after a release
// that was neither an error nor a silent failure: released, release in
// progress, or unknown.
func releaseTransportResultOf(transport string, res *adt.ReleaseResult) ReleaseTransportResult {
	return ReleaseTransportResult{
		Transport: transport,
		Released:  res.State == adt.TransportStateReleased,
		State:     res.State.String(),
		RawStatus: res.RawStatus,
	}
}

func getTransportRequestsHandler(client adt.TransportClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opts := adt.TransportListOptions{
			User:  req.GetString("user", ""),
			Limit: req.GetInt("limit", 0),
			After: req.GetString("after", ""),
		}
		if status := req.GetString("status", ""); status != "" {
			if err := opts.Filter.UnmarshalText([]byte(strings.ToLower(status))); err != nil {
				return errorResult(err), nil
			}
		}
		page, err := client.GetTransportRequests(ctx, opts)
		partial := err != nil && errors.Is(err, adt.ErrTransportListPartial) && page != nil
		if err != nil && !partial {
			return errorResult(err), nil
		}
		res := transportRequestsResultOf(page)
		if partial {
			res.Warning = err.Error()
		}
		return mcp.NewToolResultJSON(res)
	}
}

func addToTransportHandler(client adt.TransportClient, fallback BlackMagicClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		objType, errRes := requireString(req, "object_type")
		if errRes != nil {
			return errRes, nil
		}
		objName, errRes := requireString(req, "object_name")
		if errRes != nil {
			return errRes, nil
		}
		transport, errRes := requireString(req, "transport")
		if errRes != nil {
			return errRes, nil
		}
		pgmid := strings.ToUpper(req.GetString("pgmid", "R3TR"))
		objType = strings.ToUpper(objType)
		res := AddToTransportResult{PgmID: pgmid, ObjectType: objType, ObjectName: objName, Transport: transport, Added: true}
		err := client.AddToTransport(ctx, pgmid, objType, objName, transport)
		if fb, ok := entryFallbackFor(err, fallback); ok {
			if fbErr := fb.AddToTransportFallback(ctx, pgmid, objType, objName, transport); fbErr != nil {
				return errorResult(fmt.Errorf("ADT cannot record this entry here and the fallback failed: %w (ADT: %v)", fbErr, err)), nil
			}
			res.ViaFallback = true
			return mcp.NewToolResultJSON(res)
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	}
}

func removeFromTransportHandler(client adt.TransportClient, fallback BlackMagicClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskNr := req.GetString("task_number", "")
		parentTr := req.GetString("parent_transport", "")
		pgmid := req.GetString("pgmid", "")
		objType := req.GetString("object_type", "")
		objName := req.GetString("object_name", "")
		wbType := req.GetString("wb_type", "")
		position := req.GetString("position", "")
		res := RemoveFromTransportResult{TaskNumber: taskNr, ObjectName: objName, Removed: true}
		err := client.RemoveFromTransport(ctx, taskNr, parentTr, pgmid, objType, objName, wbType, position)
		if fb, ok := entryFallbackFor(err, fallback); ok {
			if fbErr := fb.RemoveFromTransportFallback(ctx, taskNr, parentTr, pgmid, objType, objName); fbErr != nil {
				return errorResult(fmt.Errorf("ADT cannot remove this entry here and the fallback failed: %w (ADT: %v)", fbErr, err)), nil
			}
			res.ViaFallback = true
			return mcp.NewToolResultJSON(res)
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	}
}

// entryFallbackFor returns the configured TransportEntryFallback when err is
// adtler's ErrorNotSupported, the only case the fallback is for: an error
// SAP returned is reported as is (#616).
func entryFallbackFor(err error, fallback BlackMagicClient) (TransportEntryFallback, bool) {
	if err == nil || adt.ClassifyError(err) != adt.ErrorNotSupported {
		return nil, false
	}
	fb, ok := fallback.(TransportEntryFallback)
	return fb, ok
}
