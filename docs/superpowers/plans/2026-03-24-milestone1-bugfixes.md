# Milestone 1 Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 4 remaining broken ADT endpoints in Milestone 1 (#10 BrowsePackage, #12 SyntaxCheck, #13 GetObjectInfo, #14 GetTransportRequests) so they work against a real SAP system.

**Architecture:** Each fix follows the same pattern: update HTTP method/headers to match SAP's requirements (documented in `docs/superpowers/evidence/endpoint-verification.md`), write a new XML parser for the actual response format, and update tests to use realistic SAP response XML. All changes are in the `adt/` package.

**Tech Stack:** Go, `encoding/xml`, `net/http/httptest`

---

### Task 1: Fix GetTransportRequests Accept header (#14)

**Files:**
- Modify: `adt/transport.go:38` (Accept header)
- Modify: `adt/transport_test.go` (verify Accept header, use realistic XML)

- [ ] **Step 1: Update test to verify Accept header and use realistic SAP XML**

Update `TestGetTransportRequests` in `adt/transport_test.go` to:
1. Assert the request's Accept header is `application/vnd.sap.adt.transportorganizertree.v1+xml`
2. Use the real SAP response XML format with `tm:` namespace prefix

```go
func TestGetTransportRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/cts/transportrequests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.sap.adt.transportorganizertree.v1+xml" {
			t.Errorf("Accept header: got %q", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.transportorganizertree.v1+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<tm:root adtcore:name="DEVELOPER"
  xmlns:tm="http://www.sap.com/cts/adt/tm"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <tm:workbenchRequests>
    <tm:workbenchRequest tm:number="DEVK900123" tm:owner="DEVELOPER" tm:shortDescription="Feature transport" tm:status="D"/>
  </tm:workbenchRequests>
</tm:root>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	transports, err := client.GetTransportRequests(context.Background(), "", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(transports))
	}
	if transports[0].Number != "DEVK900123" {
		t.Errorf("number: got %q", transports[0].Number)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./adt/ -run TestGetTransportRequests -v`
Expected: FAIL — Accept header assertion fails (current code sends `application/xml`)

- [ ] **Step 3: Fix the Accept header in transport.go**

In `adt/transport.go` line 38, change:
```go
resp, err := c.doRead(ctx, path, map[string]string{"Accept": contentTypeXML})
```
to:
```go
resp, err := c.doRead(ctx, path, map[string]string{
    "Accept": "application/vnd.sap.adt.transportorganizertree.v1+xml",
})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./adt/ -run TestGetTransportRequests -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add adt/transport.go adt/transport_test.go
git commit -m "fix(#14): GetTransportRequests use correct Accept header

SAP requires application/vnd.sap.adt.transportorganizertree.v1+xml,
generic application/xml returns 406 Not Acceptable."
```

---

### Task 2: Fix BrowsePackage (#10)

**Files:**
- Modify: `adt/repository.go:11-28` (HTTP method, Accept, parser)
- Modify: `adt/repository_test.go:13-42` (realistic SAP response, method assertion)

- [ ] **Step 1: Write updated test with realistic SAP XML and method/header assertions**

Replace `TestBrowsePackage` in `adt/repository_test.go`:

```go
func TestBrowsePackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/repository/nodestructure" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.sap.as+xml" {
			t.Errorf("Accept header: got %q", accept)
		}
		q := r.URL.Query()
		if q.Get("parent_name") != "STUN" {
			t.Errorf("parent_name: got %q", q.Get("parent_name"))
		}
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <TREE_CONTENT>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>PROG/P</OBJECT_TYPE>
          <OBJECT_NAME>RSPARAM</OBJECT_NAME>
          <TECH_NAME>RSPARAM</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/programs/programs/RSPARAM</OBJECT_URI>
          <DESCRIPTION>Display SAP Profile Parameters</DESCRIPTION>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>DEVC/K</OBJECT_TYPE>
          <OBJECT_NAME>STUN_COMMON</OBJECT_NAME>
          <TECH_NAME>STUN_COMMON</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/packages/stun_common</OBJECT_URI>
          <DESCRIPTION>Common Monitoring</DESCRIPTION>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
      </TREE_CONTENT>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	results, err := client.BrowsePackage(context.Background(), "STUN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "RSPARAM" {
		t.Errorf("name[0]: got %q", results[0].Name)
	}
	if results[0].URI != "/sap/bc/adt/programs/programs/RSPARAM" {
		t.Errorf("uri[0]: got %q", results[0].URI)
	}
	if results[0].Type != "PROG/P" {
		t.Errorf("type[0]: got %q", results[0].Type)
	}
	if results[0].Description != "Display SAP Profile Parameters" {
		t.Errorf("description[0]: got %q", results[0].Description)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./adt/ -run TestBrowsePackage -v`
Expected: FAIL — method is GET (not POST), Accept header wrong, XML parsing fails

- [ ] **Step 3: Rewrite BrowsePackage in repository.go**

Add `"net/http"` to the imports in `repository.go`. Replace the `BrowsePackage` function and add a new parser:

```go
func (c *httpClient) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("parent_type", "DEVC/K")
	params.Set("parent_name", packageName)
	path := "/sap/bc/adt/repository/nodestructure?" + params.Encode()

	resp, err := c.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{
			"Accept":       "application/vnd.sap.as+xml",
			"Content-Type": contentTypeXML,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("BrowsePackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseNodeStructure(data)
}
```

Add the new XML types and parser (in `adt/repository.go`):

```go
type xmlNodeStructure struct {
	XMLName xml.Name          `xml:"abap"`
	Nodes   []xmlNodeStructureNode `xml:"values>DATA>TREE_CONTENT>SEU_ADT_REPOSITORY_OBJ_NODE"`
}

type xmlNodeStructureNode struct {
	ObjectType  string `xml:"OBJECT_TYPE"`
	ObjectName  string `xml:"OBJECT_NAME"`
	ObjectURI   string `xml:"OBJECT_URI"`
	Description string `xml:"DESCRIPTION"`
}

func parseNodeStructure(data []byte) ([]ObjectInfo, error) {
	var ns xmlNodeStructure
	if err := xml.Unmarshal(data, &ns); err != nil {
		return nil, fmt.Errorf("parsing node structure: %w", err)
	}
	result := make([]ObjectInfo, 0, len(ns.Nodes))
	for _, n := range ns.Nodes {
		if n.ObjectName == "" {
			continue // skip empty root node
		}
		result = append(result, ObjectInfo{
			URI:         n.ObjectURI,
			Type:        n.ObjectType,
			Name:        n.ObjectName,
			Description: n.Description,
		})
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./adt/ -run TestBrowsePackage -v`
Expected: PASS

- [ ] **Step 5: Run all tests to check for regressions**

Run: `go test ./adt/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add adt/repository.go adt/repository_test.go
git commit -m "fix(#10): BrowsePackage use POST with correct Accept and XML parser

SAP nodestructure endpoint requires POST and returns asx:abap format
with SEU_ADT_REPOSITORY_OBJ_NODE elements, not objectReferences."
```

---

### Task 3: Fix SyntaxCheck (#12)

**Files:**
- Modify: `adt/syntaxcheck.go` (headers, request body, response parser)
- Modify: `adt/syntaxcheck_test.go` (realistic SAP XML, header/body assertions)

- [ ] **Step 1: Write updated tests with realistic SAP XML**

Replace the content of `adt/syntaxcheck_test.go`:

```go
package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestSyntaxCheckWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/checkruns" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify headers
		ct := r.Header.Get("Content-Type")
		if ct != "application/vnd.sap.adt.checkobjects+xml" {
			t.Errorf("Content-Type: got %q", ct)
		}
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.sap.adt.checkmessages+xml" {
			t.Errorf("Accept: got %q", accept)
		}
		// Verify request body contains checkObjectList with URI
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "checkObjectList") {
			t.Errorf("body missing checkObjectList: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "/sap/bc/adt/programs/programs/ZTEST") {
			t.Errorf("body missing object URI: %s", bodyStr)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun"
    chkrun:triggeringUri="/sap/bc/adt/programs/programs/ZTEST"
    chkrun:status="processed" chkrun:statusText="Syntax check performed">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/programs/ZTEST/source/main#start=42,5"
        chkrun:type="E" chkrun:shortText="Field &quot;FOO&quot; is unknown."/>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "E" {
		t.Errorf("type: got %q", msgs[0].Type)
	}
	if msgs[0].Text != `Field "FOO" is unknown.` {
		t.Errorf("text: got %q", msgs[0].Text)
	}
	if msgs[0].Line != 42 {
		t.Errorf("line: got %d", msgs[0].Line)
	}
	if msgs[0].Column != 5 {
		t.Errorf("column: got %d", msgs[0].Column)
	}
}

func TestSyntaxCheckClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun"
    chkrun:triggeringUri="/sap/bc/adt/programs/programs/ZTEST"
    chkrun:status="processed" chkrun:statusText="Object ZTEST has been checked"/>
</chkrun:checkRunReports>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for clean check, got %d", len(msgs))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./adt/ -run TestSyntaxCheck -v`
Expected: FAIL — wrong headers sent, wrong XML response parsed

- [ ] **Step 3: Rewrite SyntaxCheck in syntaxcheck.go**

Replace the entire content of `adt/syntaxcheck.go`:

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type xmlCheckRunReports struct {
	XMLName xml.Name             `xml:"checkRunReports"`
	Reports []xmlCheckRunReport  `xml:"checkReport"`
}

type xmlCheckRunReport struct {
	Reporter     string             `xml:"reporter,attr"`
	TriggerURI   string             `xml:"triggeringUri,attr"`
	Status       string             `xml:"status,attr"`
	StatusText   string             `xml:"statusText,attr"`
	Messages     []xmlCheckMessage  `xml:"checkMessageList>checkMessage"`
}

type xmlCheckMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText string `xml:"shortText,attr"`
}

func (c *httpClient) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" `+
		`xmlns:adtcore="http://www.sap.com/adt/core">`+
		`<chkrun:checkObject adtcore:uri="%s" chkrun:version="active"/>`+
		`</chkrun:checkObjectList>`, objectURI)

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/checkruns",
		strings.NewReader(body),
		map[string]string{
			"Content-Type": "application/vnd.sap.adt.checkobjects+xml",
			"Accept":       "application/vnd.sap.adt.checkmessages+xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("SyntaxCheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var reports xmlCheckRunReports
	xml.Unmarshal(data, &reports) //nolint:errcheck

	var result []SyntaxMessage
	for _, report := range reports.Reports {
		for _, m := range report.Messages {
			line, col := parseMessagePosition(m.URI)
			result = append(result, SyntaxMessage{
				Type:   m.Type,
				Text:   m.ShortText,
				Line:   line,
				Column: col,
			})
		}
	}
	return result, nil
}

// parseMessagePosition extracts line and column from a checkMessage URI fragment.
// Format: ".../source/main#start=42,5" → line=42, col=5
func parseMessagePosition(uri string) (int, int) {
	idx := strings.Index(uri, "#start=")
	if idx < 0 {
		return 0, 0
	}
	parts := strings.SplitN(uri[idx+7:], ",", 2)
	line, _ := strconv.Atoi(parts[0])
	col := 0
	if len(parts) == 2 {
		col, _ = strconv.Atoi(parts[1])
	}
	return line, col
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./adt/ -run TestSyntaxCheck -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./adt/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add adt/syntaxcheck.go adt/syntaxcheck_test.go
git commit -m "fix(#12): SyntaxCheck use correct headers, XML body, and response parser

SAP checkruns endpoint requires Content-Type checkobjects+xml, Accept
checkmessages+xml, and an XML body with chkrun:checkObjectList. Response
uses chkrun:checkRunReports with checkMessage elements containing line/col
in URI fragments."
```

---

### Task 4: Fix GetObjectInfo (#13)

**Files:**
- Modify: `adt/repository.go:30-48` (Accept header mapping, generic parser)
- Modify: `adt/repository_test.go:44-75` (tests for multiple object types)

- [ ] **Step 1: Write tests for multiple object types**

Add new tests to `adt/repository_test.go`:

```go
func TestGetObjectInfoProgram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/programs/programs/RSPARAM" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/vnd.sap.adt.programs.programs") {
			t.Errorf("Accept header missing program type: %q", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.programs.programs.v2+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<program:abapProgram
  adtcore:name="RSPARAM" adtcore:type="PROG/P"
  adtcore:description="Display SAP Profile Parameters"
  xmlns:program="http://www.sap.com/adt/programs/programs"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/stun" adtcore:name="STUN"/>
</program:abapProgram>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/programs/programs/RSPARAM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "RSPARAM" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "PROG/P" {
		t.Errorf("type: got %q", info.Type)
	}
	if info.Description != "Display SAP Profile Parameters" {
		t.Errorf("description: got %q", info.Description)
	}
	if info.PackageName != "STUN" {
		t.Errorf("packageName: got %q", info.PackageName)
	}
}

func TestGetObjectInfoClass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/oo/classes/ZCL_EXAMPLE" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/vnd.sap.adt.oo.classes") {
			t.Errorf("Accept header missing class type: %q", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.oo.classes.v4+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<class:abapClass adtcore:name="ZCL_EXAMPLE" adtcore:type="CLAS/OC"
  adtcore:description="Example Class"
  xmlns:class="http://www.sap.com/adt/oo/classes"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/ztest" adtcore:name="ZTEST"/>
</class:abapClass>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/oo/classes/ZCL_EXAMPLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZCL_EXAMPLE" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "CLAS/OC" {
		t.Errorf("type: got %q", info.Type)
	}
	if info.PackageName != "ZTEST" {
		t.Errorf("packageName: got %q", info.PackageName)
	}
}

func TestGetObjectInfoInterface(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/oo/interfaces/ZIF_EXAMPLE" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.oo.interfaces.v5+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<intf:abapInterface adtcore:name="ZIF_EXAMPLE" adtcore:type="INTF/OI"
  adtcore:description="Example Interface"
  xmlns:intf="http://www.sap.com/adt/oo/interfaces"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/ztest" adtcore:name="ZTEST"/>
</intf:abapInterface>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/oo/interfaces/ZIF_EXAMPLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZIF_EXAMPLE" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "INTF/OI" {
		t.Errorf("type: got %q", info.Type)
	}
}
```

Note: Remove the old `TestGetObjectInfo` function.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./adt/ -run TestGetObjectInfo -v`
Expected: FAIL — wrong Accept header, parser fails on program/class/interface root elements

- [ ] **Step 3: Rewrite GetObjectInfo in repository.go**

Replace the `GetObjectInfo` function and add the Accept header mapping and generic parser. Add the `strings` import if not already present.

```go
// objectTypeAcceptHeaders maps ADT URI path prefixes to their required Accept headers.
var objectTypeAcceptHeaders = map[string]string{
	"/sap/bc/adt/programs/programs":  "application/vnd.sap.adt.programs.programs.v2+xml",
	"/sap/bc/adt/programs/includes":  "application/vnd.sap.adt.programs.includes.v2+xml",
	"/sap/bc/adt/oo/classes":         "application/vnd.sap.adt.oo.classes.v4+xml",
	"/sap/bc/adt/oo/interfaces":      "application/vnd.sap.adt.oo.interfaces.v5+xml",
	"/sap/bc/adt/functions/groups":    "application/vnd.sap.adt.functions.groups.v3+xml",
	"/sap/bc/adt/ddic/dataelements":  "application/vnd.sap.adt.dataelements.v2+xml",
	"/sap/bc/adt/ddic/domains":       "application/vnd.sap.adt.domains.v2+xml",
	"/sap/bc/adt/ddic/tables":        "application/vnd.sap.adt.tables.v2+xml",
	"/sap/bc/adt/ddic/tabletypes":    "application/vnd.sap.adt.tabletype.v1+xml",
	"/sap/bc/adt/ddic/typegroups":    "application/vnd.sap.adt.ddic.typegroups.v2+xml",
	"/sap/bc/adt/ddic/ddl/sources":   "application/vnd.sap.adt.ddlSource+xml",
	"/sap/bc/adt/ddic/ddlx/sources":  "application/vnd.sap.adt.ddic.ddlx.v1+xml",
	"/sap/bc/adt/ddic/ddla/sources":  "application/vnd.sap.adt.ddic.ddla.v1+xml",
	"/sap/bc/adt/ddic/srvd/sources":  "application/vnd.sap.adt.ddic.srvd.v1+xml",
	"/sap/bc/adt/packages":           "application/vnd.sap.adt.packages.v2+xml",
	"/sap/bc/adt/bo/behaviordefinitions": "application/vnd.sap.adt.blues.v1+xml",
	"/sap/bc/adt/acm/dcl/sources":    "application/vnd.sap.adt.dclSource+xml",
}

// acceptHeaderForURI returns the best Accept header for a given object URI.
// Note: nested sub-URIs (e.g. /functions/groups/GRP/fmodules/FM) will match
// the parent prefix. This is acceptable since SAP typically serves the parent
// content type for metadata requests on sub-objects.
func acceptHeaderForURI(objectURI string) string {
	// Try longest prefix match
	bestPrefix := ""
	bestAccept := ""
	for prefix, accept := range objectTypeAcceptHeaders {
		if strings.HasPrefix(objectURI, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestAccept = accept
		}
	}
	if bestAccept != "" {
		return bestAccept + ", application/xml"
	}
	return "application/xml"
}

func (c *httpClient) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	accept := acceptHeaderForURI(objectURI)
	resp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": accept})
	if err != nil {
		return nil, fmt.Errorf("GetObjectInfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseGenericObjectInfo(data)
}

// parseGenericObjectInfo extracts ObjectInfo from any ADT object XML response.
// All ADT object types share adtcore:name, adtcore:type, adtcore:description
// attributes on the root element and an <adtcore:packageRef> child element.
func parseGenericObjectInfo(data []byte) (*ObjectInfo, error) {
	// Use a struct that captures adtcore:* attributes on any root element
	var obj struct {
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		PackageRef  struct {
			Name string `xml:"name,attr"`
		} `xml:"packageRef"`
	}
	if err := xml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("GetObjectInfo parsing: %w", err)
	}
	return &ObjectInfo{
		Name:        obj.Name,
		Type:        obj.Type,
		Description: obj.Description,
		PackageName: obj.PackageRef.Name,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./adt/ -run TestGetObjectInfo -v`
Expected: PASS for all 3 tests (Program, Class, Interface)

- [ ] **Step 5: Run all tests including tools package**

Run: `go test ./... -v`
Expected: All PASS. The tools package tests may use the old test patterns — if any fail, update them to match.

- [ ] **Step 6: Run linter**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 7: Commit**

```bash
git add adt/repository.go adt/repository_test.go
git commit -m "fix(#13): GetObjectInfo use object-type-specific Accept headers

SAP returns 406 for generic application/xml. Add a mapping from URI
prefix to vendor-specific Accept header covering programs, classes,
interfaces, function groups, DDIC objects, CDS, packages, and more.
Use a generic parser that extracts adtcore:* attributes from any root
element."
```

---

### Task 5: Update registry_test.go stubs

The `allEndpointsHandler` in `adt/registry_test.go` returns fake XML responses for all endpoints. After the fixes above, three stubs return XML that won't parse under the new parsers.

**Files:**
- Modify: `adt/registry_test.go:106-174` (update stubs)

- [ ] **Step 1: Update allEndpointsHandler stubs**

In `adt/registry_test.go`, update the constants and cases:

1. Replace `emptyObjectRefs` constant with:
```go
emptyNodeStructure = `<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><TREE_CONTENT></TREE_CONTENT></DATA></asx:values></asx:abap>`
```

2. Replace `emptyMessages` constant with:
```go
emptyCheckReports = `<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"/>`
```

3. Split the `nodestructure` case from `search` and fix the activation path:
```go
case path == "/sap/bc/adt/repository/informationsystem/search":
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(emptyObjectRefs))
case path == "/sap/bc/adt/repository/nodestructure":
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(emptyNodeStructure))
```

4. Update checkruns case:
```go
case path == "/sap/bc/adt/checkruns":
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(emptyCheckReports))
```

5. Update activation path constant:
```go
activatePath = "/sap/bc/adt/activation"
```

6. Update activation stub response:
```go
case path == activatePath:
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(`<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist"><chkl:properties checkExecuted="false" activationExecuted="false" generationExecuted="true"/></chkl:messages>`))
```

7. Update GetObjectInfo stub to return realistic XML:
```go
case path == "/sap/bc/adt/programs/programs/ZTEST":
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(`<program:abapProgram adtcore:name="ZTEST" adtcore:type="PROG/P" adtcore:description="" xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:packageRef adtcore:name=""/></program:abapProgram>`))
```

- [ ] **Step 2: Run all tests**

Run: `go test ./adt/ -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add adt/registry_test.go
git commit -m "test: update registry stubs for realistic SAP response formats"
```

---

### Task 6: Run full test suite and verify

- [ ] **Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Run linter**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 3: Close GitHub issues**

```bash
gh issue close 10 --reason completed --comment "Fixed: BrowsePackage now uses POST with correct Accept header and asx:abap XML parser."
gh issue close 12 --reason completed --comment "Fixed: SyntaxCheck now uses correct Content-Type/Accept headers, sends proper XML body, and parses chkrun:checkRunReports response."
gh issue close 13 --reason completed --comment "Fixed: GetObjectInfo now uses object-type-specific Accept headers with a generic adtcore attribute parser."
gh issue close 14 --reason completed --comment "Fixed: GetTransportRequests now uses correct Accept header."
```
