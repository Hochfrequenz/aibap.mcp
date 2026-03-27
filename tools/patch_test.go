package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

// ---- Integration tests for patch_source MCP tool ----

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
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"e1"`}, nil
		},
		lockObjectFn: func(ctx context.Context, u string) (string, error) {
			autoLockCalled = true
			return "auto-handle", nil
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
	if state.LockHandle != "auto-handle" {
		t.Errorf("lock handle: got %q, want %q", state.LockHandle, "auto-handle")
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
			return `"e1"`, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri":  uri,
		"lock_handle": "explicit-handle",
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
	if gotLockHandle != "explicit-handle" {
		t.Errorf("SetSource lock handle: got %q, want %q", gotLockHandle, "explicit-handle")
	}
}

func TestPatchSourceToolGetSourceError(t *testing.T) {
	const uri = testObjectURI
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+uri, "handle", `"e1"`)

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
