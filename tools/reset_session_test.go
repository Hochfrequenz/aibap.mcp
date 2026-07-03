package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// reset_session terminates the active system's SAP session (releasing every
// ENQUEUE server-side) and clears that system's cached lock handles from the
// session lock map. See #383.
func TestResetSessionTool(t *testing.T) {
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

	result := callTool(t, s, "reset_session", map[string]interface{}{})
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

// When SAP session termination fails, reset_session must report an error and
// leave the lock map alone (the enqueues are still held server-side).
func TestResetSessionToolLogoutError(t *testing.T) {
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

	result := callTool(t, s, "reset_session", map[string]interface{}{})
	if !result.IsError {
		t.Fatal("expected IsError=true when Logout fails")
	}
	// Lock map must be preserved — SAP still holds the enqueue.
	if _, ok := lockMap.Get("dev:" + testObjectURI); !ok {
		t.Error("lock map entry must be preserved when session termination fails")
	}
}
