# RFC Debugger Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable stateful ABAP debugging (step/variables/stack) and program execution via RFC, using gorfc with the SAP NW RFC SDK.

**Architecture:** New `rfc/` package wraps gorfc behind a clean interface. `rfc/adtcaller/` translates existing ADT REST requests into `SADT_REST_RFC_ENDPOINT` calls. `DebugSession` gets an optional RFC caller for stateful operations. Build tag `rfc` makes the SDK dependency optional.

**Tech Stack:** gorfc (`github.com/sap/gorfc`), SAP NW RFC SDK (C library, user-installed), cgo

---

### Task 1: RFC client interface and gorfc wrapper

**Files:**
- Create: `rfc/client.go`
- Create: `rfc/gorfc.go` (build tag `//go:build rfc`)
- Create: `rfc/stub.go` (build tag `//go:build !rfc`)
- Test: `rfc/client_test.go`

- [ ] **Step 1: Write the interface and stub**

`rfc/client.go`:
```go
// Package rfc provides a minimal RFC client interface for SAP systems.
package rfc

// Client is a minimal RFC interface. Only what the debugger needs.
type Client interface {
	// Call invokes an RFC function module with the given parameters.
	// Parameters and results use map[string]interface{} with gorfc conventions:
	// strings, []byte for XSTRING/RAWSTRING, nested maps for structures,
	// []interface{} for tables.
	Call(funcName string, params map[string]interface{}) (map[string]interface{}, error)
	// Close closes the RFC connection.
	Close() error
	// Alive returns true if the connection is open.
	Alive() bool
}
```

`rfc/stub.go`:
```go
//go:build !rfc

package rfc

import "fmt"

// NewClient returns an error when built without the rfc build tag.
func NewClient(host, sysnr, client, user, password, lang string) (Client, error) {
	return nil, fmt.Errorf("RFC support not available: rebuild with -tags rfc (requires SAP NW RFC SDK)")
}
```

- [ ] **Step 2: Write the gorfc implementation**

`rfc/gorfc.go`:
```go
//go:build rfc

package rfc

import "github.com/sap/gorfc/gorfc"

type gorFCClient struct {
	conn *gorfc.Connection
}

// NewClient creates an RFC connection using gorfc.
func NewClient(host, sysnr, client, user, password, lang string) (Client, error) {
	conn, err := gorfc.ConnectionFromParams(gorfc.ConnectionParameters{
		"ashost": host,
		"sysnr":  sysnr,
		"client": client,
		"user":   user,
		"passwd": password,
		"lang":   lang,
	})
	if err != nil {
		return nil, fmt.Errorf("rfc connect: %w", err)
	}
	return &gorFCClient{conn: conn}, nil
}

func (c *gorFCClient) Call(funcName string, params map[string]interface{}) (map[string]interface{}, error) {
	return c.conn.Call(funcName, params)
}

func (c *gorFCClient) Close() error {
	return c.conn.Close()
}

func (c *gorFCClient) Alive() bool {
	return c.conn.Alive()
}
```

- [ ] **Step 3: Write test for stub (no build tag)**

`rfc/client_test.go`:
```go
//go:build !rfc

package rfc_test

import (
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/rfc"
)

func TestNewClient_Stub(t *testing.T) {
	_, err := rfc.NewClient("host", "00", "100", "user", "pass", "EN")
	if err == nil {
		t.Fatal("expected error from stub, got nil")
	}
	if !strings.Contains(err.Error(), "RFC support not available") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 4: Run test to verify stub works**

Run: `go test ./rfc/ -v`
Expected: PASS — stub returns error

- [ ] **Step 5: Verify gorfc build compiles (if SDK available)**

Run: `go build -tags rfc ./rfc/`
Expected: compiles if SAP NW RFC SDK is installed, build error if not (expected on CI)

- [ ] **Step 6: Commit**

```
git add rfc/
git commit -m "feat(#68): add rfc package with Client interface and gorfc wrapper"
```

---

### Task 2: ADT-over-RFC caller

**Files:**
- Create: `rfc/adtcaller/caller.go`
- Test: `rfc/adtcaller/caller_test.go`

- [ ] **Step 1: Write the failing test**

`rfc/adtcaller/caller_test.go`:
```go
package adtcaller_test

import (
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/rfc/adtcaller"
)

// mockRFC implements rfc.Client for testing.
type mockRFC struct {
	lastFunc   string
	lastParams map[string]interface{}
	result     map[string]interface{}
	err        error
}

func (m *mockRFC) Call(funcName string, params map[string]interface{}) (map[string]interface{}, error) {
	m.lastFunc = funcName
	m.lastParams = params
	return m.result, m.err
}
func (m *mockRFC) Close() error { return nil }
func (m *mockRFC) Alive() bool  { return true }

func TestDoRequest_BuildsCorrectRFCParams(t *testing.T) {
	mock := &mockRFC{
		result: map[string]interface{}{
			"RESPONSE": map[string]interface{}{
				"STATUS_LINE": map[string]interface{}{
					"STATUS_CODE":   "200",
					"REASON_PHRASE": "OK",
				},
				"HEADER_FIELDS": []interface{}{},
				"MESSAGE_BODY":  []byte("<xml>ok</xml>"),
			},
		},
	}

	caller := adtcaller.New(mock)
	resp, err := caller.DoRequest("POST", "/sap/bc/adt/debugger/actions?action=stepInto",
		map[string]string{"Accept": "application/xml"}, nil)

	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if mock.lastFunc != "SADT_REST_RFC_ENDPOINT" {
		t.Errorf("func: got %q, want SADT_REST_RFC_ENDPOINT", mock.lastFunc)
	}

	// Verify request structure
	req, ok := mock.lastParams["REQUEST"].(map[string]interface{})
	if !ok {
		t.Fatal("REQUEST param missing or wrong type")
	}
	reqLine, ok := req["REQUEST_LINE"].(map[string]interface{})
	if !ok {
		t.Fatal("REQUEST_LINE missing")
	}
	if reqLine["METHOD"] != "POST" {
		t.Errorf("method: got %v, want POST", reqLine["METHOD"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./rfc/adtcaller/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write the implementation**

`rfc/adtcaller/caller.go`:
```go
// Package adtcaller translates ADT REST requests into SADT_REST_RFC_ENDPOINT calls.
package adtcaller

import (
	"fmt"
	"strconv"

	"github.com/Hochfrequenz/mcp-server-abap/rfc"
)

// Response holds the parsed RFC response.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// Caller wraps an RFC client to make ADT REST requests via SADT_REST_RFC_ENDPOINT.
type Caller struct {
	rfc rfc.Client
}

// New creates a Caller from an RFC client.
func New(c rfc.Client) *Caller {
	return &Caller{rfc: c}
}

// DoRequest sends an ADT REST request through the RFC tunnel.
func (c *Caller) DoRequest(method, uri string, headers map[string]string, body []byte) (*Response, error) {
	// Build TIHTTPNVP table (name-value pairs)
	headerFields := make([]interface{}, 0, len(headers))
	for k, v := range headers {
		headerFields = append(headerFields, map[string]interface{}{
			"NAME": k, "VALUE": v,
		})
	}

	var messageBody []byte
	if body != nil {
		messageBody = body
	}

	params := map[string]interface{}{
		"REQUEST": map[string]interface{}{
			"REQUEST_LINE": map[string]interface{}{
				"METHOD":  method,
				"URI":     uri,
				"VERSION": "HTTP/1.1",
			},
			"HEADER_FIELDS": headerFields,
			"MESSAGE_BODY":  messageBody,
		},
	}

	result, err := c.rfc.Call("SADT_REST_RFC_ENDPOINT", params)
	if err != nil {
		return nil, fmt.Errorf("RFC call: %w", err)
	}

	return parseResponse(result)
}

func parseResponse(result map[string]interface{}) (*Response, error) {
	respMap, ok := result["RESPONSE"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("RESPONSE not found in RFC result")
	}

	statusLine, _ := respMap["STATUS_LINE"].(map[string]interface{})
	codeStr, _ := statusLine["STATUS_CODE"].(string)
	code, _ := strconv.Atoi(codeStr)

	headers := make(map[string]string)
	if hf, ok := respMap["HEADER_FIELDS"].([]interface{}); ok {
		for _, h := range hf {
			if hm, ok := h.(map[string]interface{}); ok {
				name, _ := hm["NAME"].(string)
				value, _ := hm["VALUE"].(string)
				if name != "" {
					headers[name] = value
				}
			}
		}
	}

	var body []byte
	if b, ok := respMap["MESSAGE_BODY"].([]byte); ok {
		body = b
	}

	return &Response{StatusCode: code, Headers: headers, Body: body}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rfc/adtcaller/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add rfc/adtcaller/
git commit -m "feat(#68): add adtcaller package to tunnel ADT REST via RFC"
```

---

### Task 3: Wire RFC into DebugSession

**Files:**
- Modify: `adt/debugger.go`
- Test: `adt/debugger_test.go`

- [ ] **Step 1: Write failing test for RFC-backed Step**

Add to `adt/debugger_test.go`:
```go
func TestDebugSession_Step_UsesRFC(t *testing.T) {
	// Setup: DebugSession with mock RFC caller
	// Call Step("stepInto")
	// Verify it uses RFC caller, not HTTP
}
```

The exact test will depend on how we expose the RFC caller on DebugSession. The key contract: if `rfcCaller` is set, Step/GetVariable/GetStack use it instead of HTTP.

- [ ] **Step 2: Add rfcCaller field to DebugSession**

In `adt/debugger.go`:
```go
type DebugSession struct {
	client      *httpClient
	rfcCaller   *adtcaller.Caller // optional, for stateful debug ops
	user        string
	terminalID  string
	ideID       string
	debuggeeID  string
	breakpoints map[string]string
}
```

Add `SetRFCCaller(c *adtcaller.Caller)` method.

- [ ] **Step 3: Modify Step/GetVariable/GetStack to use RFC when available**

For each method, add a check at the top:
```go
func (d *DebugSession) Step(ctx context.Context, action string) ([]byte, error) {
	if d.rfcCaller != nil {
		resp, err := d.rfcCaller.DoRequest("POST",
			fmt.Sprintf("/sap/bc/adt/debugger/actions?action=%s", action),
			map[string]string{"Accept": "application/xml"}, nil)
		if err != nil {
			return nil, fmt.Errorf("Step (RFC): %w", err)
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("Step (RFC): HTTP %d: %s", resp.StatusCode, string(resp.Body))
		}
		return resp.Body, nil
	}
	// existing HTTP path...
}
```

Same pattern for `GetVariable`, `GetStack`, `Attach`.

- [ ] **Step 4: Run tests**

Run: `go test ./adt/ -v -run TestDebug`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add adt/debugger.go adt/debugger_test.go
git commit -m "feat(#68): wire RFC caller into DebugSession for stateful debug ops"
```

---

### Task 4: Integration test with real SAP system

**Files:**
- Create: `rfc/rfc_integration_test.go`
- Create: `adt/debugger_rfc_integration_test.go`

- [ ] **Step 1: Write RFC connection integration test**

`rfc/rfc_integration_test.go` (build tags: `integration,rfc`):
```go
//go:build integration && rfc

package rfc_test

func TestRFCConnection_Integration(t *testing.T) {
	c, err := rfc.NewClient(os.Getenv("SAP_INTEGRATION_HOST"), "00",
		os.Getenv("SAP_INTEGRATION_CLIENT"),
		os.Getenv("SAP_INTEGRATION_USER"),
		os.Getenv("SAP_INTEGRATION_PASSWORD"), "EN")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()
	if !c.Alive() {
		t.Fatal("connection not alive")
	}
}
```

- [ ] **Step 2: Write SADT_REST_RFC_ENDPOINT integration test**

Test a simple ADT request through RFC (e.g., GET discovery):
```go
func TestADTCallerViaRFC_Integration(t *testing.T) {
	// Connect via RFC
	// Create adtcaller.Caller
	// DoRequest("GET", "/sap/bc/adt/discovery", headers, nil)
	// Verify 200 response with XML body
}
```

- [ ] **Step 3: Write stateful debug integration test**

Full flow: Set breakpoint (HTTP) → Start listener (HTTP) → Run unit tests (HTTP) → Listener returns → Attach via RFC → Step via RFC → GetVariable via RFC → GetStack via RFC.

- [ ] **Step 4: Run integration tests**

Run: `go test -tags 'integration rfc' -v ./rfc/ ./adt/ -run RFC`
Expected: PASS (requires SAP NW RFC SDK + VPN)

- [ ] **Step 5: Commit**

```
git add rfc/rfc_integration_test.go adt/debugger_rfc_integration_test.go
git commit -m "test(#68): add RFC integration tests for debugger"
```

---

### Task 5: Program execution via RFC (stretch goal)

**Files:**
- Modify: `adt/debugger.go`
- Test: `adt/debugger_test.go`

This task depends on verifying that gorfc can maintain a stateful session where `SADT_START_TCODE` can be called. This may require using the connection in a specific way or discovering additional RFC function modules.

- [ ] **Step 1: Research — can we call SADT_START_TCODE via RFC?**

`SADT_START_TCODE` is a GUI transaction, not an RFC-enabled FM. We need to find an alternative:
- Option A: Use `SADT_REST_RFC_ENDPOINT` to call classrun-like endpoints
- Option B: Find/create an RFC FM that does `SUBMIT <report> AND RETURN`
- Option C: Call unit test runner via RFC (already works via HTTP, would also work via RFC tunnel)

- [ ] **Step 2: Implement chosen approach**

TBD based on research in step 1.

- [ ] **Step 3: Integration test**

Full debug flow: breakpoint → listener → **execute report** → attach → step → variables.

- [ ] **Step 4: Commit**

```
git commit -m "feat(#68): add program execution via RFC"
```
