package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- lock_object ---

func TestLockObjectTool(t *testing.T) {
	var gotURI string
	lockMap := adt.NewLockMap()
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			gotURI = uri
			return testLockHandle123, nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "lock_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q, want %q", gotURI, testObjectURI)
	}
	// Result text should be the lock handle.
	text := firstText(result)
	if text != testLockHandle123 {
		t.Errorf("result text = %q, want %q", text, testLockHandle123)
	}
	// Lock map should be populated.
	state, ok := lockMap.Get("dev:" + testObjectURI)
	if !ok {
		t.Fatal("expected lock map entry after lock_object")
	}
	if state.LockHandle != testLockHandle123 {
		t.Errorf("lock map handle: got %q, want %q", state.LockHandle, testLockHandle123)
	}
}

func TestLockObjectToolError(t *testing.T) {
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return "", &adt.ADTError{StatusCode: 423, Message: "already locked"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "lock_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- unlock_object ---

func TestUnlockObjectTool(t *testing.T) {
	var gotURI, gotHandle string
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "handle123", "")
	mock := &mockClient{
		unlockObjectFn: func(ctx context.Context, uri, lockHandle string) error {
			gotURI, gotHandle = uri, lockHandle
			return nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "unlock_object", map[string]interface{}{
		"object_uri":  testObjectURI,
		"lock_handle": "handle123",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q, want %q", gotURI, testObjectURI)
	}
	if gotHandle != "handle123" {
		t.Errorf("lock_handle: got %q", gotHandle)
	}
	// Lock map entry should be cleared.
	if _, ok := lockMap.Get("dev:" + testObjectURI); ok {
		t.Error("expected lock map entry to be deleted after unlock_object")
	}
}

func TestUnlockObjectToolFromLockMap(t *testing.T) {
	var gotHandle string
	lockMap := adt.NewLockMap()
	lockMap.Set("dev:"+testObjectURI, "auto-handle-456", "")
	mock := &mockClient{
		unlockObjectFn: func(ctx context.Context, uri, lockHandle string) error {
			gotHandle = lockHandle
			return nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)
	// Do NOT pass lock_handle — should be looked up from lock map automatically.
	result := callTool(t, s, "unlock_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotHandle != "auto-handle-456" {
		t.Errorf("lock_handle from map: got %q, want %q", gotHandle, "auto-handle-456")
	}
	// Lock map entry should be cleared.
	if _, ok := lockMap.Get("dev:" + testObjectURI); ok {
		t.Error("expected lock map entry to be deleted after unlock_object")
	}
}

func TestUnlockObjectToolNoHandle(t *testing.T) {
	mock := &mockClient{}
	// Use an empty lock map — no entry for this URI.
	s := newTestServerWithLockMap(mock, adt.NewLockMap())
	result := callTool(t, s, "unlock_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true when no handle and nothing in lock map")
	}
}

func TestUnlockObjectToolError(t *testing.T) {
	mock := &mockClient{
		unlockObjectFn: func(ctx context.Context, uri, lockHandle string) error {
			return &adt.ADTError{StatusCode: 400, Message: "unlock failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "unlock_object", map[string]interface{}{
		"object_uri":  testObjectURI,
		"lock_handle": "handle123",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- pretty_print ---

func TestPrettyPrintTool(t *testing.T) {
	const inputSource = "REPORT ZTEST .\nDATA: lv_x TYPE i."
	const formatted = "REPORT ZTEST.\nDATA: lv_x TYPE i."
	mock := &mockClient{
		prettyPrintFn: func(ctx context.Context, source string) (string, error) {
			if source != inputSource {
				t.Errorf("source: got %q, want %q", source, inputSource)
			}
			return formatted, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "pretty_print", map[string]interface{}{
		"source": inputSource,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if text := firstText(result); text != formatted {
		t.Errorf("result text = %q, want %q", text, formatted)
	}
}

func TestPrettyPrintToolError(t *testing.T) {
	mock := &mockClient{
		prettyPrintFn: func(ctx context.Context, source string) (string, error) {
			return "", &adt.ADTError{StatusCode: 500, Message: "pretty print failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "pretty_print", map[string]interface{}{
		"source": "REPORT Z.",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- create_object ---

func TestCreateObjectTool(t *testing.T) {
	var gotType, gotName, gotPkg, gotDesc, gotTransport string
	mock := &mockClient{
		createObjectFn: func(ctx context.Context, objectType, name, pkg, desc, transport string) error {
			gotType, gotName, gotPkg, gotDesc, gotTransport = objectType, name, pkg, desc, transport
			return nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "create_object", map[string]interface{}{
		"object_type": "PROG",
		"name":        "ZTEST_NEW",
		"package":     "$TMP",
		"description": "Test program",
		"transport":   "",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotType != "PROG" {
		t.Errorf("object_type: got %q", gotType)
	}
	if gotName != "ZTEST_NEW" {
		t.Errorf("name: got %q", gotName)
	}
	if gotPkg != "$TMP" {
		t.Errorf("package: got %q", gotPkg)
	}
	if gotDesc != "Test program" {
		t.Errorf("description: got %q", gotDesc)
	}
	if gotTransport != "" {
		t.Errorf("transport: got %q", gotTransport)
	}
}

func TestCreateObjectToolError(t *testing.T) {
	mock := &mockClient{
		createObjectFn: func(ctx context.Context, objectType, name, pkg, desc, transport string) error {
			return &adt.ADTError{StatusCode: 409, Message: "already exists"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "create_object", map[string]interface{}{
		"object_type": "PROG",
		"name":        "ZEXISTS",
		"package":     "$TMP",
		"description": "desc",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- delete_object ---

func TestDeleteObjectTool(t *testing.T) {
	var gotURI, gotLockHandle, gotTransport string
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return "LOCK-HANDLE-123", nil
		},
		deleteObjectFn: func(ctx context.Context, uri, lockHandle, transport string) error {
			gotURI, gotLockHandle, gotTransport = uri, lockHandle, transport
			return nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": testObjectURI,
		"transport":  "DEVK900001",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("object_uri: got %q", gotURI)
	}
	if gotLockHandle != "LOCK-HANDLE-123" {
		t.Errorf("lockHandle: got %q", gotLockHandle)
	}
	if gotTransport != "DEVK900001" {
		t.Errorf("transport: got %q", gotTransport)
	}
}

func TestDeleteObjectToolError(t *testing.T) {
	mock := &mockClient{
		lockObjectFn: func(ctx context.Context, uri string) (string, error) {
			return "LOCK-HANDLE-123", nil
		},
		deleteObjectFn: func(ctx context.Context, uri, lockHandle, transport string) error {
			return &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "delete_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- get_completions ---

func TestGetCompletionsTool(t *testing.T) {
	var gotURI, gotSource string
	var gotLine, gotColumn int
	mock := &mockClient{
		getCompletionsFn: func(ctx context.Context, uri, source string, line, column int) ([]adt.CompletionItem, error) {
			gotURI, gotSource, gotLine, gotColumn = uri, source, line, column
			return []adt.CompletionItem{{Text: "DATA", Description: "keyword"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_completions", map[string]interface{}{
		"object_uri": testObjectURI,
		"source":     "REPORT Z.\nDA",
		"line":       float64(2),
		"column":     float64(3),
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q", gotURI)
	}
	if gotSource != "REPORT Z.\nDA" {
		t.Errorf("source: got %q", gotSource)
	}
	if gotLine != 2 {
		t.Errorf("line: got %d, want 2", gotLine)
	}
	if gotColumn != 3 {
		t.Errorf("column: got %d, want 3", gotColumn)
	}
}

func TestGetCompletionsToolError(t *testing.T) {
	mock := &mockClient{
		getCompletionsFn: func(ctx context.Context, uri, source string, line, column int) ([]adt.CompletionItem, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "completion error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_completions", map[string]interface{}{
		"object_uri": testObjectURI,
		"source":     "REPORT Z.",
		"line":       float64(1),
		"column":     float64(9),
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- where_used ---

func TestWhereUsedTool(t *testing.T) {
	var gotURI string
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			gotURI = uri
			return []adt.ObjectInfo{{Name: "ZCALLER", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q", gotURI)
	}
}

func TestWhereUsedToolError(t *testing.T) {
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "where used failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- browse_package ---

func TestBrowsePackageTool(t *testing.T) {
	var gotPkg string
	mock := &mockClient{
		browsePackageFn: func(ctx context.Context, pkg string) ([]adt.ObjectInfo, error) {
			gotPkg = pkg
			return []adt.ObjectInfo{{Name: "ZREPORT", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "browse_package", map[string]interface{}{
		"package_name": "ZPACKAGE",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotPkg != "ZPACKAGE" {
		t.Errorf("package: got %q, want %q", gotPkg, "ZPACKAGE")
	}
}

func TestBrowsePackageToolError(t *testing.T) {
	mock := &mockClient{
		browsePackageFn: func(ctx context.Context, pkg string) ([]adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "package not found"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "browse_package", map[string]interface{}{
		"package_name": "ZNOEXIST",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- get_object_info ---

func TestGetObjectInfoTool(t *testing.T) {
	var gotURI string
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			gotURI = uri
			return &adt.ObjectInfo{Name: "ZTEST", Type: "PROG/P", Description: "My test program"}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q", gotURI)
	}
	text := firstText(result)
	var info adt.ObjectInfo
	if err := json.Unmarshal([]byte(text), &info); err != nil {
		t.Fatalf("unmarshal result: %v\ntext: %q", err, text)
	}
	if info.Name != "ZTEST" {
		t.Errorf("name: got %q, want %q", info.Name, "ZTEST")
	}
}

func TestGetObjectInfoToolError(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- syntax_check ---

func TestSyntaxCheckTool(t *testing.T) {
	var gotURI string
	mock := &mockClient{
		syntaxCheckFn: func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
			gotURI = uri
			return []adt.SyntaxMessage{{Line: 1, Column: 1, Type: "E", Text: "syntax error"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "syntax_check", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q", gotURI)
	}
}

func TestSyntaxCheckToolError(t *testing.T) {
	mock := &mockClient{
		syntaxCheckFn: func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "check failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "syntax_check", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- run_unit_tests ---

func TestRunUnitTestsTool(t *testing.T) {
	var gotURI string
	var gotTimeout int
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			gotURI, gotTimeout = uri, timeout
			return &adt.TestResult{Passed: 5, Failed: 0}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri":      testObjectURI,
		"timeout_seconds": float64(60),
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("uri: got %q", gotURI)
	}
	if gotTimeout != 60 {
		t.Errorf("timeout: got %d, want 60", gotTimeout)
	}
}

func TestRunUnitTestsToolError(t *testing.T) {
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "test run failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// --- add_to_transport error path ---

func TestAddToTransportToolError(t *testing.T) {
	mock := &mockClient{
		addTransportFn: func(ctx context.Context, uri, transport string) error {
			return &adt.ADTError{StatusCode: 500, Message: "transport error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "add_to_transport", map[string]interface{}{
		"object_uri": testObjectURI,
		"transport":  "DEVK900001",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

// firstText extracts the first text content from a tool result.
func firstText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
