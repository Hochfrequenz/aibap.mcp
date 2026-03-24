package adt

import "sync"

// LockState holds the lock handle and ETag for a locked object.
type LockState struct {
	LockHandle string
	ETag       string
}

// LockMap is a thread-safe map tracking active locks per system:objectURI.
type LockMap struct {
	mu    sync.RWMutex
	locks map[string]LockState
}

// NewLockMap creates a new empty LockMap.
func NewLockMap() *LockMap {
	return &LockMap{locks: make(map[string]LockState)}
}

// Set stores or overwrites a lock entry.
func (m *LockMap) Set(key, lockHandle, etag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locks[key] = LockState{LockHandle: lockHandle, ETag: etag}
}

// Get retrieves a lock entry. Returns false if not found.
func (m *LockMap) Get(key string) (LockState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.locks[key]
	return s, ok
}

// UpdateETag updates only the ETag for an existing entry. No-op if key is missing.
func (m *LockMap) UpdateETag(key, etag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.locks[key]; ok {
		s.ETag = etag
		m.locks[key] = s
	}
}

// Delete removes a lock entry.
func (m *LockMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, key)
}
