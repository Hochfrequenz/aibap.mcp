package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
)

// ---- Unit tests for ApplyPatchOps ----

func TestApplyOpsInsert(t *testing.T) {
	source := "line1\nline2\nline3"
	ops := []tools.PatchOp{
		{Type: "insert", AfterLine: 1, Content: "inserted"},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\ninserted\nline2\nline3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsInsertAtZero(t *testing.T) {
	source := "line1\nline2"
	ops := []tools.PatchOp{
		{Type: "insert", AfterLine: 0, Content: "before"},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "before\nline1\nline2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsReplace(t *testing.T) {
	source := "line1\nline2\nline3\nline4"
	ops := []tools.PatchOp{
		{Type: "replace", FromLine: 2, ToLine: 3, Content: "new"},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nnew\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsDelete(t *testing.T) {
	source := "line1\nline2\nline3\nline4"
	ops := []tools.PatchOp{
		{Type: "delete", FromLine: 2, ToLine: 3},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsSearchReplace(t *testing.T) {
	source := "REPORT ZTEST.\nDATA: lv_x TYPE i."
	ops := []tools.PatchOp{
		{Type: "search_replace", Search: "ZTEST", Replace: "ZNEW"},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "REPORT ZNEW.\nDATA: lv_x TYPE i."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsSearchReplaceAll(t *testing.T) {
	source := "foo bar foo baz foo"
	ops := []tools.PatchOp{
		{Type: "search_replace", Search: "foo", Replace: "qux", All: true},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "qux bar qux baz qux"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsMultiple(t *testing.T) {
	// Two inserts: ops should be sorted descending by after_line so bottom-up application works.
	source := "line1\nline2\nline3"
	ops := []tools.PatchOp{
		{Type: "insert", AfterLine: 1, Content: "after1"},
		{Type: "insert", AfterLine: 2, Content: "after2"},
	}
	got, err := tools.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After sorting desc (after_line 2 first, then 1):
	// Apply after_line=2: line1, line2, after2, line3
	// Apply after_line=1: line1, after1, line2, after2, line3
	want := "line1\nafter1\nline2\nafter2\nline3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsOverlapRejected(t *testing.T) {
	source := "line1\nline2\nline3\nline4"
	ops := []tools.PatchOp{
		{Type: "replace", FromLine: 1, ToLine: 3, Content: "new1"},
		{Type: "replace", FromLine: 2, ToLine: 4, Content: "new2"},
	}
	_, err := tools.ApplyPatchOps(source, ops)
	if err == nil {
		t.Fatal("expected error for overlapping ops")
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("error should mention overlap, got: %v", err)
	}
}

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
			return `"etag-new"`, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": uri,
		"operations": []interface{}{
			map[string]interface{}{
				"type":      "search_replace",
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
	if state.ETag != `"etag-new"` {
		t.Errorf("lock map ETag: got %q, want %q", state.ETag, `"etag-new"`)
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
				"type":      "search_replace",
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
				"type":      "search_replace",
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
				"type":      "search_replace",
				"search":  "x",
				"replace": "y",
			},
		},
	})

	if !result.IsError {
		t.Fatal("expected IsError=true when GetSource fails")
	}
}
