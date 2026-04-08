package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestSetSourceFromFileTool(t *testing.T) {
	const fileContent = "REPORT ZTEST.\nWRITE 'Hello'."
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ztest.abap")
	if err := os.WriteFile(filePath, []byte(fileContent), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "pre-lock-handle", `"etag-pre"`)

	var gotURI, gotSource, gotHandle, gotTransport, gotETag string
	mock := &mockClient{
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			gotURI, gotSource, gotHandle, gotTransport, gotETag = uri, source, lockHandle, transport, etag
			return testETagAfter, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  filePath,
		"transport":  "DEVK900001",
	})
	if result.IsError {
		t.Fatalf("unexpected error result: %s", firstText(result))
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q, want %q", gotURI, testObjectURI)
	}
	if gotSource != fileContent {
		t.Errorf("source: got %q, want %q", gotSource, fileContent)
	}
	if gotHandle != "pre-lock-handle" {
		t.Errorf("lock_handle: got %q, want %q", gotHandle, "pre-lock-handle")
	}
	if gotTransport != "DEVK900001" {
		t.Errorf("transport: got %q, want %q", gotTransport, "DEVK900001")
	}
	if gotETag != `"etag-pre"` {
		t.Errorf("etag: got %q, want %q", gotETag, `"etag-pre"`)
	}

	// Response JSON should have success=true and correct line count.
	text := firstText(result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response JSON: %v\ntext: %q", err, text)
	}
	if resp["success"] != true {
		t.Errorf("success: got %v", resp["success"])
	}
	if resp["lines"] != float64(2) {
		t.Errorf("lines: got %v, want 2", resp["lines"])
	}
	if resp["lock_handle"] != "pre-lock-handle" {
		t.Errorf("lock_handle in response: got %v", resp["lock_handle"])
	}
	if resp["etag"] != testETagAfter {
		t.Errorf("etag in response: got %v", resp["etag"])
	}

	// Lock map ETag should be updated.
	state, ok := lockMap.Get("dev:" + testObjectURI)
	if !ok {
		t.Fatal("expected lock map entry to exist after set_source_from_file")
	}
	if state.ETag != testETagAfter {
		t.Errorf("lock map ETag: got %q, want %q", state.ETag, testETagAfter)
	}
}

func TestSetSourceFromFileToolMissingFile(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  "/nonexistent/path/ztest.abap",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true for missing file")
	}
}

func TestSetSourceFromFileToolAutoLock(t *testing.T) {
	const fileContent = "REPORT ZTEST."
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ztest.abap")
	if err := os.WriteFile(filePath, []byte(fileContent), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	lockMap := adt.NewLockMap() // empty — no pre-existing lock

	var lockCalled bool
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			lockCalled = true
			return "auto-lock-handle", nil
		},
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "", ETag: `"etag-fresh"`}, nil
		},
		setSourceFn: func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
			return testETagNew, nil
		},
	}

	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": testObjectURI,
		"file_path":  filePath,
	})
	if result.IsError {
		t.Fatalf("unexpected error result: %s", firstText(result))
	}
	if !lockCalled {
		t.Error("expected LockObject to be called for auto-lock")
	}

	// Lock map should now contain an entry for this object.
	state, ok := lockMap.Get("dev:" + testObjectURI)
	if !ok {
		t.Fatal("expected lock map entry to be populated after auto-lock")
	}
	if state.LockHandle != "auto-lock-handle" {
		t.Errorf("lock_handle in map: got %q, want %q", state.LockHandle, "auto-lock-handle")
	}
}
