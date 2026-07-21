package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerLockTools(s toolAdder, client adt.LockClient, lockMap *adt.LockMap, tracker *sessionLockTracker, selector SystemSelector) {
	s.AddTool(mcp.NewTool("lock_object",
		mcp.WithTitleAnnotation("Lock Object"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Lock an ABAP object for editing. Returns a lock handle stored in the server lock map. Usually not needed — patch_source and set_source_from_file auto-lock."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithOutputSchema[LockResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		handle, err := client.LockObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		key := adt.LockKey(selector.ActiveName(), uri)
		lockMap.Set(key, handle, "")
		tracker.track(key)
		return mcp.NewToolResultJSON(LockResult{Handle: handle})
	})

	s.AddTool(mcp.NewTool("unlock_object",
		mcp.WithTitleAnnotation("Unlock Object"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Unlock a previously locked ABAP object. Validates the supplied handle against the session lock map; rejects mismatched or untracked handles without contacting SAP."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Lock handle returned by lock_object (optional, looked up automatically from the session lock map)"),
		),
		mcp.WithOutputSchema[UnlockResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		lockHandle := req.GetString("lock_handle", "")
		key := adt.LockKey(selector.ActiveName(), uri)
		// SAP's UNLOCK endpoint returns 2xx regardless of whether the lock
		// existed, the handle was valid, or the URI matched (see #383).
		// Validate against the session lock map first so we never propagate
		// that lie and never wipe a real entry on a no-op call.
		state, tracked := lockMap.Get(key)
		if !tracked {
			return errorResult(fmt.Errorf("no lock tracked for %s in this session", uri)), nil
		}
		if lockHandle == "" {
			lockHandle = state.LockHandle
		} else if lockHandle != state.LockHandle {
			return errorResult(fmt.Errorf("lock_handle does not match the handle tracked for %s in this session", uri)), nil
		}
		if err := client.UnlockObject(ctx, uri, lockHandle); err != nil {
			return errorResult(err), nil
		}
		lockMap.Delete(key)
		tracker.untrack(key)
		return mcp.NewToolResultJSON(UnlockResult{URI: uri, Unlocked: true})
	})
}

// registerForceUnlockTool registers force_unlock — the in-process recovery
// path for stuck ENQUEUE locks (#383). SAP's UNLOCK endpoint is unreliable and
// secondary auto-locks on coupled objects are invisible to the client, so the
// only recovery used to be SM12 deletion or a full server-process restart.
// Terminating the stateful SAP session releases every ENQUEUE held under it;
// adt.SystemClient.Logout does exactly that (GET /sap/public/bc/icf/logoff)
// and drops the cookie jar. The next request re-authenticates lazily.
func registerForceUnlockTool(s toolAdder, client adt.SystemClient, lockMap *adt.LockMap, tracker *sessionLockTracker, selector SystemSelector) {
	s.AddTool(mcp.NewTool("force_unlock",
		mcp.WithTitleAnnotation("Force Unlock (Reset SAP Session)"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Force-release stuck edit locks by terminating the SAP stateful session for the active system. This releases ALL ENQUEUE locks held under THIS session server-side — including secondary locks on coupled objects that unlock_object cannot reach — and clears the active system's cached lock handles. Use when a write fails with 403 \"currently editing\" or 423 and unlock_object does not help. Scope: this affects ONLY your own session's locks. It cannot touch locks held by other users, or by other sessions of the same user (a SAP GUI session, or another MCP process) — SAP ties every ENQUEUE to the session that acquired it, and this only logs off your own. The trade-off is within your session: any locks you are intentionally holding on the active system are also released. The connection re-authenticates automatically on the next call."),
		mcp.WithOutputSchema[ForceUnlockResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		active := selector.ActiveName()
		// Terminate the SAP session first. Only clear local state if the
		// server-side release actually happened — otherwise the lock map
		// would desync from SAP (the enqueues would still be held).
		if err := client.Logout(ctx); err != nil {
			return errorResult(fmt.Errorf("force_unlock: terminating SAP session for %q: %w", active, err)), nil
		}
		cleared := tracker.forgetSystem(lockMap, active)
		return mcp.NewToolResultJSON(ForceUnlockResult{
			System:       active,
			SessionReset: true,
			LocksCleared: cleared,
			Message:      fmt.Sprintf("SAP session for %q terminated; %d cached lock handle(s) cleared. The connection re-authenticates on the next call.", active, cleared),
		})
	})
}
