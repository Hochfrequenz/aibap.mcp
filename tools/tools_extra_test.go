package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// SAP object names and use_type strings that appear in multiple test assertions.
// Extracted to satisfy the goconst linter (threshold: 3 occurrences).
const (
	testSYST      = "SYST"
	testStructure = "STRUCTURE"
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
	// Result is a structured LockResult with the handle.
	var got struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.Handle != testLockHandle123 {
		t.Errorf("handle = %q, want %q", got.Handle, testLockHandle123)
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
	var got struct {
		Formatted string `json:"formatted"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.Formatted != formatted {
		t.Errorf("formatted = %q, want %q", got.Formatted, formatted)
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
	var gotURI, gotTransport string
	mock := &mockClient{
		deleteObjectFn: func(ctx context.Context, uri, lockHandle, transport string) error {
			gotURI, gotTransport = uri, transport
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
	if gotTransport != "DEVK900001" {
		t.Errorf("transport: got %q", gotTransport)
	}
}

func TestDeleteObjectToolError(t *testing.T) {
	mock := &mockClient{
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

// --- where_used (batch) ---

func TestBatchWhereUsedTool(t *testing.T) {
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			if uri == testObjectURIFail {
				return nil, &adt.ADTError{StatusCode: 500, Message: "lookup failed"}
			}
			return []adt.ObjectInfo{{Name: "ZCALLER", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": []string{
			testObjectURIOK,
			testObjectURIFail,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total           int `json:"total"`
		TotalReferences int `json:"total_references"`
		Results         []struct {
			ObjectURI  string           `json:"object_uri"`
			References []adt.ObjectInfo `json:"references"`
			Error      string           `json:"error,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
	if out.TotalReferences != 1 {
		t.Errorf("total_references: got %d, want 1", out.TotalReferences)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results length: got %d, want 2", len(out.Results))
	}
	if out.Results[0].ObjectURI != testObjectURIOK {
		t.Errorf("first result URI: got %q", out.Results[0].ObjectURI)
	}
	if len(out.Results[0].References) != 1 {
		t.Errorf("first result should have 1 reference, got %d", len(out.Results[0].References))
	}
	if out.Results[1].ObjectURI != testObjectURIFail {
		t.Errorf("second result URI: got %q", out.Results[1].ObjectURI)
	}
	if out.Results[1].Error == "" {
		t.Error("second result should have an error")
	}
}

func TestBatchWhereUsedSingleURI(t *testing.T) {
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			return []adt.ObjectInfo{{Name: "ZCALLER", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": []string{testObjectURI},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total           int `json:"total"`
		TotalReferences int `json:"total_references"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 {
		t.Errorf("total: got %d, want 1", out.Total)
	}
	if out.TotalReferences != 1 {
		t.Errorf("total_references: got %d, want 1", out.TotalReferences)
	}
}

func TestBatchWhereUsedAllErrors(t *testing.T) {
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "server error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": []string{
			"/sap/bc/adt/programs/programs/ZFAIL1",
			"/sap/bc/adt/programs/programs/ZFAIL2",
		},
	})
	if result.IsError {
		t.Fatalf("unexpected tool-level error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		TotalReferences int `json:"total_references"`
		Results         []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalReferences != 0 {
		t.Errorf("total_references: got %d, want 0", out.TotalReferences)
	}
	for i, r := range out.Results {
		if r.Error == "" {
			t.Errorf("result[%d] should have an error", i)
		}
	}
}

func TestBatchWhereUsedEmptyURIs(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": []string{},
	})
	if result.IsError {
		t.Errorf("unexpected error for empty array: %s", firstText(result))
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

// --- get_object_info (batch) ---

func TestBatchGetObjectInfoTool(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			if uri == testObjectURIFail {
				return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
			}
			return &adt.ObjectInfo{Name: "ZOK", Type: "PROG/P", URI: uri}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": []string{testObjectURIOK, testObjectURIFail},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		Results   []struct {
			ObjectURI string          `json:"object_uri"`
			Info      *adt.ObjectInfo `json:"info,omitempty"`
			Error     string          `json:"error,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
	if out.Succeeded != 1 {
		t.Errorf("succeeded: got %d, want 1", out.Succeeded)
	}
	if out.Failed != 1 {
		t.Errorf("failed: got %d, want 1", out.Failed)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results length: got %d, want 2", len(out.Results))
	}
	if out.Results[0].ObjectURI != testObjectURIOK {
		t.Errorf("first result URI: got %q", out.Results[0].ObjectURI)
	}
	if out.Results[0].Info == nil || out.Results[0].Info.Name != "ZOK" {
		t.Errorf("first result info: got %v", out.Results[0].Info)
	}
	if out.Results[1].Error == "" {
		t.Error("second result should have an error")
	}
}

func TestBatchGetObjectInfoSingleURI(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			return &adt.ObjectInfo{Name: "ZTEST", Type: "PROG/P"}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": []string{testObjectURI},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 || out.Succeeded != 1 {
		t.Errorf("expected total=1, succeeded=1, got total=%d, succeeded=%d", out.Total, out.Succeeded)
	}
}

func TestBatchGetObjectInfoAllErrors(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "server error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": []string{"/sap/bc/adt/programs/programs/ZFAIL1", "/sap/bc/adt/programs/programs/ZFAIL2"},
	})
	if result.IsError {
		t.Fatalf("unexpected tool-level error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Failed != 2 {
		t.Errorf("failed: got %d, want 2", out.Failed)
	}
}

func TestBatchGetObjectInfoEmptyURIs(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": []string{},
	})
	if result.IsError {
		t.Errorf("unexpected error for empty array: %s", firstText(result))
	}
}

// --- run_unit_tests (batch) ---

func TestBatchRunUnitTestsTool(t *testing.T) {
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			if uri == testObjectURIFail {
				return nil, &adt.ADTError{StatusCode: 500, Message: "test run failed"}
			}
			return &adt.TestResult{Passed: 3, Failed: 1}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": []string{
			testObjectURIOK,
			testObjectURIFail,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		TotalObjects int `json:"total_objects"`
		TotalPassed  int `json:"total_passed"`
		TotalFailed  int `json:"total_failed"`
		Results      []struct {
			ObjectURI  string          `json:"object_uri"`
			TestResult *adt.TestResult `json:"test_result,omitempty"`
			Error      string          `json:"error,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalObjects != 2 {
		t.Errorf("total_objects: got %d, want 2", out.TotalObjects)
	}
	if out.TotalPassed != 3 {
		t.Errorf("total_passed: got %d, want 3", out.TotalPassed)
	}
	if out.TotalFailed != 1 {
		t.Errorf("total_failed: got %d, want 1", out.TotalFailed)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results length: got %d, want 2", len(out.Results))
	}
	// First result should have test_result, no error.
	if out.Results[0].TestResult == nil {
		t.Error("first result should have test_result")
	}
	if out.Results[0].Error != "" {
		t.Errorf("first result should have no error, got %q", out.Results[0].Error)
	}
	// Second result should have error.
	if out.Results[1].Error == "" {
		t.Error("second result should have error")
	}
}

func TestBatchRunUnitTestsSingleURI(t *testing.T) {
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			return &adt.TestResult{Passed: 10, Failed: 0}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": []string{testObjectURIOK},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		TotalObjects int `json:"total_objects"`
		TotalPassed  int `json:"total_passed"`
		TotalFailed  int `json:"total_failed"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalObjects != 1 {
		t.Errorf("total_objects: got %d, want 1", out.TotalObjects)
	}
	if out.TotalPassed != 10 {
		t.Errorf("total_passed: got %d, want 10", out.TotalPassed)
	}
}

func TestBatchRunUnitTestsAllErrors(t *testing.T) {
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "server error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": []string{
			"/sap/bc/adt/programs/programs/ZFAIL1",
			"/sap/bc/adt/programs/programs/ZFAIL2",
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		TotalObjects int `json:"total_objects"`
		TotalPassed  int `json:"total_passed"`
		TotalFailed  int `json:"total_failed"`
		Results      []struct {
			Error string `json:"error,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalPassed != 0 {
		t.Errorf("total_passed: got %d, want 0", out.TotalPassed)
	}
	if out.TotalFailed != 0 {
		t.Errorf("total_failed: got %d, want 0", out.TotalFailed)
	}
	for i, r := range out.Results {
		if r.Error == "" {
			t.Errorf("result %d should have error", i)
		}
	}
}

func TestBatchRunUnitTestsEmptyURIs(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": []string{},
	})
	if result.IsError {
		t.Errorf("unexpected error for empty array: %s", firstText(result))
	}
}

// --- syntax_check (batch) ---

func TestBatchSyntaxCheckTool(t *testing.T) {
	mock := &mockClient{
		syntaxCheckFn: func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
			if uri == testObjectURIFail {
				return []adt.SyntaxMessage{
					{Type: "E", Text: "Syntax error", Line: 10, Column: 5},
				}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "syntax_check", map[string]interface{}{
		"object_uri": []string{
			testObjectURIOK,
			testObjectURIFail,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total         int                      `json:"total"`
		Clean         int                      `json:"clean"`
		TotalErrors   int                      `json:"total_errors"`
		TotalWarnings int                      `json:"total_warnings"`
		Results       []adt.ObjectSyntaxResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
	if out.Clean != 1 {
		t.Errorf("clean: got %d, want 1", out.Clean)
	}
	if out.TotalErrors != 1 {
		t.Errorf("total_errors: got %d, want 1", out.TotalErrors)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results length: got %d, want 2", len(out.Results))
	}
	if len(out.Results[0].Messages) != 0 {
		t.Errorf("first result should have 0 messages, got %d", len(out.Results[0].Messages))
	}
	if len(out.Results[1].Messages) != 1 {
		t.Errorf("second result should have 1 message, got %d", len(out.Results[1].Messages))
	}
}

func TestBatchSyntaxCheckEmptyURIs(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "syntax_check", map[string]interface{}{
		"object_uri": []string{},
	})
	if result.IsError {
		t.Errorf("unexpected error for empty array: %s", firstText(result))
	}
}

// --- batch_get_source ---

func TestBatchGetSourceTool(t *testing.T) {
	lockMap := adt.NewLockMap()
	// Pre-populate lock map entry so UpdateETag has something to update.
	lockMap.Set("dev:/sap/bc/adt/programs/programs/ZOK", "lock-handle-ok", "")
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			if uri == testObjectURIFail {
				return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
			}
			return &adt.SourceResult{Source: "REPORT ZOK.", ETag: `"etag-ok"`}, nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []string{
			testObjectURIOK,
			testObjectURIFail,
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total   int `json:"total"`
		Results []struct {
			ObjectURI string `json:"object_uri"`
			Source    string `json:"source"`
			ETag      string `json:"etag"`
			Error     string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results length: got %d, want 2", len(out.Results))
	}
	// First result: success
	if out.Results[0].ObjectURI != testObjectURIOK {
		t.Errorf("first result URI: got %q", out.Results[0].ObjectURI)
	}
	if out.Results[0].Source != "REPORT ZOK." {
		t.Errorf("first result source: got %q", out.Results[0].Source)
	}
	if out.Results[0].ETag != `"etag-ok"` {
		t.Errorf("first result etag: got %q", out.Results[0].ETag)
	}
	if out.Results[0].Error != "" {
		t.Errorf("first result should have no error, got %q", out.Results[0].Error)
	}
	// Second result: error
	if out.Results[1].ObjectURI != testObjectURIFail {
		t.Errorf("second result URI: got %q", out.Results[1].ObjectURI)
	}
	if out.Results[1].Error == "" {
		t.Error("second result should have an error")
	}
	// Verify ETag stored in lockMap for successful result
	state, ok := lockMap.Get("dev:/sap/bc/adt/programs/programs/ZOK")
	if !ok {
		t.Fatal("expected lock map entry for successful URI")
	}
	if state.ETag != `"etag-ok"` {
		t.Errorf("lock map ETag: got %q, want %q", state.ETag, `"etag-ok"`)
	}
	// Verify no lock map entry for failed result
	if _, ok := lockMap.Get("dev:/sap/bc/adt/programs/programs/ZFAIL"); ok {
		t.Error("expected no lock map entry for failed URI")
	}
}

func TestBatchGetSourceSingleURI(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"etag-1"`}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []string{testObjectURI},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total   int `json:"total"`
		Results []struct {
			Source string `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 {
		t.Errorf("total: got %d, want 1", out.Total)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results length: got %d, want 1", len(out.Results))
	}
	if out.Results[0].Source != "REPORT ZTEST." {
		t.Errorf("source: got %q", out.Results[0].Source)
	}
}

func TestBatchGetSourceAllErrors(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "server error"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []string{
			"/sap/bc/adt/programs/programs/ZFAIL1",
			"/sap/bc/adt/programs/programs/ZFAIL2",
		},
	})
	if result.IsError {
		t.Fatalf("unexpected tool-level error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for i, r := range out.Results {
		if r.Error == "" {
			t.Errorf("result[%d] should have an error", i)
		}
	}
}

func TestBatchGetSourceEmptyURIs(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []string{},
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 0 {
		t.Errorf("total: got %d, want 0", out.Total)
	}
}

// --- get_object_dependencies ---

func TestGetObjectDependenciesTool(t *testing.T) {
	var d010tabSQL string
	var d010tabMaxRows int
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				d010tabSQL = sql
				d010tabMaxRows = maxRows
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{"SCREEN"}, {testSYST}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				// DD02L is queried first for all names. SCREEN=INTTAB (structure),
				// SYST=TRANSP (transparent table). Both are classified here, so no
				// TADIR query follows.
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{"SCREEN", "INTTAB"}, {testSYST, "TRANSP"}},
				}, nil
			default:
				// TADIR must not be queried when DD02L already classified all names.
				t.Errorf("unexpected SQL query: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_MY_REPORT",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}

	if !strings.Contains(d010tabSQL, "MASTER = 'Z_MY_REPORT'") {
		t.Errorf("D010TAB SQL missing program name filter, got: %s", d010tabSQL)
	}
	if d010tabMaxRows != 200 {
		t.Errorf("D010TAB maxRows: got %d, want 200 (default)", d010tabMaxRows)
	}

	text := firstText(result)
	var out struct {
		ObjectType   string `json:"object_type"`
		ObjectName   string `json:"object_name"`
		Count        int    `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal result: %v\ntext: %q", err, text)
	}
	if out.ObjectType != "PROG" {
		t.Errorf("object_type: got %q, want %q", out.ObjectType, "PROG")
	}
	if out.ObjectName != "Z_MY_REPORT" {
		t.Errorf("object_name: got %q, want %q", out.ObjectName, "Z_MY_REPORT")
	}
	if out.Count != 2 {
		t.Errorf("count: got %d, want 2", out.Count)
	}
	if len(out.Dependencies) != 2 {
		t.Fatalf("dependencies length: got %d, want 2", len(out.Dependencies))
	}
	if out.Dependencies[0].Name != "SCREEN" {
		t.Errorf("dep[0].name: got %q, want SCREEN", out.Dependencies[0].Name)
	}
	if out.Dependencies[0].UseType != testStructure {
		t.Errorf("dep[0].use_type: got %q, want STRUCTURE (SCREEN is INTTAB in DD02L)", out.Dependencies[0].UseType)
	}
	if out.Dependencies[1].Name != testSYST {
		t.Errorf("dep[1].name: got %q, want SYST", out.Dependencies[1].Name)
	}
	if out.Dependencies[1].UseType != "TABLE" {
		t.Errorf("dep[1].use_type: got %q, want TABLE (SYST is TRANSP in DD02L)", out.Dependencies[1].UseType)
	}
}

func TestGetObjectDependenciesClassifiesTypes(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					// INT1=data element, MANDT=domain, ZVIEW=view, ZTTYP=table type,
					// ZUNKNOWN=not in any catalog.
					Rows: [][]string{{"INT1"}, {"MANDT"}, {"ZVIEW"}, {"ZTTYP"}, {"ZUNKNOWN"}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				// DD02L is always queried first. None of these names are tables or
				// structures, so no rows are returned.
				return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}}, Rows: nil}, nil
			case strings.Contains(sql, "TADIR"):
				// After DD02L returns nothing, all names are still UNKNOWN and are
				// passed to TADIR. ZUNKNOWN has no TADIR entry and stays UNKNOWN.
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "OBJECT"}, {Name: "OBJ_NAME"}},
					Rows: [][]string{
						{"DTEL", "INT1"},
						{"DOMA", "MANDT"},
						{"VIEW", "ZVIEW"},
						{"TTYP", "ZTTYP"},
					},
				}, nil
			default:
				t.Errorf("unexpected SQL query: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_TEST",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}

	var out struct {
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Dependencies) != 5 {
		t.Fatalf("dependencies length: got %d, want 5", len(out.Dependencies))
	}

	byName := map[string]string{}
	for _, d := range out.Dependencies {
		byName[d.Name] = d.UseType
	}
	if byName["INT1"] != "DATA_ELEMENT" {
		t.Errorf("INT1 use_type: got %q, want DATA_ELEMENT", byName["INT1"])
	}
	if byName["MANDT"] != "DOMAIN" {
		t.Errorf("MANDT use_type: got %q, want DOMAIN", byName["MANDT"])
	}
	if byName["ZVIEW"] != "VIEW" {
		t.Errorf("ZVIEW use_type: got %q, want VIEW", byName["ZVIEW"])
	}
	if byName["ZTTYP"] != "TABLE_TYPE" {
		t.Errorf("ZTTYP use_type: got %q, want TABLE_TYPE", byName["ZTTYP"])
	}
	if byName["ZUNKNOWN"] != "UNKNOWN" {
		t.Errorf("ZUNKNOWN use_type: got %q, want UNKNOWN", byName["ZUNKNOWN"])
	}
}

func TestGetObjectDependenciesUnknownTabclass(t *testing.T) {
	// An object present in DD02L with an unrecognised TABCLASS must not be classified
	// as TABLE — it should fall through to UNKNOWN rather than silently misclassify.
	// This guards against future SAP TABCLASS values we don't know about yet.
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{"ZFUTURE"}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				// DD02L is queried first. It returns an unrecognised TABCLASS value,
				// which tabclassToUseType maps to UNKNOWN. Because the name is still
				// UNKNOWN after DD02L, TADIR is queried next.
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{"ZFUTURE", "NEWTYPE"}},
				}, nil
			case strings.Contains(sql, "TADIR"):
				// TADIR is queried for the still-UNKNOWN name. Returning empty means
				// ZFUTURE stays UNKNOWN — the correct outcome when classification is
				// genuinely ambiguous.
				return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "OBJECT"}, {Name: "OBJ_NAME"}}, Rows: nil}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_TEST",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Dependencies) != 1 {
		t.Fatalf("dependencies length: got %d, want 1", len(out.Dependencies))
	}
	if out.Dependencies[0].UseType != "UNKNOWN" {
		t.Errorf("ZFUTURE with unrecognised TABCLASS=NEWTYPE: got %q, want UNKNOWN", out.Dependencies[0].UseType)
	}
}

func TestGetObjectDependenciesToolCustomMaxResults(t *testing.T) {
	var gotMaxRows int
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, maxRows int) (*adt.QueryResult, error) {
			gotMaxRows = maxRows
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "TABNAME"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "ZTEST",
		"max_results": float64(50),
	})
	if gotMaxRows != 50 {
		t.Errorf("maxRows: got %d, want 50", gotMaxRows)
	}
}

func TestGetObjectDependenciesToolEmpty(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "TABNAME"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "Z_STANDALONE",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	text := firstText(result)
	var out struct {
		Count        int `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count != 0 {
		t.Errorf("count: got %d, want 0", out.Count)
	}
	if len(out.Dependencies) != 0 {
		t.Errorf("dependencies: got %d, want 0", len(out.Dependencies))
	}
}

func TestGetObjectDependenciesToolError(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "query failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "CLAS",
		"object_name": "/HFQ/MY_CLASS",
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestGetObjectDependenciesLowercaseType(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "D010TAB") {
				return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "TABNAME"}}, Rows: nil}, nil
			}
			if strings.Contains(sql, "DD02L") {
				return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}}, Rows: nil}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "prog", // lowercase — must still work
		"object_name": "Z_MY_REPORT",
	})
	if result.IsError {
		t.Errorf("lowercase 'prog' should be accepted, got error: %s", firstText(result))
	}
}

func TestGetObjectDependenciesToolSQLEscaping(t *testing.T) {
	var gotSQL string
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			gotSQL = sql
			return &adt.QueryResult{
				Columns: []adt.QueryColumn{{Name: "TABNAME"}},
				Rows:    [][]string{},
			}, nil
		},
	}
	s := newTestServer(mock)
	callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "PROG",
		"object_name": "O'REILLY_PROG",
	})
	if !strings.Contains(gotSQL, "O''REILLY_PROG") {
		t.Errorf("single quote not escaped in object_name, got: %s", gotSQL)
	}
}

func TestGetObjectDependenciesFUGR(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				if !strings.Contains(sql, "MASTER = 'SAPLZ_ADT_MCP_TEST_FGRP'") {
					t.Errorf("FUGR: unexpected MASTER, got SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{testSYST}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{testSYST, "INTTAB"}},
				}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "FUGR",
		"object_name": "Z_ADT_MCP_TEST_FGRP",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType string `json:"object_type"`
		ObjectName string `json:"object_name"`
		Count      int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "FUGR" {
		t.Errorf("object_type: got %q, want FUGR", out.ObjectType)
	}
	if out.ObjectName != "Z_ADT_MCP_TEST_FGRP" {
		t.Errorf("object_name: got %q, want Z_ADT_MCP_TEST_FGRP", out.ObjectName)
	}
	if out.Count != 1 {
		t.Errorf("count: got %d, want 1", out.Count)
	}
}

func TestGetObjectDependenciesFUNC(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "TFDIR"):
				if !strings.Contains(sql, "FUNCNAME = 'Z_ADT_MCP_TEST_FM'") {
					t.Errorf("TFDIR: unexpected filter, got SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "PNAME"}},
					Rows:    [][]string{{"SAPLZ_ADT_MCP_TEST_FGRP"}},
				}, nil
			case strings.Contains(sql, "D010TAB"):
				if !strings.Contains(sql, "MASTER = 'SAPLZ_ADT_MCP_TEST_FGRP'") {
					t.Errorf("FUNC D010TAB: unexpected MASTER, got SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{testSYST}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{testSYST, "INTTAB"}},
				}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "FUNC",
		"object_name": "Z_ADT_MCP_TEST_FM",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType string `json:"object_type"`
		ObjectName string `json:"object_name"`
		Count      int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "FUNC" {
		t.Errorf("object_type: got %q, want FUNC", out.ObjectType)
	}
	if out.ObjectName != "Z_ADT_MCP_TEST_FM" {
		t.Errorf("object_name: got %q, want Z_ADT_MCP_TEST_FM", out.ObjectName)
	}
}

func TestGetObjectDependenciesFUNCNotFound(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "TFDIR") {
				return &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "PNAME"}}, Rows: nil}, nil
			}
			t.Errorf("unexpected SQL after empty TFDIR: %s", sql)
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "FUNC",
		"object_name": "Z_NONEXISTENT_FM",
	})
	if !result.IsError {
		t.Errorf("expected IsError=true for unknown FUNC, got false")
	}
}

func TestGetObjectDependenciesUnsupportedType(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "MSAG",
		"object_name": "ZSOME_MSG",
	})
	if !result.IsError {
		t.Errorf("expected IsError=true for unsupported type MSAG, got false")
	}
}

func TestGetObjectDependenciesCLAS(t *testing.T) {
	// ZCL_ADT_MCP_TEST_UNITS = 22 chars → pad to 30 with 8 '=' signs → + "CP" = 32 total
	const className = "ZCL_ADT_MCP_TEST_UNITS"
	const wantMaster = "ZCL_ADT_MCP_TEST_UNITS========CP"

	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				if !strings.Contains(sql, "MASTER = '"+wantMaster+"'") {
					t.Errorf("CLAS D010TAB: unexpected MASTER in SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{testSYST}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{testSYST, "INTTAB"}},
				}, nil
			case strings.Contains(sql, "SEOMETAREL"):
				if !strings.Contains(sql, "CLSNAME = '"+className+"'") {
					t.Errorf("SEOMETAREL: unexpected CLSNAME in SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "REFCLSNAME"}, {Name: "RELTYPE"}},
					Rows: [][]string{
						{"ZIF_MY_INTF", "1"},
						{"ZCL_PARENT", "2"},
					},
				}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "CLAS",
		"object_name": className,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType   string `json:"object_type"`
		ObjectName   string `json:"object_name"`
		Count        int    `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "CLAS" {
		t.Errorf("object_type: got %q, want CLAS", out.ObjectType)
	}
	if out.Count != 3 {
		t.Errorf("count: got %d, want 3 (1 DDIC + 2 OO)", out.Count)
	}
	if out.Dependencies[0].Name != testSYST || out.Dependencies[0].UseType != testStructure {
		t.Errorf("dep[0]: got {%q,%q}, want {SYST,STRUCTURE}", out.Dependencies[0].Name, out.Dependencies[0].UseType)
	}
	if out.Dependencies[1].Name != "ZIF_MY_INTF" || out.Dependencies[1].UseType != "INTERFACE" {
		t.Errorf("dep[1]: got {%q,%q}, want {ZIF_MY_INTF,INTERFACE}", out.Dependencies[1].Name, out.Dependencies[1].UseType)
	}
	if out.Dependencies[2].Name != "ZCL_PARENT" || out.Dependencies[2].UseType != "SUPERCLASS" {
		t.Errorf("dep[2]: got {%q,%q}, want {ZCL_PARENT,SUPERCLASS}", out.Dependencies[2].Name, out.Dependencies[2].UseType)
	}
}

func TestGetObjectDependenciesINTF(t *testing.T) {
	const intfName = "ZIF_ABAPGIT_AJSON"
	// 17 chars → pad to 30 with 13 '=' signs → + "IP" = 32 total
	const wantMaster = "ZIF_ABAPGIT_AJSON=============IP"

	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				if !strings.Contains(sql, "MASTER = '"+wantMaster+"'") {
					t.Errorf("INTF D010TAB: unexpected MASTER in SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    [][]string{{testSYST}},
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    [][]string{{testSYST, "INTTAB"}},
				}, nil
			case strings.Contains(sql, "SEOMETAREL"):
				if !strings.Contains(sql, "CLSNAME = '"+intfName+"'") {
					t.Errorf("SEOMETAREL: unexpected CLSNAME in SQL: %s", sql)
				}
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "REFCLSNAME"}, {Name: "RELTYPE"}},
					Rows:    [][]string{{"ZIF_EXTENDED", "0"}},
				}, nil
			default:
				t.Errorf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "INTF",
		"object_name": intfName,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType   string `json:"object_type"`
		Count        int    `json:"count"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "INTF" {
		t.Errorf("object_type: got %q, want INTF", out.ObjectType)
	}
	if out.Count != 2 {
		t.Errorf("count: got %d, want 2 (1 DDIC + 1 OO)", out.Count)
	}
	if out.Dependencies[0].Name != testSYST || out.Dependencies[0].UseType != testStructure {
		t.Errorf("dep[0]: got {%q,%q}, want {SYST,STRUCTURE}", out.Dependencies[0].Name, out.Dependencies[0].UseType)
	}
	if out.Dependencies[1].Name != "ZIF_EXTENDED" || out.Dependencies[1].UseType != "INTERFACE" {
		t.Errorf("dep[1]: got {%q,%q}, want {ZIF_EXTENDED,INTERFACE}", out.Dependencies[1].Name, out.Dependencies[1].UseType)
	}
}

// --- get_object_dependencies: DDIC types (TABL/DTEL/DOMA/TTYP) ---

func TestGetObjectDependenciesTABL(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "DD03L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ROLLNAME"}, {Name: "CHECKTABLE"}},
					Rows:    [][]string{{"S_CARR_ID", ""}},
				}, nil
			default:
				return nil, nil
			}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "TABL",
		"object_name": "SCARR",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType   string   `json:"object_type"`
		ObjectName   string   `json:"object_name"`
		Count        int      `json:"count"`
		Warnings     []string `json:"warnings,omitempty"`
		Dependencies []struct {
			Name    string `json:"name"`
			UseType string `json:"use_type"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "TABL" {
		t.Errorf("object_type: got %q, want TABL", out.ObjectType)
	}
	if out.ObjectName != "SCARR" {
		t.Errorf("object_name: got %q, want SCARR", out.ObjectName)
	}
	if out.Count != 1 {
		t.Errorf("count: got %d, want 1", out.Count)
	}
	if out.Warnings != nil {
		t.Errorf("expected no warnings, got: %v", out.Warnings)
	}
	if len(out.Dependencies) != 1 || out.Dependencies[0].Name != "S_CARR_ID" || out.Dependencies[0].UseType != "DATA_ELEMENT" {
		t.Errorf("dependency: got %+v", out.Dependencies)
	}
}

//nolint:dupl
func TestGetObjectDependenciesDTEL(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "DD04L") {
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "DOMNAME"}},
					Rows:    [][]string{{"S_CARR_ID"}},
				}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "DTEL",
		"object_name": "S_CARR_ID",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType string `json:"object_type"`
		Count      int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "DTEL" {
		t.Errorf("object_type: got %q, want DTEL", out.ObjectType)
	}
	if out.Count != 1 {
		t.Errorf("count: got %d, want 1", out.Count)
	}
}

//nolint:dupl
func TestGetObjectDependenciesDOMA(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "DD01L") {
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ENTITYTAB"}},
					Rows:    [][]string{{"T000"}},
				}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "DOMA",
		"object_name": "MANDT",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType string `json:"object_type"`
		Count      int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "DOMA" {
		t.Errorf("object_type: got %q, want DOMA", out.ObjectType)
	}
	if out.Count != 1 {
		t.Errorf("count: got %d, want 1", out.Count)
	}
}

func TestGetObjectDependenciesTTYP(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "DD40L") {
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "ROWTYPE"}, {Name: "ROWKIND"}},
					Rows:    [][]string{{"S_CARR_ID", "E"}},
				}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "TTYP",
		"object_name": "TT_CARR_IDS",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var out struct {
		ObjectType string `json:"object_type"`
		Count      int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ObjectType != "TTYP" {
		t.Errorf("object_type: got %q, want TTYP", out.ObjectType)
	}
	if out.Count != 1 {
		t.Errorf("count: got %d, want 1", out.Count)
	}
}

func TestGetObjectDependenciesMaxDepthClamping(t *testing.T) {
	// max_depth outside [1,10] must be silently clamped. The test verifies the
	// tool returns a result rather than an error — actual depth-limiting is
	// covered by TestDdicChainDeps_MaxDepth.
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, nil
		},
	}
	s := newTestServer(mock)
	for _, depth := range []float64{11, 0, -1} {
		result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
			"object_type": "TABL",
			"object_name": "SCARR",
			"max_depth":   depth,
		})
		if result.IsError {
			t.Errorf("max_depth=%v should be accepted (clamped), got error: %s", depth, firstText(result))
		}
	}
}

func TestGetObjectDependenciesLowercaseDDICType(t *testing.T) {
	mock := &mockClient{
		runQueryFn: func(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "tabl",
		"object_name": "SCARR",
	})
	if result.IsError {
		t.Errorf("lowercase 'tabl' should be accepted, got error: %s", firstText(result))
	}
}

func TestGetObjectDependenciesTABLWarningsInOutput(t *testing.T) {
	// A query failure should produce a warning in the output, not an error result.
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			if strings.Contains(sql, "DD03L") {
				return nil, &adt.ADTError{StatusCode: 500, Message: "connection refused"}
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "TABL",
		"object_name": "SCARR",
	})
	if result.IsError {
		t.Fatalf("expected IsError=false (query error is a warning, not a tool error), got: %s", firstText(result))
	}
	var out struct {
		Count    int      `json:"count"`
		Warnings []string `json:"warnings,omitempty"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count != 0 {
		t.Errorf("count: got %d, want 0", out.Count)
	}
	if len(out.Warnings) == 0 {
		t.Error("expected at least one warning for query failure")
	}
	if !strings.Contains(out.Warnings[0], "DD03L") {
		t.Errorf("warning should mention DD03L, got: %q", out.Warnings[0])
	}
}

func TestGetObjectDependenciesNoWarningsForNonDDIC(t *testing.T) {
	// Warnings field must be omitted (omitempty) for non-DDIC types like CLAS
	// that use a different code path and never populate the warnings slice.
	mock := &mockClient{
		runQueryFn: func(_ context.Context, sql string, _ int) (*adt.QueryResult, error) {
			switch {
			case strings.Contains(sql, "D010TAB"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}},
					Rows:    nil,
				}, nil
			case strings.Contains(sql, "DD02L"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "TABNAME"}, {Name: "TABCLASS"}},
					Rows:    nil,
				}, nil
			case strings.Contains(sql, "SEOMETAREL"):
				return &adt.QueryResult{
					Columns: []adt.QueryColumn{{Name: "REFCLSNAME"}, {Name: "RELTYPE"}},
					Rows:    nil,
				}, nil
			}
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_dependencies", map[string]interface{}{
		"object_type": "CLAS",
		"object_name": "ZCL_FOO",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	// Unmarshal into a raw map so we can check key presence.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(firstText(result)), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["warnings"]; ok {
		t.Error("'warnings' key must be absent (omitempty) for CLAS responses with no warnings")
	}
}
