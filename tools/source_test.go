package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const testObjectURI = "/sap/bc/adt/programs/programs/ZTEST"

// Shared test constants to satisfy goconst across the package.
const (
	testETagNew       = `"etag-new"`
	testETagAfter     = `"etag-after"`
	testLockHandle123 = "lock-handle-123"
	testAutoHandle    = "auto-handle"
	testObjectURIOK   = "/sap/bc/adt/programs/programs/ZOK"
	testObjectURIFail = "/sap/bc/adt/programs/programs/ZFAIL"
	testTransportNum  = "DEVK900123"
)

// mockClient is a test double for adt.Client.
type mockClient struct {
	getSourceFn           func(ctx context.Context, uri string) (*adt.SourceResult, error)
	setSourceFn           func(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error)
	activateObjectsFn     func(ctx context.Context, uris []string) (*adt.ActivationResult, error)
	searchFn              func(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error)
	whereUsedFn           func(ctx context.Context, uri string) ([]adt.ObjectInfo, error)
	browsePackageFn       func(ctx context.Context, pkg string) ([]adt.ObjectInfo, error)
	getObjectFn           func(ctx context.Context, uri string) (*adt.ObjectInfo, error)
	syntaxCheckFn         func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error)
	runTestsFn            func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error)
	getTransportFn        func(ctx context.Context, user, status string) ([]adt.TransportRequest, error)
	addTransportFn        func(ctx context.Context, uri, transport string) error
	lockObjectFn          func(ctx context.Context, uri string) (string, error)
	unlockObjectFn        func(ctx context.Context, uri, lockHandle string) error
	prettyPrintFn         func(ctx context.Context, source string) (string, error)
	createObjectFn        func(ctx context.Context, objectType, name, pkg, desc, transport string) error
	deleteObjectFn        func(ctx context.Context, uri, lockHandle, transport string) error
	getCompletionsFn      func(ctx context.Context, uri, source string, line, column int) ([]adt.CompletionItem, error)
	createTransportFn     func(ctx context.Context, category, target, description, devClass string) (string, error)
	deleteTransportFn     func(ctx context.Context, transport string) error
	releaseTransportFn    func(ctx context.Context, transport string) error
	renameFn              func(ctx context.Context, uri, newName, transport string) (*adt.RenameResult, error)
	removeFromTransportFn func(ctx context.Context, taskNr, parentTr, pgmid, objType, objName, wbType, position string) error
	getTransportObjectsFn func(ctx context.Context, transport string) ([]adt.TransportObject, error)
	getTransportInfoFn    func(ctx context.Context, transport string) (*adt.TransportRequest, error)
	runQueryFn            func(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error)
	setTextElementsFn     func(ctx context.Context, uri string, symbols []adt.TextSymbol, selections []adt.SelectionText, lockHandle, transport string) error
	createTestIncludeFn   func(ctx context.Context, uri, lockHandle, transport string) error
}

func (m *mockClient) GetSource(ctx context.Context, uri string) (*adt.SourceResult, error) {
	if m.getSourceFn != nil {
		return m.getSourceFn(ctx, uri)
	}
	return &adt.SourceResult{}, nil
}
func (m *mockClient) GetClassDefinition(context.Context, string) (*adt.SourceResult, error) {
	return &adt.SourceResult{}, nil
}
func (m *mockClient) SetSource(ctx context.Context, uri, source, lockHandle, transport, etag string) (string, error) {
	if m.setSourceFn != nil {
		return m.setSourceFn(ctx, uri, source, lockHandle, transport, etag)
	}
	return "new-etag", nil
}
func (m *mockClient) GetIncludeSource(context.Context, string, string) (*adt.SourceResult, error) {
	return &adt.SourceResult{}, nil
}
func (m *mockClient) SetIncludeSource(context.Context, string, string, string, string, string, string) (string, error) {
	return "new-etag", nil
}
func (m *mockClient) CreateTestInclude(ctx context.Context, uri, lockHandle, transport string) error {
	if m.createTestIncludeFn != nil {
		return m.createTestIncludeFn(ctx, uri, lockHandle, transport)
	}
	return nil
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
func (m *mockClient) SearchPackages(ctx context.Context, q string, n int) ([]adt.ObjectInfo, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, q, adt.ObjectTypePackage, n)
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
func (m *mockClient) BatchSyntaxCheck(ctx context.Context, uris []string) []adt.ObjectSyntaxResult {
	results := make([]adt.ObjectSyntaxResult, len(uris))
	for i, uri := range uris {
		msgs, err := m.SyntaxCheck(ctx, uri)
		if err != nil {
			results[i] = adt.ObjectSyntaxResult{ObjectURI: uri, Error: err.Error()}
		} else {
			results[i] = adt.ObjectSyntaxResult{ObjectURI: uri, Messages: msgs}
		}
	}
	return results
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
func (m *mockClient) CreateTransport(ctx context.Context, category, target, description, devClass string) (string, error) {
	if m.createTransportFn != nil {
		return m.createTransportFn(ctx, category, target, description, devClass)
	}
	return "DEVK999999", nil
}
func (m *mockClient) CreateTransportTask(context.Context, string, string, string) (string, error) {
	return "DEVK999998", nil
}
func (m *mockClient) DeleteTransport(ctx context.Context, transport string) error {
	if m.deleteTransportFn != nil {
		return m.deleteTransportFn(ctx, transport)
	}
	return nil
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
func (m *mockClient) RemoveFromTransport(ctx context.Context, taskNr, parentTr, pgmid, objType, objName, wbType, position string) error {
	if m.removeFromTransportFn != nil {
		return m.removeFromTransportFn(ctx, taskNr, parentTr, pgmid, objType, objName, wbType, position)
	}
	return nil
}
func (m *mockClient) GetTransportInfo(ctx context.Context, transport string) (*adt.TransportRequest, error) {
	if m.getTransportInfoFn != nil {
		return m.getTransportInfoFn(ctx, transport)
	}
	return nil, nil
}
func (m *mockClient) GetTransportObjects(ctx context.Context, transport string) ([]adt.TransportObject, error) {
	if m.getTransportObjectsFn != nil {
		return m.getTransportObjectsFn(ctx, transport)
	}
	return nil, nil
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
func (m *mockClient) RunQuery(ctx context.Context, sql string, maxRows int) (*adt.QueryResult, error) {
	if m.runQueryFn != nil {
		return m.runQueryFn(ctx, sql, maxRows)
	}
	return &adt.QueryResult{}, nil
}
func (m *mockClient) ReleaseTransport(ctx context.Context, transport string) error {
	if m.releaseTransportFn != nil {
		return m.releaseTransportFn(ctx, transport)
	}
	return nil
}
func (m *mockClient) ReleaseTransportWithTasks(context.Context, string) error {
	return nil
}

// ReleaseTransportVerified mirrors adtler's real composition (release, then
// confirm via a status read) so it routes through the same releaseTransportFn
// and getTransportInfoFn hooks the tests configure.
func (m *mockClient) ReleaseTransportVerified(ctx context.Context, transport string, includeTasks bool) (*adt.ReleaseResult, error) {
	var err error
	if includeTasks {
		err = m.ReleaseTransportWithTasks(ctx, transport)
	} else {
		err = m.ReleaseTransport(ctx, transport)
	}
	if err != nil {
		return nil, err
	}
	if info, infoErr := m.GetTransportInfo(ctx, transport); infoErr == nil && info != nil && info.Status == "D" {
		return &adt.ReleaseResult{Transport: transport, Released: false}, nil
	}
	return &adt.ReleaseResult{Transport: transport, Released: true}, nil
}
func (m *mockClient) GetTransportTasks(context.Context, string) ([]string, error) {
	return nil, nil
}
func (m *mockClient) GetABAPDoc(context.Context, string) (string, error) { return "", nil }
func (m *mockClient) GetTextElements(context.Context, string) (*adt.TextElements, error) {
	return &adt.TextElements{}, nil
}
func (m *mockClient) SetTextElements(ctx context.Context, uri string, symbols []adt.TextSymbol, selections []adt.SelectionText, lockHandle, transport string) error {
	if m.setTextElementsFn != nil {
		return m.setTextElementsFn(ctx, uri, symbols, selections, lockHandle, transport)
	}
	return nil
}
func (m *mockClient) GetMessageClass(context.Context, string) (*adt.MessageClassInfo, error) {
	return &adt.MessageClassInfo{}, nil
}
func (m *mockClient) SearchMessages(context.Context, string, int) ([]adt.MessageSearchResult, error) {
	return nil, nil
}
func (m *mockClient) SetMessages(context.Context, string, string, []adt.Message) error { return nil }
func (m *mockClient) NavigateToDefinition(context.Context, string, string) (string, error) {
	return "", nil
}
func (m *mockClient) Rename(ctx context.Context, uri, newName, transport string) (*adt.RenameResult, error) {
	if m.renameFn != nil {
		return m.renameFn(ctx, uri, newName, transport)
	}
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
func (m *mockClient) GetEnhancementSpot(context.Context, string) (*adt.EnhancementSpotInfo, error) {
	return &adt.EnhancementSpotInfo{}, nil
}
func (m *mockClient) GetEnhancementImplementation(context.Context, string) (*adt.BAdIImplementationInfo, error) {
	return &adt.BAdIImplementationInfo{}, nil
}
func (m *mockClient) SetEnhancementImplementation(context.Context, string, string, string, string, string) error {
	return nil
}
func (m *mockClient) ListShortDumps(context.Context, string, string, string) ([]adt.ShortDumpHeader, error) {
	return nil, nil
}
func (m *mockClient) GetShortDumps(context.Context, string, string, string) ([]adt.ShortDump, error) {
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
	tools.RegisterAllWithLockMap(s, client, selector, lockMap, tools.ParseToolGroups([]string{"all"}), nil, nil)
	return s
}

func newTestServerWithLockMap(client adt.Client, lockMap *adt.LockMap) *server.MCPServer {
	return newTestServerWithSelector(client, &mockSelector{}, lockMap)
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

func TestGetSourceBatch(t *testing.T) {
	callCount := 0
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			callCount++
			// Two-line body: one newline per successful result.
			return &adt.SourceResult{Source: "REPORT " + uri + ".\n", ETag: `"etag-` + uri + `"`}, nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []any{
			"/sap/bc/adt/programs/programs/ZA",
			"/sap/bc/adt/programs/programs/ZB",
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}

	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response: %v", err)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total: got %v", resp["total"])
	}
	if resp["succeeded"] != float64(2) {
		t.Errorf("succeeded: got %v", resp["succeeded"])
	}
	if resp["failed"] != float64(0) {
		t.Errorf("failed: got %v", resp["failed"])
	}
	// 2 successful results × 1 newline each = 2.
	if resp["total_lines"] != float64(2) {
		t.Errorf("total_lines: got %v, want 2", resp["total_lines"])
	}
}

func TestGetSourceBatchEmpty(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			t.Fatalf("mock should not be called for empty batch, got %q", uri)
			return nil, nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []any{},
	})

	if result.IsError {
		t.Fatalf("unexpected error result")
	}

	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response: %v", err)
	}
	for _, field := range []string{"total", "succeeded", "failed", "total_lines"} {
		if resp[field] != float64(0) {
			t.Errorf("%s: got %v, want 0", field, resp[field])
		}
	}
}

func TestGetSourceBatchSummaryCounters(t *testing.T) {
	const missingURI = "/sap/bc/adt/programs/programs/ZMISSING"
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			if uri == missingURI {
				return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
			}
			// Three-line body: two newlines per successful result.
			return &adt.SourceResult{
				Source: "REPORT " + uri + ".\nWRITE 'hi'.\n",
				ETag:   `"etag"`,
			}, nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": []any{
			"/sap/bc/adt/programs/programs/ZA",
			missingURI,
			"/sap/bc/adt/programs/programs/ZB",
		},
	})

	if result.IsError {
		t.Fatalf("unexpected error result")
	}

	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response: %v", err)
	}
	if resp["total"] != float64(3) {
		t.Errorf("total: got %v, want 3", resp["total"])
	}
	if resp["succeeded"] != float64(2) {
		t.Errorf("succeeded: got %v, want 2", resp["succeeded"])
	}
	if resp["failed"] != float64(1) {
		t.Errorf("failed: got %v, want 1", resp["failed"])
	}
	// 2 successful results × 2 newlines each = 4. Failed result contributes 0.
	if resp["total_lines"] != float64(4) {
		t.Errorf("total_lines: got %v, want 4", resp["total_lines"])
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
			return []adt.TransportRequest{{Number: testTransportNum, Status: "D"}}, nil
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

func TestGetObjectInfoBatch(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			return &adt.ObjectInfo{Name: "ZOBJ", Type: "PROG/P"}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_object_info", map[string]interface{}{
		"object_uri": []any{"/sap/bc/adt/programs/programs/ZA", "/sap/bc/adt/programs/programs/ZB"},
	})
	if result.IsError {
		t.Fatalf("unexpected error")
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total: got %v", resp["total"])
	}
}

func TestObjectExistsBatch(t *testing.T) {
	mock := &mockClient{
		getObjectFn: func(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
			if uri == testObjectURIFail {
				return nil, &adt.ADTError{StatusCode: 404, Message: "Not found"}
			}
			return &adt.ObjectInfo{Name: "ZOK", Type: "PROG/P"}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "object_exists", map[string]interface{}{
		"object_uri": []any{testObjectURIOK, testObjectURIFail},
	})
	if result.IsError {
		t.Fatalf("unexpected error")
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if resp["found"] != float64(1) {
		t.Errorf("found: got %v", resp["found"])
	}
	if resp["missing"] != float64(1) {
		t.Errorf("missing: got %v", resp["missing"])
	}
}

func TestWhereUsedBatch(t *testing.T) {
	mock := &mockClient{
		whereUsedFn: func(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
			return []adt.ObjectInfo{{Name: "ZCALLER", Type: "PROG/P"}}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "where_used", map[string]interface{}{
		"object_uri": []any{"/sap/bc/adt/programs/programs/ZA", "/sap/bc/adt/programs/programs/ZB"},
	})
	if result.IsError {
		t.Fatalf("unexpected error")
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total: got %v", resp["total"])
	}
	if resp["total_references"] != float64(2) {
		t.Errorf("total_references: got %v", resp["total_references"])
	}
}

func TestSyntaxCheckBatch(t *testing.T) {
	mock := &mockClient{
		syntaxCheckFn: func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "syntax_check", map[string]interface{}{
		"object_uri": []any{"/sap/bc/adt/programs/programs/ZA", "/sap/bc/adt/programs/programs/ZB"},
	})
	if result.IsError {
		t.Fatalf("unexpected error")
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total: got %v", resp["total"])
	}
	if resp["clean"] != float64(2) {
		t.Errorf("clean: got %v", resp["clean"])
	}
}

func TestRunUnitTestsBatch(t *testing.T) {
	mock := &mockClient{
		runTestsFn: func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
			return &adt.TestResult{Passed: 3, Failed: 0}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "run_unit_tests", map[string]interface{}{
		"object_uri": []any{"/sap/bc/adt/programs/programs/ZA", "/sap/bc/adt/programs/programs/ZB"},
	})
	if result.IsError {
		t.Fatalf("unexpected error")
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if resp["total_objects"] != float64(2) {
		t.Errorf("total_objects: got %v", resp["total_objects"])
	}
	if resp["total_passed"] != float64(6) {
		t.Errorf("total_passed: got %v", resp["total_passed"])
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
		"transport":  testTransportNum,
	})
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if gotURI != testObjectURI {
		t.Errorf("object_uri: got %q", gotURI)
	}
	if gotTransport != testTransportNum {
		t.Errorf("transport: got %q", gotTransport)
	}
}

// Regression for #380: get_include_source must surface a "missing
// parameter" error rather than forwarding an empty string to adtler,
// which produces the misleading 'unknown include type ""' message.
// An LLM that omits the parameter (or uses a wrong key like
// 'include_type') needs to know the *key* was wrong, not the *value*.
func TestGetIncludeSourceToolRejectsMissingInclude(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "get_include_source", map[string]interface{}{
		"object_uri": testObjectURI,
		// 'include' deliberately omitted
	})
	if !result.IsError {
		t.Fatal("expected IsError=true when 'include' is missing")
	}
	text := firstText(result)
	if !strings.Contains(text, "'include'") {
		t.Errorf("error message should name the missing parameter 'include'; got: %s", text)
	}
	if strings.Contains(text, "unknown include type") {
		t.Errorf("error message must not be the adtler 'unknown include type' message — that misleads callers into thinking they passed a bad value; got: %s", text)
	}
}

// Same guard, set side.
func TestSetIncludeSourceToolRejectsMissingInclude(t *testing.T) {
	mock := &mockClient{}
	s := newTestServer(mock)
	result := callTool(t, s, "set_include_source", map[string]interface{}{
		"object_uri": testObjectURI,
		"source":     "* hello",
		"etag":       "etag-1",
		// 'include' deliberately omitted
	})
	if !result.IsError {
		t.Fatal("expected IsError=true when 'include' is missing")
	}
	text := firstText(result)
	if !strings.Contains(text, "'include'") {
		t.Errorf("error message should name the missing parameter 'include'; got: %s", text)
	}
}

func TestCreateTestIncludeTool(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/ZCL_TEST"
	var gotURI, gotLH, gotTransport string
	mock := &mockClient{
		createTestIncludeFn: func(_ context.Context, uri, lh, transport string) error {
			gotURI, gotLH, gotTransport = uri, lh, transport
			return nil
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set(adt.LockKey("dev", classURI), testAutoHandle, "")
	s := newTestServerWithLockMap(mock, lockMap)

	result := callTool(t, s, "create_test_include", map[string]interface{}{
		"object_uri": classURI,
		"transport":  testTransportNum,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	var got struct {
		ClassURI string `json:"class_uri"`
		Created  bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(firstText(result)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !got.Created {
		t.Error("expected created=true")
	}
	if got.ClassURI != classURI {
		t.Errorf("class_uri: got %q, want %q", got.ClassURI, classURI)
	}
	if gotURI != classURI {
		t.Errorf("adtler uri: got %q, want %q", gotURI, classURI)
	}
	if gotLH != testAutoHandle {
		t.Errorf("lock handle from map: got %q, want %q", gotLH, testAutoHandle)
	}
	if gotTransport != testTransportNum {
		t.Errorf("transport: got %q, want %q", gotTransport, testTransportNum)
	}
}

func TestCreateTestIncludeToolExplicitLockHandle(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/ZCL_TEST"
	var gotLH string
	mock := &mockClient{
		createTestIncludeFn: func(_ context.Context, _, lh, _ string) error {
			gotLH = lh
			return nil
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set(adt.LockKey("dev", classURI), "map-handle", "")
	s := newTestServerWithLockMap(mock, lockMap)

	result := callTool(t, s, "create_test_include", map[string]interface{}{
		"object_uri":  classURI,
		"lock_handle": "explicit-handle",
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", firstText(result))
	}
	if gotLH != "explicit-handle" {
		t.Errorf("expected explicit handle to take precedence; got %q", gotLH)
	}
}

func TestCreateTestIncludeToolPropagatesAdtlerError(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/ZCL_TEST"
	mock := &mockClient{
		createTestIncludeFn: func(_ context.Context, _, _, _ string) error {
			return fmt.Errorf("SAP ADT error 500: ExceptionResourceSaveFailure")
		},
	}
	lockMap := adt.NewLockMap()
	lockMap.Set(adt.LockKey("dev", classURI), testAutoHandle, "")
	s := newTestServerWithLockMap(mock, lockMap)

	result := callTool(t, s, "create_test_include", map[string]interface{}{
		"object_uri": classURI,
	})

	if !result.IsError {
		t.Fatal("expected IsError=true when adtler returns an error")
	}
}

func TestCreateTestIncludeToolRejectsWhenNoLockTracked(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/ZCL_TEST"
	s := newTestServerWithLockMap(&mockClient{}, adt.NewLockMap())

	result := callTool(t, s, "create_test_include", map[string]interface{}{
		"object_uri": classURI,
		// no lock_handle, nothing in lock map
	})

	if !result.IsError {
		t.Fatal("expected IsError=true when no lock is tracked and no handle provided")
	}
	text := firstText(result)
	if !strings.Contains(text, "lock_object") {
		t.Errorf("error message should hint at lock_object; got: %s", text)
	}
}
