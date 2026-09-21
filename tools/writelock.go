package tools

import (
	"context"
	"fmt"

	"github.com/Hochfrequenz/adtler/adt"
)

// writeLockClient is the subset of adt.Client needed to defend a write
// against #377: ECC's ADT write handlers accept any lock_handle silently —
// a bogus or drifted handle succeeds with no enqueue held and no error
// raised — so a cached handle can never be trusted there the way it can on
// S/4 (which returns 423 on a mismatch).
type writeLockClient interface {
	adt.LockClient
	adt.SystemClient
}

// resolveWriteLockHandle resolves the lock handle a write handler should use.
//
// explicitHandle always wins, on every system flavor — an operator-supplied
// handle is the caller's own responsibility to manage, not this function's.
//
// Otherwise, on ECC the cached handle is never trusted: every write forces a
// fresh LockObject call, stores the result in lockMap, and uses that handle —
// regardless of whether a cache entry already existed. On S/4, or when the
// flavor probe itself errors (fail open: the probe is defense-in-depth on top
// of S/4's own 423 validation, not the only safety net, so a transient
// failure here shouldn't block every write), the existing cache-reuse path
// applies. allowAutoLock controls that fallback path's behavior on a cache
// miss: true auto-acquires a lock (patch_source, set_source_from_file — both
// already did this before #377); false treats a cache miss as a caller error
// (set_include_source, which has always required an explicit prior
// lock_object call).
//
// autoLocked reports whether this call acquired a previously untracked lock.
// releaseOnFailure reports whether this call acquired the concrete SAP lock
// handle the write is about to use, so callers should drop it again if a later
// step fails. On ECC that includes forced relocks even when a stale/tracked
// cache entry already existed.
func resolveWriteLockHandle(ctx context.Context, client writeLockClient, lockMap *adt.LockMap, tracker *sessionLockTracker, key, uri, explicitHandle string, allowAutoLock bool) (handle string, autoLocked bool, releaseOnFailure bool, err error) {
	if explicitHandle != "" {
		return explicitHandle, false, false, nil
	}

	if flavor, ferr := client.SystemFlavor(ctx); ferr == nil && flavor == adt.SystemFlavorECC {
		autoLocked = !lockPreExisted(lockMap, key, "")
		h, err := client.LockObject(ctx, uri)
		if err != nil {
			return "", false, false, fmt.Errorf("auto-lock %s: %w", uri, err)
		}
		// Only the lock handle is untrustworthy on ECC (#377) — preserve any
		// cached ETag rather than wiping it and forcing a needless GetSource.
		prev, _ := lockMap.Get(key)
		lockMap.Set(key, h, prev.ETag)
		tracker.track(key)
		return h, autoLocked, true, nil
	}

	if !allowAutoLock {
		state, tracked := lockMap.Get(key)
		if !tracked || state.LockHandle == "" {
			return "", false, false, fmt.Errorf("no lock tracked for %s in this session — call lock_object first", uri)
		}
		return state.LockHandle, false, false, nil
	}

	autoLocked = !lockPreExisted(lockMap, key, explicitHandle)
	h, err := lockMap.ResolveLock(ctx, client, key, uri, explicitHandle)
	if err != nil {
		return "", false, false, err
	}
	tracker.track(key)
	return h, autoLocked, autoLocked, nil
}
