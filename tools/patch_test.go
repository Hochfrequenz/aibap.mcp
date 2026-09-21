package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
)

// ---- Integration tests for patch_source MCP tool ----

const testETagE1 = `"e1"`

func TestPatchSourceToolSearchReplace(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+uri, "handle-abc", `"etag-old"`)

	var gotSource string
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.\nDATA: lv_x TYPE i.", ETag: `"etag-old"`}, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			gotSource = source
			return testETagNew, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{
				"type":    "search_replace",
				"search":  "ZTEST",
				"replace": "ZNEW",
			},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}

	if !strings.Contains(gotSource, "ZNEW") {
		t.Errorf("expected patched source to contain ZNEW, got: %q", gotSource)
	}

	// Lock map ETag should be updated.
	state, ok := lockMap.Get("dev:" + uri)
	if !ok {
		t.Fatal("lock map entry should still exist")
	}
	if state.ETag != testETagNew {
		t.Errorf("lock map ETag: got %q, want %q", state.ETag, testETagNew)
	}
}

func TestPatchSourceToolAutoLock(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap() // empty — no pre-existing lock

	var autoLockCalled bool
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: testETagE1}, nil
		},
		lockObjectFn: func(ctx context.Context, u string) (string, error) {
			autoLockCalled = true
			return testAutoHandle, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			return `"e2"`, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{
				"type":    "search_replace",
				"search":  "ZTEST",
				"replace": "ZAUTO",
			},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if !autoLockCalled {
		t.Error("expected auto-lock to be called when lock map is empty")
	}

	// Lock map should be populated after auto-lock.
	state, ok := lockMap.Get("dev:" + uri)
	if !ok {
		t.Fatal("expected lock map entry after auto-lock")
	}
	if state.LockHandle != testAutoHandle {
		t.Errorf("lock handle: got %q, want %q", state.LockHandle, testAutoHandle)
	}

	// Response should indicate locked=true.
	text := firstText(result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\ntext: %q", err, text)
	}
	if locked, _ := resp["locked"].(bool); !locked {
		t.Errorf("expected locked=true in response, got: %v", resp["locked"])
	}
}

func TestPatchSourceToolExplicitLockHandle(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	// Pre-populate with a different handle.
	lockMap.Set("dev:"+uri, "map-handle", `"e0"`)

	var gotLockHandle string
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"e0"`}, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			gotLockHandle = lockHandle
			return testETagE1, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri":  uri,
		"lock_handle": testExplicitHandle,
		"operations": []interface{}{
			map[string]interface{}{
				"type":    "search_replace",
				"search":  "ZTEST",
				"replace": "ZEXPLICIT",
			},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if gotLockHandle != testExplicitHandle {
		t.Errorf("SetSource lock handle: got %q, want %q", gotLockHandle, testExplicitHandle)
	}
}

// TestPatchSourceToolECCForcesFreshLock guards #377: ECC's ADT write handlers
// accept any lock_handle silently (no validation, no enqueue held), so a
// stale cached handle could drift undetected. On ECC, patch_source must
// ignore the cache and force a fresh LockObject call before every write.
func TestPatchSourceToolECCForcesFreshLock(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+uri, "stale-cached-handle", `"e0"`)

	var lockObjectCalls int
	var gotLockHandle string
	mock := &mockClient{
		systemFlavorFn: func(ctx context.Context) (adt.SystemFlavor, error) {
			return adt.SystemFlavorECC, nil
		},
		lockObjectFn: func(ctx context.Context, u string) (string, error) {
			lockObjectCalls++
			return testECCFreshHandle, nil
		},
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"e0"`}, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			gotLockHandle = lockHandle
			return testETagE1, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{"type": "search_replace", "search": "ZTEST", "replace": "ZECC"},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if lockObjectCalls != 1 {
		t.Errorf("expected LockObject to be called once on ECC despite a cached handle, got %d calls", lockObjectCalls)
	}
	if gotLockHandle != testECCFreshHandle {
		t.Errorf("SetSource lock handle: got %q, want the freshly-acquired handle, not the stale cached one", gotLockHandle)
	}
	var out tools.PatchSourceResult
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Locked {
		t.Errorf("result locked=%v, want false when ECC refreshed an already-tracked lock", out.Locked)
	}
	state, ok := lockMap.Get("dev:" + uri)
	if !ok || state.LockHandle != testECCFreshHandle {
		t.Errorf("lock map should be refreshed with the new handle, got %+v", state)
	}
}

// TestPatchSourceToolECCExplicitHandleSkipsRelock ensures an operator-supplied
// lock_handle is still honored as-is on ECC — the #377 defense only distrusts
// the *cache*, never an explicit caller instruction.
func TestPatchSourceToolECCExplicitHandleSkipsRelock(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()

	var lockObjectCalls int
	var gotLockHandle string
	mock := &mockClient{
		systemFlavorFn: func(ctx context.Context) (adt.SystemFlavor, error) {
			return adt.SystemFlavorECC, nil
		},
		lockObjectFn: func(ctx context.Context, u string) (string, error) {
			lockObjectCalls++
			return "should-not-be-used", nil
		},
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"e0"`}, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			gotLockHandle = lockHandle
			return testETagE1, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri":  uri,
		"lock_handle": testExplicitHandle,
		"operations": []interface{}{
			map[string]interface{}{"type": "search_replace", "search": "ZTEST", "replace": "ZEXPLICIT"},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if lockObjectCalls != 0 {
		t.Errorf("expected LockObject NOT to be called when an explicit handle is supplied, got %d calls", lockObjectCalls)
	}
	if gotLockHandle != testExplicitHandle {
		t.Errorf("SetSource lock handle: got %q, want explicit handle %q", gotLockHandle, testExplicitHandle)
	}
}

// TestPatchSourceToolFlavorProbeErrorFallsBackToCachedLock pins the intended
// fail-open path in resolveWriteLockHandle: if the flavor probe itself fails,
// write tools still follow the normal cache-reuse path instead of blocking the
// write outright.
func TestPatchSourceToolFlavorProbeErrorFallsBackToCachedLock(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+uri, "cached-handle", `"e0"`)

	var lockObjectCalls int
	var gotLockHandle string
	mock := &mockClient{
		systemFlavorFn: func(ctx context.Context) (adt.SystemFlavor, error) {
			return adt.SystemFlavorUnknown, fmt.Errorf("probe failed")
		},
		lockObjectFn: func(ctx context.Context, u string) (string, error) {
			lockObjectCalls++
			return "should-not-be-used", nil
		},
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"e0"`}, nil
		},
		setSourceFn: func(ctx context.Context, u, source, lockHandle, transport, etag string) (string, error) {
			gotLockHandle = lockHandle
			return testETagE1, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{"type": "search_replace", "search": "ZTEST", "replace": "ZFALLBACK"},
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if lockObjectCalls != 0 {
		t.Errorf("expected LockObject not to be called when SystemFlavor fails open, got %d calls", lockObjectCalls)
	}
	if gotLockHandle != "cached-handle" {
		t.Errorf("SetSource lock handle: got %q, want cached handle during fail-open fallback", gotLockHandle)
	}
	var out tools.PatchSourceResult
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Locked {
		t.Errorf("result locked=%v, want false when fail-open reused the cached lock", out.Locked)
	}
}

func TestPatchSourceToolGetSourceError(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+uri, "handle", testETagE1)

	mock := &mockClient{
		getSourceFn: func(ctx context.Context, u string) (*adt.SourceResult, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{
				"type":    "search_replace",
				"search":  "x",
				"replace": "y",
			},
		},
	})

	if !result.IsError {
		t.Fatal("expected IsError=true when GetSource fails")
	}
}
