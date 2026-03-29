package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const testObjectURI = "/sap/bc/adt/programs/programs/ZTEST"

// Shared test constants to satisfy goconst across the package.
const (
	testETagNew       = `"etag-new"`
	testETagAfter     = `"etag-after"`
	testLockHandle123 = "lock-handle-123"
)

// mockClient is a test double for adt.Client.
type mockClient struct {
	getSourceFn       func(ctx context.Context, uri string) (*adt.SourceResult, error)
	setSourceFn       func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error)
	activateObjectsFn func(ctx context.Context, uris []string) (*adt.ActivationResult, error)
	searchFn          func(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error)
	whereUsedFn       func(ctx context.Context, uri string) ([]adt.ObjectInfo, error)
	browsePackageFn   func(ctx context.Context, pkg string) ([]adt.ObjectInfo, error)
	getObjectFn       func(ctx context.Context, uri string) (*adt.ObjectInfo, error)
	syntaxCheckFn     func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error)
	runTestsFn        func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error)
	getTransportFn    func(ctx context.Context, user, status string) ([]adt.TransportRequest, error)
	addTransportFn    func(ctx context.Context, uri, transport string) error
	lockObjectFn      func(ctx context.Context, uri string) (string, error)
	unlockObjectFn    func(ctx context.Context, uri, lockHandle string) error
	prettyPrintFn     func(ctx context.Context, source string) (string, error)
	createObjectFn    func(ctx context.Context, objectType, name, pkg, desc, transport string) error
	deleteObjectFn    func(ctx context.Context, uri, lockHandle, transport string) error
	getCompletionsFn  func(ctx context.Context, uri, source string, line, column int) ([]adt.CompletionItem, error)
}

func (m *mockClient) GetSource(ctx context.Context, uri string) (*adt.SourceResult, error) {
	if m.getSourceFn != nil {
		return m.getSourceFn(ctx, uri)
	}
	return &adt.SourceResult{}, nil
}
func (m *mockClient) SetSource(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
	if m.setSourceFn != nil {
		return m.setSourceFn(ctx, uri, source, lockHandle, transport, etag)
	}
	return "new-etag", nil
}
func (m *mockClient) ActivateObjects(ctx context.Context, uris []string) (*adt.ActivationResult, error) {
	if m.activateObjectsFn != nil {
		return m.activateObjectsFn(ctx, uris)
	}
	return &adt.ActivationResult{Success: true}, nil
}
func (m *mockClient) GetInactiveObjects(context.Context) ([]adt.ObjectInfo, error) {
	return nil, nil
}
func (m *mockClient) SearchObjects(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, q, t, n)
	}
	return nil, nil
}
func (m *mockClient) WhereUsed(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
	if m.whereUsedFn != nil {
		return m.whereUsedFn(ctx, uri)
	}
	return nil, nil
}
func (m *mockClient) BrowsePackage(ctx context.Context, pkg string) ([]adt.ObjectInfo, error) {
	if m.browsePackageFn != nil {
		return m.browsePackageFn(ctx, pkg)
	}
	return nil, nil
}
func (m *mockClient) GetObjectInfo(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, uri)
	}
	return &adt.ObjectInfo{}, nil
}
func (m *mockClient) SyntaxCheck(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
	if m.syntaxCheckFn != nil {
		return m.syntaxCheckFn(ctx, uri)
	}
	return nil, nil
}
func (m *mockClient) InlineSyntaxCheck(_ context.Context, _, _ string) ([]adt.SyntaxMessage, error) {
	return nil, nil
}
func (m *mockClient) RunUnitTests(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
	if m.runTestsFn != nil {
		return m.runTestsFn(ctx, uri, timeout)
	}
	return &adt.TestResult{}, nil
}
func (m *mockClient) CheckTransport(context.Context, string, string, string) (*adt.TransportCheckResult, error) {
	return &adt.TransportCheckResult{}, nil
}
func (m *mockClient) CreateTransport(context.Context, string, string, string, string) (string, error) {
	return "DEVK999999", nil
}
func (m *mockClient) GetTransportRequests(ctx context.Context, user, status string) ([]adt.TransportRequest, error) {
	if m.getTransportFn != nil {
		return m.getTransportFn(ctx, user, status)
	}
	return nil, nil
}
func (m *mockClient) AddToTransport(ctx context.Context, uri, transport string) error {
	if m.addTransportFn != nil {
		return m.addTransportFn(ctx, uri, transport)
	}
	return nil
}
func (m *mockClient) LockObject(ctx context.Context, uri string) (string, error) {
	if m.lockObjectFn != nil {
		return m.lockObjectFn(ctx, uri)
	}
	return "mock-lock-handle", nil
}
func (m *mockClient) UnlockObject(ctx context.Context, uri, lockHandle string) error {
	if m.unlockObjectFn != nil {
		return m.unlockObjectFn(ctx, uri, lockHandle)
	}
	return nil
}
func (m *mockClient) PrettyPrint(ctx context.Context, source string) (string, error) {
	if m.prettyPrintFn != nil {
		return m.prettyPrintFn(ctx, source)
	}
	return source, nil
}
func (m *mockClient) CreateObject(ctx context.Context, objectType, name, pkg, desc, transport string) error {
	if m.createObjectFn != nil {
		return m.createObjectFn(ctx, objectType, name, pkg, desc, transport)
	}
	return nil
}
func (m *mockClient) CreateFunctionModule(context.Context, string, string, string, string, string) error {
	return nil
}
func (m *mockClient) CreatePackage(context.Context, string, string, string, string, string, string) error {
	return nil
}
func (m *mockClient) DeleteObject(ctx context.Context, uri, lockHandle, transport string) error {
	if m.deleteObjectFn != nil {
		return m.deleteObjectFn(ctx, uri, lockHandle, transport)
	}
	return nil
}
func (m *mockClient) GetCompletions(ctx context.Context, uri, source string, line, column int) ([]adt.CompletionItem, error) {
	if m.getCompletionsFn != nil {
		return m.getCompletionsFn(ctx, uri, source, line, column)
	}
	return nil, nil
}
func (m *mockClient) ExportPackage(ctx context.Context, packageName string) ([]byte, error) {
	return nil, nil
}
func (m *mockClient) GetATCCustomizing(_ context.Context) (*adt.ATCCustomizingResult, error) {
	return &adt.ATCCustomizingResult{Properties: map[string]string{}}, nil
}
func (m *mockClient) RunATCCheck(_ context.Context, _ []string, _ string) (*adt.ATCResult, error) {
	return &adt.ATCResult{}, nil
}
func (m *mockClient) RunQuery(_ context.Context, _ string, _ int) (*adt.QueryResult, error) {
	return nil, nil
}
func (m *mockClient) ReleaseTransport(context.Context, string) error {
	return nil
}
func (m *mockClient) GetABAPDoc(context.Context, string) (string, error)           { return "", nil }
func (m *mockClient) NavigateToDefinition(context.Context, string) (string, error) { return "", nil }
func (m *mockClient) Rename(context.Context, string, string, string) (*adt.RenameResult, error) {
	return &adt.RenameResult{}, nil
}
func (m *mockClient) GetVersionHistory(context.Context, string) ([]adt.VersionInfo, error) {
	return nil, nil
}
func (m *mockClient) GetVersionSource(context.Context, string) (string, error) { return "", nil }
func (m *mockClient) DiffActiveInactive(context.Context, string) (*adt.DiffResult, error) {
	return &adt.DiffResult{}, nil
}
func (m *mockClient) GetTableFields(context.Context, string) ([]adt.FieldInfo, error) {
	return nil, nil
}
func (m *mockClient) SystemInfo() (string, string) {
	return "https://mock.example.com:443", "100"
}
func (m *mockClient) Logout(context.Context) error { return nil }

func newTestServer(client adt.Client) *server.MCPServer {
	return newTestServerWithSelector(client, &mockSelector{}, adt.NewLockMap())
}

func newTestServerWithSelector(client adt.Client, selector tools.SystemSelector, lockMap *adt.LockMap) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAllWithLockMap(s, client, selector, lockMap)
	return s
}

func newTestServerWithLockMap(client adt.Client, lockMap *adt.LockMap) *server.MCPServer {
	return newTestServerWithSelector(client, &mockSelector{}, lockMap)
}

// callTool invokes a tool via HandleMessage using JSON-RPC protocol.
func callTool(t *testing.T, s *server.MCPServer, toolName string, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		toolName, string(argsJSON))

	resp := s.HandleMessage(context.Background(), []byte(msg))

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var envelope struct {
		Result *mcp.CallToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("unmarshal response envelope: %v\nraw: %s", err, string(respBytes))
	}
	if envelope.Error != nil {
		t.Fatalf("JSON-RPC error calling %q: code=%d msg=%s", toolName, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		t.Fatalf("nil result for tool %q\nraw: %s", toolName, string(respBytes))
	}
	return envelope.Result
}

func TestGetSourceTool(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			if uri != testObjectURI {
				t.Errorf("unexpected uri: %q", uri)
			}
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"abc123"`}, nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": testObjectURI,
	})

	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	// Find text content and parse JSON
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response JSON: %v\ntext: %q", err, text)
	}
	if resp["source"] != "REPORT ZTEST." {
		t.Errorf("source: got %q", resp["source"])
	}
	if resp["etag"] != `"abc123"` {
		t.Errorf("etag: got %q", resp["etag"])
	}
}

func TestGetSourceToolError(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "Object not found"}
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": testObjectURI,
	})

	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestGetSourceUpdatesLockMapETag(t *testing.T) {
	lockMap := adt.NewLockMap()
	// Pre-populate with a lock entry (simulating a prior lock_object call).
	lockMap.Set("dev:"+testObjectURI, "lock-handle-abc", "")

	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: testETagNew}, nil
		},
	}
	s := newTestServerWithLockMap(mock, lockMap)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}

	state, ok := lockMap.Get("dev:" + testObjectURI)
	if !ok {
		t.Fatal("expected lock map entry to exist after get_source")
	}
	if state.ETag != testETagNew {
		t.Errorf("ETag in lock map: got %q, want %q", state.ETag, testETagNew)
	}
	if state.LockHandle != "lock-handle-abc" {
		t.Errorf("LockHandle should be unchanged: got %q", state.LockHandle)
	}
}

func TestActivateObjectTool(t *testing.T) {
	mock := &mockClient{
		activateObjectsFn: func(ctx context.Context, uris []string) (*adt.ActivationResult, error) {
			if len(uris) != 1 || uris[0] != testObjectURI {
				t.Errorf("unexpected uris: %v", uris)
			}
			return &adt.ActivationResult{Success: true, Messages: []adt.ActivationMessage{}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "activate_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
}

func TestActivateObjectToolError(t *testing.T) {
	mock := &mockClient{
		activateObjectsFn: func(ctx context.Context, uris []string) (*adt.ActivationResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "Activation failed"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "activate_object", map[string]interface{}{
		"object_uri": testObjectURI,
	})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestSearchObjectsTool(t *testing.T) {
	mock := &mockClient{
		searchFn: func(ctx context.Context, q, objType string, n int) ([]adt.ObjectInfo, error) {
			if q != "ZREPORT*" {
				t.Errorf("unexpected query: %q", q)
			}
			if n != 50 {
				t.Errorf("unexpected max_results: %d", n)
			}
			return []adt.ObjectInfo{{Name: "ZREPORT", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "search_objects", map[string]interface{}{
		"query": "ZREPORT*",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
}

func TestSearchObjectsToolWithType(t *testing.T) {
	var gotType string
	mock := &mockClient{
		searchFn: func(ctx context.Context, q, objType string, n int) ([]adt.ObjectInfo, error) {
			gotType = objType
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "search_objects", map[string]interface{}{
		"query":       "Z*",
		"object_type": "PROG/P",
		"max_results": float64(10),
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotType != "PROG/P" {
		t.Errorf("object_type: got %q", gotType)
	}
}

func TestGetTransportRequestsTool(t *testing.T) {
	mock := &mockClient{
		getTransportFn: func(ctx context.Context, user, status string) ([]adt.TransportRequest, error) {
			if status != "D" {
				t.Errorf("unexpected status: %q", status)
			}
			return []adt.TransportRequest{{Number: "DEVK900123", Status: "D"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_transport_requests", map[string]interface{}{
		"status": "D",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
}

func TestGetTransportRequestsToolError(t *testing.T) {
	mock := &mockClient{
		getTransportFn: func(ctx context.Context, user, status string) ([]adt.TransportRequest, error) {
			return nil, &adt.ADTError{StatusCode: 403, Message: "Forbidden"}
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_transport_requests", map[string]interface{}{})
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestAddToTransportTool(t *testing.T) {
	var gotURI, gotTransport string
	mock := &mockClient{
		addTransportFn: func(ctx context.Context, uri, transport string) error {
			gotURI, gotTransport = uri, transport
			return nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "add_to_transport", map[string]interface{}{
		"object_uri": testObjectURI,
		"transport":  "DEVK900123",
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("object_uri: got %q", gotURI)
	}
	if gotTransport != "DEVK900123" {
		t.Errorf("transport: got %q", gotTransport)
	}
}
