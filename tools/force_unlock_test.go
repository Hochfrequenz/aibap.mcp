package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// force_unlock terminates the active system's SAP session (releasing every
// ENQUEUE server-side) and clears that system's cached lock handles from the
// session lock map. See #383.
func TestForceUnlockTool(t *testing.T) {
	var logoutCalled bool
	lockMap := adt.NewLockMap()
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return testLockHandle123, nil
		},
		logoutFn: func(ctx context.Context) error {
			logoutCalled = true
			return nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)

	// Populate a cached lock for the active system ("dev") via lock_object so
	// the tracker records it, plus a lock for another system directly in the
	// map that reset must NOT touch.
	if r := callTool(t, s, "lock_object", map[string]interface{}{"object_uri": testObjectURI}); r.IsError {
		t.Fatalf("lock_object failed: %v", r)
	}
	lockMap.Set("prod:"+testObjectURI, "prod-handle", "")

	result := callTool(t, s, "force_unlock", map[string]interface{}{})
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	if !logoutCalled {
		t.Error("expected client.Logout to be called")
	}

	var got struct {
		System       string `json:"system"`
		SessionReset bool   `json:"session_reset"`
		LocksCleared int    `json:"locks_cleared"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.System != "dev" {
		t.Errorf("system = %q, want %q", got.System, "dev")
	}
	if !got.SessionReset {
		t.Error("session_reset = false, want true")
	}
	if got.LocksCleared != 1 {
		t.Errorf("locks_cleared = %d, want 1", got.LocksCleared)
	}
	// Active system's cached handle is gone.
	if _, ok := lockMap.Get("dev:" + testObjectURI); ok {
		t.Error("expected active-system lock entry to be cleared")
	}
	// Another system's lock is untouched.
	if _, ok := lockMap.Get("prod:" + testObjectURI); !ok {
		t.Error("expected other-system lock entry to be retained")
	}
}

// force_unlock must not over-report LocksCleared: a tracked key whose lock the
// map no longer holds (e.g. released by activate, or an explicit-handle write
// that never stored in the map) is a no-op Delete and must not be counted.
func TestForceUnlockToolDoesNotOverCount(t *testing.T) {
	lockMap := adt.NewLockMap()
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return testLockHandle123, nil
		},
		unlockObjectFn: func(ctx context.Context, uri, lockHandle string) error { return nil },
		activateObjectsFn: func(ctx context.Context, uris []string) (*adt.ActivationResult, error) {
			return &adt.ActivationResult{}, nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)

	// Lock, then activate — activate releases the lock (deletes from the map)
	// and untracks it. force_unlock should now report 0 cleared, not 1.
	if r := callTool(t, s, "lock_object", map[string]interface{}{"object_uri": testObjectURI}); r.IsError {
		t.Fatalf("lock_object failed: %v", r)
	}
	if r := callTool(t, s, "activate_object", map[string]interface{}{"object_uri": testObjectURI}); r.IsError {
		t.Fatalf("activate_object failed: %v", r)
	}

	result := callTool(t, s, "force_unlock", map[string]interface{}{})
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	var got struct {
		LocksCleared int `json:"locks_cleared"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.LocksCleared != 0 {
		t.Errorf("locks_cleared = %d, want 0 (lock already released by activate)", got.LocksCleared)
	}
}

// When SAP session termination fails, force_unlock must report an error and
// leave the lock map alone (the enqueues are still held server-side).
func TestForceUnlockToolLogoutError(t *testing.T) {
	lockMap := adt.NewLockMap()
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return testLockHandle123, nil
		},
		logoutFn: func(ctx context.Context) error {
			return &adt.ADTError{StatusCode: 500, Message: "logoff failed"}
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)
	if r := callTool(t, s, "lock_object", map[string]interface{}{"object_uri": testObjectURI}); r.IsError {
		t.Fatalf("lock_object failed: %v", r)
	}

	result := callTool(t, s, "force_unlock", map[string]interface{}{})
	if !result.IsError {
		t.Fatal("expected IsError=true when Logout fails")
	}
	// Lock map must be preserved — SAP still holds the enqueue.
	if _, ok := lockMap.Get("dev:" + testObjectURI); !ok {
		t.Error("lock map entry must be preserved when session termination fails")
	}
}
