package tools

import (
	"context"

	"github.com/Hochfrequenz/adtler/adt"
)

// releaseAutoLock releases a lock that a write handler auto-acquired during the
// current call, so a write that fails *after* locking does not leave the object
// locked (aibap.mcp#383, "no lock rollback on write failure": a failed
// set_source/patch_source otherwise leaves list_locks at 1).
//
// Call it ONLY when the lock was newly acquired by this call — never when the
// caller passed an explicit handle or a lock already existed, because the
// caller owns that lock's lifecycle. It is best-effort and same-session:
// UnlockObject on the handle we just acquired releases the primary enqueue
// server-side (a same-session dequeue with a real handle does release — see
// adtler#58), and the client-side ledger is cleared regardless.
//
// Limitation: secondary/coupled locks that SAP auto-acquires on related objects
// (e.g. a RAP BDEF's implementation class) are not reachable by this primary
// handle and are not released here — that is tracked separately (#442/#443).
// Ledger handling is conditional on the unlock outcome:
//   - UnlockObject returns an ERROR (non-2xx: network/CSRF/auth) → the lock is
//     definitely still held, so KEEP the ledger entry. Discarding it would hide
//     the lock from list_locks and throw away the only handle force_unlock or a
//     retry could act on.
//   - UnlockObject returns nil (2xx) → SAP's UNLOCK has no reliable success
//     signal (it 2xx's even for no-ops, adtler#58), so we can't be certain the
//     enqueue is gone; clear the ledger anyway rather than keep a handle a retry
//     might reuse against an already-released lock.
func releaseAutoLock(ctx context.Context, unlocker adt.LockClient, lockMap *adt.LockMap, tracker *sessionLockTracker, key, uri, lockHandle string) {
	if err := unlocker.UnlockObject(ctx, uri, lockHandle); err != nil {
		return // lock still held — keep it tracked and recoverable
	}
	lockMap.Delete(key)
	tracker.untrack(key)
}

// lockPreExisted reports whether a usable lock for key already exists before a
// write handler resolves/auto-acquires one: either the caller passed an
// explicit handle, or the session lock map already holds one. When this is
// false, the handler auto-acquires the lock and owns rolling it back on failure.
func lockPreExisted(lockMap *adt.LockMap, key, explicitHandle string) bool {
	if explicitHandle != "" {
		return true
	}
	if state, ok := lockMap.Get(key); ok && state.LockHandle != "" {
		return true
	}
	return false
}
