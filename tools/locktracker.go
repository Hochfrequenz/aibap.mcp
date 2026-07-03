package tools

import (
	"strings"
	"sync"

	"github.com/Hochfrequenz/adtler/adt"
)

// sessionLockTracker records the lock-map keys created during the process's
// SAP session(s). adt.LockMap is not enumerable from outside adtler, so
// without this a reset_session could not forget the cached lock handles that a
// dropped SAP session has invalidated — the next write would reuse a stale
// handle and fail with 423 ExceptionResourceInvalidLockHandle instead of the
// old 403. See #383.
//
// Keys are system-qualified ("<system>:<uri>", produced by adt.LockKey), so a
// reset can clear exactly the active system's locks and leave other systems'
// tracked locks intact.
type sessionLockTracker struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func newSessionLockTracker() *sessionLockTracker {
	return &sessionLockTracker{keys: make(map[string]struct{})}
}

// track records that a lock exists for key. Safe to call repeatedly for the
// same key. A nil tracker is a no-op so registrations can stay ergonomic.
func (t *sessionLockTracker) track(key string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.keys[key] = struct{}{}
}

// untrack removes a single key (e.g. after an explicit unlock_object). A nil
// tracker is a no-op.
func (t *sessionLockTracker) untrack(key string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.keys, key)
}

// forgetSystem drops every tracked key belonging to systemName from both the
// tracker and the backing lock map, returning the number of entries cleared.
// A nil tracker clears nothing and returns 0.
func (t *sessionLockTracker) forgetSystem(lockMap *adt.LockMap, systemName string) int {
	if t == nil {
		return 0
	}
	prefix := systemName + ":"
	t.mu.Lock()
	defer t.mu.Unlock()
	cleared := 0
	for key := range t.keys {
		if strings.HasPrefix(key, prefix) {
			lockMap.Delete(key)
			delete(t.keys, key)
			cleared++
		}
	}
	return cleared
}
