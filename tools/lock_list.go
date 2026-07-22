package tools

import (
	"context"
	"sort"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// collectTrackedLocks builds the honest client-side lock ledger: every lock this
// session acquired AND still holds in the lock map, across all systems. Tracked
// keys the map no longer holds (released by unlock_object/activate, or never
// stored because the caller passed an explicit handle) are excluded — the same
// honesty rule force_unlock uses. Sorted by system-qualified key for determinism.
//
// This is a client-side ledger only: it reflects locks THIS process acquired,
// never a system-wide SM12 view. SAP exposes no ADT endpoint to read live
// enqueues (adtler#58), so this is the best-effort answer to "what do I hold".
func collectTrackedLocks(tracker *sessionLockTracker, lockMap *adt.LockMap) LockListResult {
	keys := tracker.snapshot()
	sort.Strings(keys)
	locks := make([]LockInfo, 0, len(keys))
	for _, key := range keys {
		state, held := lockMap.Get(key)
		if !held {
			continue
		}
		system, uri, _ := strings.Cut(key, ":")
		locks = append(locks, LockInfo{
			System:     system,
			URI:        uri,
			LockHandle: state.LockHandle,
			ETag:       state.ETag,
		})
	}
	return LockListResult{Count: len(locks), Locks: locks}
}

// registerListLocksTool registers list_locks — a read-only view of the locks
// this MCP session currently holds. See #383: SAP has no ADT enqueue-read
// endpoint, so a client-side ledger is the only way to answer "what do I hold".
func registerListLocksTool(s toolAdder, lockMap *adt.LockMap, tracker *sessionLockTracker) {
	s.AddTool(mcp.NewTool("list_locks",
		mcp.WithTitleAnnotation("List Held Locks"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription("List the ABAP object locks this MCP session currently holds (client-side ledger, across all systems). Reflects only locks acquired by THIS server process — NOT a system-wide SM12 view. SAP exposes no ADT endpoint to read live enqueues, so locks held by other users, other sessions of the same user, or a SAP GUI are invisible here. Use it to see what force_unlock or unlock_object would release."),
		mcp.WithOutputSchema[LockListResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultJSON(collectTrackedLocks(tracker, lockMap))
	})
}
