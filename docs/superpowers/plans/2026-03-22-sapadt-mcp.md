# SAP ADT MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go MCP server that exposes SAP ABAP Development Tools (ADT) REST API as MCP tools for AI assistants.

**Architecture:** Thin proxy — `adt/` package makes HTTP calls to SAP ADT REST API, `tools/` package maps MCP tool calls to `adt/` functions via the `adt.Client` interface. All SAP XML parsing happens in `adt/`, all MCP protocol handling in `tools/`.

**Tech Stack:** Go 1.22+, `github.com/mark3labs/mcp-go`, Go standard library (`encoding/xml`, `net/http`, `os`, `gopkg.in/yaml.v3`), `httptest` for testing, GoReleaser + GitHub Actions for distribution.

---

## File Map

| File | Responsibility |
|------|---------------|
| `go.mod` | Module definition, dependencies |
| `.gitignore` | Exclude `config.yaml`, binaries |
| `config.yaml.example` | Example config without credentials |
| `Makefile` | `build`, `build-all`, `test`, `lint`, `release` targets |
| `.goreleaser.yaml` | Multi-platform release config |
| `.github/workflows/release.yml` | GitHub Actions release on tag push |
| `config/config.go` | Load config from YAML + env vars (env overrides file) |
| `adt/types.go` | All shared Go types: `SourceResult`, `ObjectInfo`, `ActivationResult`, `ActivationMessage`, `SyntaxMessage`, `TestResult`, `TestCase`, `TransportRequest` |
| `adt/client.go` | `Client` interface + `httpClient` struct: Basic Auth, CSRF token fetch/cache/retry, session re-auth on 401, XML error parsing |
| `adt/source.go` | `GetSource` (GET `{uri}/source/main` → source + ETag), `SetSource` (PUT with `If-Match`) |
| `adt/activate.go` | `ActivateObject` (POST `/sap/bc/adt/activation/activate`) |
| `adt/search.go` | `SearchObjects` (GET search endpoint), `WhereUsed` (GET usageReferences) |
| `adt/repository.go` | `BrowsePackage` (GET nodestructure), `GetObjectInfo` (GET object URI) |
| `adt/syntaxcheck.go` | `SyntaxCheck` (POST /sap/bc/adt/checkruns) |
| `adt/unittest.go` | `RunUnitTests` (POST /sap/bc/adt/abapunit/testruns) |
| `adt/transport.go` | `GetTransportRequests` (GET /sap/bc/adt/cts/transportrequests), `AddToTransport` (POST) |
| `tools/register.go` | `RegisterAll(s *mcp.Server, client adt.Client)` — wires all tool handlers |
| `tools/source.go` | `get_source`, `set_source` MCP handlers |
| `tools/activate.go` | `activate_object` MCP handler |
| `tools/search.go` | `search_objects`, `where_used` MCP handlers |
| `tools/repository.go` | `browse_package`, `get_object_info` MCP handlers |
| `tools/syntaxcheck.go` | `syntax_check` MCP handler |
| `tools/unittest.go` | `run_unit_tests` MCP handler |
| `tools/transport.go` | `get_transport_requests`, `add_to_transport` MCP handlers |
| `main.go` | Entry point: load config, create ADT client, register tools, start MCP server |
| `testdata/*.xml` | SAP ADT XML response fixtures used in `adt/` tests |

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `config.yaml.example`
- Create: `Makefile`
- Create: `.goreleaser.yaml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Initialize Go module**

```bash
cd /c/Users/dachner/dev/sapadt.mcp
go mod init github.com/dachner/sapadt-mcp
```

Expected: `go.mod` created with `module github.com/dachner/sapadt-mcp` and Go version.

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/mark3labs/mcp-go@latest
go get gopkg.in/yaml.v3@latest
```

- [ ] **Step 3: Create .gitignore**

```
config.yaml
sapadt-mcp
sapadt-mcp.exe
dist/
```

- [ ] **Step 4: Create config.yaml.example**

```yaml
sap:
  host: "https://your-sap-system:8000"
  client: "100"
  user: "DEVELOPER"
  password: ""
  tls_skip_verify: false
```

- [ ] **Step 5: Create Makefile**

```makefile
BINARY=sapadt-mcp
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

.PHONY: build build-all test lint release

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-all:
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-windows-amd64.exe .
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 .

test:
	go test ./... -v

lint:
	golangci-lint run ./...

release:
	goreleaser release --clean
```

- [ ] **Step 6: Create .goreleaser.yaml**

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: sapadt-mcp
    binary: sapadt-mcp
    ldflags:
      - -X main.version={{.Version}}
    goos:
      - linux
      - windows
      - darwin
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    name_template: "{{ .ProjectName }}-{{ .Version }}-{{ .Os }}-{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

release:
  github:
    owner: dachner
    name: sapadt-mcp
```

- [ ] **Step 7: Create .github/workflows/release.yml**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run tests
        run: go test ./...

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 8: Create testdata directory and README**

```bash
mkdir -p testdata
touch testdata/.gitkeep
```

- [ ] **Step 9: Commit scaffold**

```bash
git add .
git commit -m "feat: project scaffold with build tooling"
```

---

## Task 2: Config Package

**Files:**
- Create: `config/config.go`
- Create: `config/config_test.go`

- [ ] **Step 1: Write failing tests**

Create `config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dachner/sapadt-mcp/config"
)

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte(`
sap:
  host: "https://sap.example.com:8000"
  client: "100"
  user: "TESTUSER"
  password: "testpass"
  tls_skip_verify: false
`), 0600)

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SAP.Host != "https://sap.example.com:8000" {
		t.Errorf("host: got %q", cfg.SAP.Host)
	}
	if cfg.SAP.User != "TESTUSER" {
		t.Errorf("user: got %q", cfg.SAP.User)
	}
}

func TestEnvVarsOverrideFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte(`
sap:
  host: "https://sap.example.com:8000"
  user: "FILEUSER"
  password: "filepass"
  client: "100"
`), 0600)

	t.Setenv("SAP_HOST", "https://override.example.com:8001")
	t.Setenv("SAP_USER", "ENVUSER")
	t.Setenv("SAP_PASSWORD", "envpass")
	t.Setenv("SAP_CLIENT", "200")

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SAP.Host != "https://override.example.com:8001" {
		t.Errorf("host: got %q, want override", cfg.SAP.Host)
	}
	if cfg.SAP.User != "ENVUSER" {
		t.Errorf("user: got %q", cfg.SAP.User)
	}
	if cfg.SAP.Client != "200" {
		t.Errorf("client: got %q", cfg.SAP.Client)
	}
}

func TestTLSSkipVerifyEnv(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, []byte(`sap:
  host: "https://sap.example.com"
  client: "100"
  user: "U"
  password: "P"
`), 0600)

	t.Setenv("SAP_TLS_SKIP_VERIFY", "true")

	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.SAP.TLSSkipVerify {
		t.Error("expected TLSSkipVerify=true from env")
	}
}

func TestMissingFileReturnsError(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./config/... -v
```
Expected: compile error (`config` package doesn't exist yet)

- [ ] **Step 3: Implement config package**

Create `config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type SAPConfig struct {
	Host          string `yaml:"host"`
	Client        string `yaml:"client"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
}

type Config struct {
	SAP SAPConfig `yaml:"sap"`
}

// Load reads config from the given YAML file, then applies environment variable overrides.
// Relative paths are resolved from the process working directory.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SAP_HOST"); v != "" {
		cfg.SAP.Host = v
	}
	if v := os.Getenv("SAP_CLIENT"); v != "" {
		cfg.SAP.Client = v
	}
	if v := os.Getenv("SAP_USER"); v != "" {
		cfg.SAP.User = v
	}
	if v := os.Getenv("SAP_PASSWORD"); v != "" {
		cfg.SAP.Password = v
	}
	if v := os.Getenv("SAP_TLS_SKIP_VERIFY"); strings.EqualFold(v, "true") {
		cfg.SAP.TLSSkipVerify = true
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./config/... -v
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add config/ go.mod go.sum
git commit -m "feat: config package with YAML + env var loading"
```

---

## Task 3: ADT Types and Client Foundation

**Files:**
- Create: `adt/types.go`
- Create: `adt/client.go`
- Create: `adt/client_test.go`
- Create: `testdata/csrf_product.xml`
- Create: `testdata/error_response.xml`

- [ ] **Step 1: Create shared types in adt/types.go**

```go
package adt

// SourceResult holds ABAP source code and its ETag for optimistic locking.
type SourceResult struct {
	Source string
	ETag   string
}

// ObjectInfo describes an ABAP repository object.
type ObjectInfo struct {
	URI         string
	Type        string
	Name        string
	Description string
	PackageName string
}

// ActivationMessage is a per-object message from an activation response.
type ActivationMessage struct {
	ObjectURI string
	Type      string // "E" error, "W" warning, "I" info
	Text      string
}

// ActivationResult is returned by ActivateObject.
type ActivationResult struct {
	Success  bool
	Messages []ActivationMessage
}

// SyntaxMessage is a single message from a syntax check.
type SyntaxMessage struct {
	Type   string // "E", "W", "I"
	Text   string
	Line   int
	Column int
}

// TestCase represents a single ABAP unit test method result.
type TestCase struct {
	Name          string
	ExecutionTime float64
	Passed        bool
	Messages      []string
}

// TestResult is returned by RunUnitTests.
type TestResult struct {
	Passed    int
	Failed    int
	Errors    int
	TestCases []TestCase
}

// TransportRequest describes a CTS transport request.
type TransportRequest struct {
	Number      string
	Owner       string
	Description string
	Status      string // "D" = modifiable, "L" = released
}

// ADTError is returned when SAP ADT responds with an error status.
type ADTError struct {
	StatusCode int
	Message    string
}

func (e *ADTError) Error() string {
	return fmt.Sprintf("SAP ADT error %d: %s", e.StatusCode, e.Message)
}
```

Note: Add `"fmt"` import to `adt/types.go`.

- [ ] **Step 2: Create testdata XML fixtures**

Create `testdata/csrf_product.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:product xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="AS ABAP" adtcore:version="7.57"/>
```

Create `testdata/error_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<exc:ExceptionText xmlns:exc="http://www.sap.com/adt/exception">
  <exc:message>Object not found</exc:message>
</exc:ExceptionText>
```

- [ ] **Step 3: Write failing tests for client**

Create `adt/client_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func newTestConfig(host string) *config.Config {
	return &config.Config{
		SAP: config.SAPConfig{
			Host:     host,
			Client:   "100",
			User:     "TESTUSER",
			Password: "testpass",
		},
	}
}

func TestCSRFTokenFetchedOnFirstMutate(t *testing.T) {
	var csrfFetched atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sap/bc/adt/compatibility/product":
			csrfFetched.Store(true)
			w.Header().Set("X-CSRF-Token", "test-csrf-token")
			w.Header().Set("Set-Cookie", "sap-session=abc123; Path=/")
			w.WriteHeader(http.StatusOK)
		default:
			// Verify CSRF token is present on mutating request
			if r.Header.Get("X-CSRF-Token") != "test-csrf-token" {
				t.Errorf("expected CSRF token in request, got %q", r.Header.Get("X-CSRF-Token"))
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	// SetSource is a PUT (mutating) — triggers CSRF fetch
	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !csrfFetched.Load() {
		t.Error("expected CSRF preflight request to /sap/bc/adt/compatibility/product")
	}
}

func TestCSRFTokenRefreshedOn403(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "refreshed-token")
			w.WriteHeader(http.StatusOK)
			return
		}
		count := callCount.Add(1)
		if count == 1 {
			// First attempt: return 403 (expired token)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// Second attempt: succeed
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (initial + retry), got %d", callCount.Load())
	}
}

func TestReauthOn401(t *testing.T) {
	var authAttempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		attempt := authAttempts.Add(1)
		if attempt == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error after re-auth: %v", err)
	}
}

func TestADTErrorParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<?xml version="1.0"?><exc:ExceptionText xmlns:exc="http://www.sap.com/adt/exception"><exc:message>Object not found</exc:message></exc:ExceptionText>`))
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
	if err == nil {
		t.Fatal("expected error")
	}
	adtErr, ok := err.(*adt.ADTError)
	if !ok {
		t.Fatalf("expected *adt.ADTError, got %T: %v", err, err)
	}
	if adtErr.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", adtErr.StatusCode)
	}
	if adtErr.Message != "Object not found" {
		t.Errorf("message: got %q", adtErr.Message)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
go test ./adt/... -v -run TestCSRF
```
Expected: compile error (adt package doesn't exist)

- [ ] **Step 5: Implement adt/client.go**

```go
package adt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dachner/sapadt-mcp/config"
)

// Client defines all SAP ADT operations exposed as MCP tools.
type Client interface {
	GetSource(ctx context.Context, objectURI string) (*SourceResult, error)
	SetSource(ctx context.Context, objectURI, source, etag string) error
	ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error)
	SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error)
	WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error)
	BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error)
	GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error)
	SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error)
	RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error)
	GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error)
	AddToTransport(ctx context.Context, objectURI, transport string) error
}

type httpClient struct {
	cfg        *config.Config
	http       *http.Client
	mu         sync.Mutex
	csrfToken  string
	sessionCookies []*http.Cookie
}

// NewClient creates a new ADT HTTP client configured from cfg.
func NewClient(cfg *config.Config) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.SAP.TLSSkipVerify, //nolint:gosec
		},
	}
	return &httpClient{
		cfg: cfg,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// fetchCSRFToken performs the CSRF preflight GET and caches the token and session cookies.
func (c *httpClient) fetchCSRFToken(ctx context.Context) error {
	url := c.cfg.SAP.Host + "/sap/bc/adt/compatibility/product"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.setBasicAuth(req)
	req.Header.Set("X-CSRF-Token", "Fetch")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("CSRF fetch: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	c.csrfToken = resp.Header.Get("X-CSRF-Token")
	c.sessionCookies = resp.Cookies()
	return nil
}

func (c *httpClient) setBasicAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.SAP.User, c.cfg.SAP.Password)
	if c.cfg.SAP.Client != "" {
		req.Header.Set("sap-client", c.cfg.SAP.Client)
	}
}

func (c *httpClient) applySession(req *http.Request) {
	for _, cookie := range c.sessionCookies {
		req.AddCookie(cookie)
	}
}

// doRead performs a GET request (no CSRF required).
func (c *httpClient) doRead(ctx context.Context, path string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.SAP.Host+path, nil)
	if err != nil {
		return nil, err
	}
	c.setBasicAuth(req)
	c.mu.Lock()
	c.applySession(req)
	c.mu.Unlock()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.mu.Lock()
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		c.mu.Unlock()
		// retry
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.SAP.Host+path, nil)
		c.setBasicAuth(req2)
		c.mu.Lock()
		c.applySession(req2)
		c.mu.Unlock()
		for k, v := range headers {
			req2.Header.Set(k, v)
		}
		return c.http.Do(req2)
	}
	return resp, nil
}

// doMutate performs a POST/PUT/DELETE request with CSRF token handling and retry logic.
// IMPORTANT: body is read into a []byte buffer before the first attempt so it can be
// replayed on CSRF-token-expired (403) or session-expired (401) retries.
func (c *httpClient) doMutate(ctx context.Context, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	// Buffer the body so retries can replay it.
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("buffering request body: %w", err)
		}
	}
	// Replace the body reader with a factory that creates fresh readers from the buffer.
	newBody := func() io.Reader {
		if bodyBytes == nil { return nil }
		return bytes.NewReader(bodyBytes)
	}
	c.mu.Lock()
	if c.csrfToken == "" {
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
	}
	token := c.csrfToken
	cookies := c.sessionCookies
	c.mu.Unlock()

	resp, err := c.execMutate(ctx, method, path, newBody(), headers, token, cookies)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		// CSRF token expired — re-fetch and retry once
		resp.Body.Close()
		c.mu.Lock()
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		token = c.csrfToken
		cookies = c.sessionCookies
		c.mu.Unlock()
		return c.execMutate(ctx, method, path, newBody(), headers, token, cookies)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.mu.Lock()
		if err := c.fetchCSRFToken(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		token = c.csrfToken
		cookies = c.sessionCookies
		c.mu.Unlock()
		return c.execMutate(ctx, method, path, newBody(), headers, token, cookies)
	}

	return resp, nil
}

func (c *httpClient) execMutate(ctx context.Context, method, path string, body io.Reader, headers map[string]string, csrfToken string, cookies []*http.Cookie) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.SAP.Host+path, body)
	if err != nil {
		return nil, err
	}
	c.setBasicAuth(req)
	req.Header.Set("X-CSRF-Token", csrfToken)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

// parseADTError reads an XML error response body and returns an *ADTError.
func parseADTError(statusCode int, body io.Reader) error {
	data, _ := io.ReadAll(body)
	var xmlErr struct {
		XMLName xml.Name `xml:"ExceptionText"`
		Message string   `xml:"message"`
	}
	if err := xml.Unmarshal(data, &xmlErr); err == nil && xmlErr.Message != "" {
		return &ADTError{StatusCode: statusCode, Message: xmlErr.Message}
	}
	return &ADTError{StatusCode: statusCode, Message: strings.TrimSpace(string(data))}
}

// checkResponse returns an *ADTError if the response status indicates failure.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return parseADTError(resp.StatusCode, resp.Body)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestCSRF|TestReauth|TestADTError"
```
Expected: all 4 client tests PASS

- [ ] **Step 7: Commit**

```bash
git add adt/ testdata/ go.mod go.sum
git commit -m "feat: adt types, client interface, CSRF/session handling"
```

---

## Task 4: Source Code Read/Write

**Files:**
- Create: `adt/source.go`
- Create: `adt/source_test.go`
- Create: `testdata/source_response.txt`

- [ ] **Step 1: Create testdata fixture**

Create `testdata/source_response.txt`:
```abap
REPORT ZTEST.
WRITE 'Hello World'.
```

- [ ] **Step 2: Write failing tests**

Create `adt/source_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestGetSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			w.Header().Set("ETag", `"etag-abc123"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("REPORT ZTEST.\nWRITE 'Hello'."))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	result, err := client.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != "REPORT ZTEST.\nWRITE 'Hello'." {
		t.Errorf("source: got %q", result.Source)
	}
	if result.ETag != `"etag-abc123"` {
		t.Errorf("etag: got %q", result.ETag)
	}
}

func TestSetSource(t *testing.T) {
	var gotMethod, gotIfMatch, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			gotMethod = r.Method
			gotIfMatch = r.Header.Get("If-Match")
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.\nNEW CODE.", `"etag-abc123"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method: got %q, want PUT", gotMethod)
	}
	if gotIfMatch != `"etag-abc123"` {
		t.Errorf("If-Match: got %q", gotIfMatch)
	}
	if gotBody != "REPORT ZTEST.\nNEW CODE." {
		t.Errorf("body: got %q", gotBody)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestGetSource|TestSetSource"
```
Expected: compile error or test failure (source.go doesn't exist)

- [ ] **Step 4: Implement adt/source.go**

```go
package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *httpClient) GetSource(ctx context.Context, objectURI string) (*SourceResult, error) {
	resp, err := c.doRead(ctx, objectURI+"/source/main", map[string]string{
		"Accept": "text/plain",
	})
	if err != nil {
		return nil, fmt.Errorf("GetSource: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetSource reading body: %w", err)
	}

	return &SourceResult{
		Source: string(body),
		ETag:   resp.Header.Get("ETag"),
	}, nil
}

func (c *httpClient) SetSource(ctx context.Context, objectURI, source, etag string) error {
	resp, err := c.doMutate(ctx, http.MethodPut, objectURI+"/source/main",
		strings.NewReader(source),
		map[string]string{
			"Content-Type": "plain/abap; charset=utf-8",
			"If-Match":     etag,
		},
	)
	if err != nil {
		return fmt.Errorf("SetSource: %w", err)
	}
	defer resp.Body.Close()
	return checkResponse(resp)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestGetSource|TestSetSource"
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add adt/source.go adt/source_test.go testdata/
git commit -m "feat: adt source read/write (GetSource, SetSource)"
```

---

## Task 5: Object Activation

**Files:**
- Create: `adt/activate.go`
- Create: `adt/activate_test.go`
- Create: `testdata/activation_success.xml`
- Create: `testdata/activation_error.xml`

- [ ] **Step 1: Create testdata fixtures**

Create `testdata/activation_success.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core"/>
```

Create `testdata/activation_error.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkl:message adtcore:uri="/sap/bc/adt/programs/programs/ZTEST" chkl:type="E">
    <chkl:shortTextElements>
      <chkl:shortText>Syntax error in line 5</chkl:shortText>
    </chkl:shortTextElements>
  </chkl:message>
</chkl:messages>
```

- [ ] **Step 2: Write failing tests**

Create `adt/activate_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestActivateObjectSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/activation/activate" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core"/>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result.Messages))
	}
}

func TestActivateObjectWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/activation/activate" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkl:message adtcore:uri="/sap/bc/adt/programs/programs/ZTEST" chkl:type="E">
    <chkl:shortTextElements><chkl:shortText>Syntax error in line 5</chkl:shortText></chkl:shortTextElements>
  </chkl:message>
</chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false when error messages present")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Type != "E" {
		t.Errorf("message type: got %q, want E", result.Messages[0].Type)
	}
	if result.Messages[0].Text != "Syntax error in line 5" {
		t.Errorf("message text: got %q", result.Messages[0].Text)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestActivate"
```

- [ ] **Step 4: Implement adt/activate.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// xmlActivationRequest is the XML body for the activation POST.
type xmlActivationRequest struct {
	XMLName xml.Name `xml:"adtcore:objectReferences"`
	NS      string   `xml:"xmlns:adtcore,attr"`
	Objects []xmlActivationObject
}

type xmlActivationObject struct {
	XMLName xml.Name `xml:"adtcore:objectReference"`
	URI     string   `xml:"adtcore:uri,attr"`
}

// xmlActivationMessages is the XML response from activation.
type xmlActivationMessages struct {
	XMLName  xml.Name               `xml:"messages"`
	Messages []xmlActivationMessage `xml:"message"`
}

type xmlActivationMessage struct {
	URI      string `xml:"uri,attr"`
	Type     string `xml:"type,attr"`
	ShortText struct {
		Text string `xml:"shortText"`
	} `xml:"shortTextElements"`
}

func (c *httpClient) ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error) {
	bodyXML, err := xml.Marshal(xmlActivationRequest{
		NS: "http://www.sap.com/adt/core",
		Objects: []xmlActivationObject{
			{URI: objectURI},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal activation request: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/activation/activate?method=activate&preauditRequested=true",
		strings.NewReader(xml.Header+string(bodyXML)),
		map[string]string{"Content-Type": "application/xml"},
	)
	if err != nil {
		return nil, fmt.Errorf("ActivateObject: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ActivateObject reading response: %w", err)
	}

	var msgs xmlActivationMessages
	xml.Unmarshal(data, &msgs) //nolint:errcheck // empty response is valid

	result := &ActivationResult{Success: true}
	for _, m := range msgs.Messages {
		msg := ActivationMessage{
			ObjectURI: m.URI,
			Type:      m.Type,
			Text:      m.ShortText.Text,
		}
		result.Messages = append(result.Messages, msg)
		if m.Type == "E" {
			result.Success = false
		}
	}
	return result, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestActivate"
```

- [ ] **Step 6: Commit**

```bash
git add adt/activate.go adt/activate_test.go testdata/
git commit -m "feat: adt object activation with per-object message parsing"
```

---

## Task 6: Search and Where-Used

**Files:**
- Create: `adt/search.go`
- Create: `adt/search_test.go`
- Create: `testdata/search_response.xml`
- Create: `testdata/whereused_response.xml`

- [ ] **Step 1: Create testdata fixtures**

Create `testdata/search_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference
    adtcore:uri="/sap/bc/adt/programs/programs/ZTEST_REPORT"
    adtcore:type="PROG/P"
    adtcore:name="ZTEST_REPORT"
    adtcore:description="Test Report"
    adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>
```

Create `testdata/whereused_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference
    adtcore:uri="/sap/bc/adt/programs/programs/ZCALLER"
    adtcore:type="PROG/P"
    adtcore:name="ZCALLER"
    adtcore:description="Caller Program"/>
</adtcore:objectReferences>
```

- [ ] **Step 2: Write failing tests**

Create `adt/search_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestSearchObjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/informationsystem/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("objectName") != "ZTEST*" {
			t.Errorf("objectName: got %q", q.Get("objectName"))
		}
		if q.Get("maxResults") != "10" {
			t.Errorf("maxResults: got %q", q.Get("maxResults"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZTEST_REPORT" adtcore:type="PROG/P" adtcore:name="ZTEST_REPORT" adtcore:description="Test Report" adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	results, err := client.SearchObjects(context.Background(), "ZTEST*", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "ZTEST_REPORT" {
		t.Errorf("name: got %q", results[0].Name)
	}
	if results[0].PackageName != "ZPACKAGE" {
		t.Errorf("package: got %q", results[0].PackageName)
	}
}

func TestWhereUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/informationsystem/usageReferences" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("adtObjectUri") == "" {
			t.Error("expected adtObjectUri parameter")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZCALLER" adtcore:type="PROG/P" adtcore:name="ZCALLER" adtcore:description="Caller"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	results, err := client.WhereUsed(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ZCALLER" {
		t.Errorf("unexpected results: %+v", results)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestSearch|TestWhereUsed"
```

- [ ] **Step 4: Implement adt/search.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
)

type xmlObjectReferences struct {
	XMLName    xml.Name           `xml:"objectReferences"`
	References []xmlObjectReference `xml:"objectReference"`
}

type xmlObjectReference struct {
	URI         string `xml:"uri,attr"`
	Type        string `xml:"type,attr"`
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	PackageName string `xml:"packageName,attr"`
}

func parseObjectReferences(data []byte) ([]ObjectInfo, error) {
	var refs xmlObjectReferences
	if err := xml.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("parsing object references: %w", err)
	}
	result := make([]ObjectInfo, len(refs.References))
	for i, r := range refs.References {
		result[i] = ObjectInfo{
			URI:         r.URI,
			Type:        r.Type,
			Name:        r.Name,
			Description: r.Description,
			PackageName: r.PackageName,
		}
	}
	return result, nil
}

func (c *httpClient) SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("objectName", query)
	if objectType != "" {
		params.Set("objectType", objectType)
	}
	if maxResults > 0 {
		params.Set("maxResults", strconv.Itoa(maxResults))
	}
	path := "/sap/bc/adt/repository/informationsystem/search?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("SearchObjects: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}

func (c *httpClient) WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("adtObjectUri", objectURI)
	path := "/sap/bc/adt/repository/informationsystem/usageReferences?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("WhereUsed: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestSearch|TestWhereUsed"
```

- [ ] **Step 6: Commit**

```bash
git add adt/search.go adt/search_test.go testdata/
git commit -m "feat: adt search objects and where-used"
```

---

## Task 7: Repository Browse and Object Info

**Files:**
- Create: `adt/repository.go`
- Create: `adt/repository_test.go`
- Create: `testdata/nodestructure_response.xml`
- Create: `testdata/objectinfo_response.xml`

- [ ] **Step 1: Create testdata fixtures**

Create `testdata/nodestructure_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT" adtcore:type="PROG/P" adtcore:name="ZREPORT" adtcore:description="My Report" adtcore:packageName="ZPACKAGE"/>
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/classes/classes/ZCL_FOO" adtcore:type="CLAS/OC" adtcore:name="ZCL_FOO" adtcore:description="My Class" adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>
```

Create `testdata/objectinfo_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReference xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT"
  adtcore:type="PROG/P"
  adtcore:name="ZREPORT"
  adtcore:description="My Report"
  adtcore:packageName="ZPACKAGE"/>
```

- [ ] **Step 2: Write failing tests**

Create `adt/repository_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestBrowsePackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/nodestructure" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("parent_name") != "ZPACKAGE" {
			t.Errorf("parent_name: got %q", q.Get("parent_name"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT" adtcore:type="PROG/P" adtcore:name="ZREPORT" adtcore:description="My Report" adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	results, err := client.BrowsePackage(context.Background(), "ZPACKAGE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ZREPORT" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestGetObjectInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/programs/programs/ZREPORT" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReference xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT"
  adtcore:type="PROG/P"
  adtcore:name="ZREPORT"
  adtcore:description="My Report"
  adtcore:packageName="ZPACKAGE"/>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/programs/programs/ZREPORT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZREPORT" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "PROG/P" {
		t.Errorf("type: got %q", info.Type)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestBrowse|TestGetObjectInfo"
```

- [ ] **Step 4: Implement adt/repository.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
)

func (c *httpClient) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("parent_type", "DEVC/K")
	params.Set("parent_name", packageName)
	path := "/sap/bc/adt/repository/nodestructure?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("BrowsePackage: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}

func (c *httpClient) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	resp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetObjectInfo: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var ref xmlObjectReference
	if err := xml.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("GetObjectInfo parsing: %w", err)
	}

	return &ObjectInfo{
		URI:         ref.URI,
		Type:        ref.Type,
		Name:        ref.Name,
		Description: ref.Description,
		PackageName: ref.PackageName,
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestBrowse|TestGetObjectInfo"
```

- [ ] **Step 6: Commit**

```bash
git add adt/repository.go adt/repository_test.go testdata/
git commit -m "feat: adt repository browse and object info"
```

---

## Task 8: Syntax Check

**Files:**
- Create: `adt/syntaxcheck.go`
- Create: `adt/syntaxcheck_test.go`
- Create: `testdata/syntaxcheck_response.xml`

- [ ] **Step 1: Create testdata fixture**

Create `testdata/syntaxcheck_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkl:message chkl:type="E" chkl:typeText="Error">
    <chkl:shortTextElements>
      <chkl:shortText>Field "FOO" is unknown.</chkl:shortText>
    </chkl:shortTextElements>
    <chkl:line>42</chkl:line>
    <chkl:column>5</chkl:column>
  </chkl:message>
</chkl:messages>
```

- [ ] **Step 2: Write failing tests**

Create `adt/syntaxcheck_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestSyntaxCheckWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/checkruns" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist">
  <chkl:message chkl:type="E" chkl:typeText="Error">
    <chkl:shortTextElements><chkl:shortText>Field "FOO" is unknown.</chkl:shortText></chkl:shortTextElements>
    <chkl:line>42</chkl:line>
    <chkl:column>5</chkl:column>
  </chkl:message>
</chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
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
	if msgs[0].Line != 42 {
		t.Errorf("line: got %d", msgs[0].Line)
	}
	if msgs[0].Column != 5 {
		t.Errorf("column: got %d", msgs[0].Column)
	}
}

func TestSyntaxCheckClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
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

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestSyntaxCheck"
```

- [ ] **Step 4: Implement adt/syntaxcheck.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type xmlCheckMessages struct {
	XMLName  xml.Name         `xml:"messages"`
	Messages []xmlCheckMessage `xml:"message"`
}

type xmlCheckMessage struct {
	Type     string `xml:"type,attr"`
	TypeText string `xml:"typeText,attr"`
	ShortText struct {
		Text string `xml:"shortText"`
	} `xml:"shortTextElements"`
	Line   int `xml:"line"`
	Column int `xml:"column"`
}

func (c *httpClient) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	params := url.Values{}
	params.Set("adtObjectUri", objectURI)

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/checkruns?"+params.Encode(),
		strings.NewReader(""),
		map[string]string{
			"Content-Type": "application/xml",
			"Accept":       "application/xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("SyntaxCheck: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var msgs xmlCheckMessages
	xml.Unmarshal(data, &msgs) //nolint:errcheck

	result := make([]SyntaxMessage, len(msgs.Messages))
	for i, m := range msgs.Messages {
		result[i] = SyntaxMessage{
			Type:   m.Type,
			Text:   m.ShortText.Text,
			Line:   m.Line,
			Column: m.Column,
		}
	}
	return result, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestSyntaxCheck"
```

- [ ] **Step 6: Commit**

```bash
git add adt/syntaxcheck.go adt/syntaxcheck_test.go testdata/
git commit -m "feat: adt syntax check"
```

---

## Task 9: ABAP Unit Tests

**Files:**
- Create: `adt/unittest.go`
- Create: `adt/unittest_test.go`
- Create: `testdata/unittest_response.xml`

- [ ] **Step 1: Create testdata fixture**

Create `testdata/unittest_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit" xmlns:adtcore="http://www.sap.com/adt/core">
  <program adtcore:uri="/sap/bc/adt/classes/classes/ZCL_TEST" adtcore:name="ZCL_TEST">
    <testClass adtcore:name="ZCL_TEST" aunit:testCount="2" aunit:errorCount="0" aunit:failureCount="0">
      <testMethod adtcore:name="TEST_PASS" aunit:executionTime="0.001">
        <alerts/>
      </testMethod>
      <testMethod adtcore:name="TEST_FAIL" aunit:executionTime="0.002">
        <alerts>
          <alert aunit:type="failedAssertion" aunit:severity="critical">
            <title>Assertion failed</title>
          </alert>
        </alerts>
      </testMethod>
    </testClass>
  </program>
</aunit:runResult>
```

- [ ] **Step 2: Write failing tests**

Create `adt/unittest_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestRunUnitTests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/abapunit/testruns" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit" xmlns:adtcore="http://www.sap.com/adt/core">
  <program adtcore:uri="/sap/bc/adt/classes/classes/ZCL_TEST" adtcore:name="ZCL_TEST">
    <testClass adtcore:name="ZCL_TEST" aunit:testCount="2" aunit:errorCount="0" aunit:failureCount="1">
      <testMethod adtcore:name="TEST_PASS" aunit:executionTime="0.001"><alerts/></testMethod>
      <testMethod adtcore:name="TEST_FAIL" aunit:executionTime="0.002">
        <alerts><alert aunit:type="failedAssertion" aunit:severity="critical"><title>Assertion failed</title></alert></alerts>
      </testMethod>
    </testClass>
  </program>
</aunit:runResult>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	result, err := client.RunUnitTests(context.Background(), "/sap/bc/adt/classes/classes/ZCL_TEST", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed != 1 {
		t.Errorf("passed: got %d, want 1", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("failed: got %d, want 1", result.Failed)
	}
	if len(result.TestCases) != 2 {
		t.Fatalf("expected 2 test cases, got %d", len(result.TestCases))
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestRunUnitTests"
```

- [ ] **Step 4: Implement adt/unittest.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type xmlUnitTestRunRequest struct {
	XMLName xml.Name `xml:"aunit:run"`
	NS      string   `xml:"xmlns:aunit,attr"`
	NSCore  string   `xml:"xmlns:adtcore,attr"`
	Timeout int      `xml:"adtcore:timeout,attr"`
	Objects []xmlUnitTestObject
}

type xmlUnitTestObject struct {
	XMLName xml.Name `xml:"adtcore:objectReference"`
	URI     string   `xml:"adtcore:uri,attr"`
}

type xmlRunResult struct {
	XMLName  xml.Name     `xml:"runResult"`
	Programs []xmlProgram `xml:"program"`
}

type xmlProgram struct {
	Classes []xmlTestClass `xml:"testClass"`
}

type xmlTestClass struct {
	Name         string          `xml:"name,attr"`
	FailureCount int             `xml:"failureCount,attr"`
	ErrorCount   int             `xml:"errorCount,attr"`
	Methods      []xmlTestMethod `xml:"testMethod"`
}

type xmlTestMethod struct {
	Name          string     `xml:"name,attr"`
	ExecutionTime float64    `xml:"executionTime,attr"`
	Alerts        []xmlAlert `xml:"alerts>alert"`
}

type xmlAlert struct {
	Type  string `xml:"type,attr"`
	Title string `xml:"title"`
}

func (c *httpClient) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	body, err := xml.Marshal(xmlUnitTestRunRequest{
		NS:      "http://www.sap.com/adt/aunit",
		NSCore:  "http://www.sap.com/adt/core",
		Timeout: timeoutSeconds * 1000, // SAP expects milliseconds
		Objects: []xmlUnitTestObject{{URI: objectURI}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal unit test request: %w", err)
	}

	resp, err := c.doMutate(reqCtx, http.MethodPost,
		"/sap/bc/adt/abapunit/testruns",
		strings.NewReader(xml.Header+string(body)),
		map[string]string{
			"Content-Type": "application/xml",
			"Accept":       "application/xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("RunUnitTests: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var runResult xmlRunResult
	xml.Unmarshal(data, &runResult) //nolint:errcheck

	result := &TestResult{}
	for _, prog := range runResult.Programs {
		for _, class := range prog.Classes {
			for _, method := range class.Methods {
				tc := TestCase{
					Name:          method.Name,
					ExecutionTime: method.ExecutionTime,
					Passed:        len(method.Alerts) == 0,
				}
				for _, alert := range method.Alerts {
					tc.Messages = append(tc.Messages, alert.Title)
				}
				result.TestCases = append(result.TestCases, tc)
				if tc.Passed {
					result.Passed++
				} else {
					result.Failed++
				}
			}
			result.Errors += class.ErrorCount
		}
	}
	return result, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestRunUnitTests"
```

- [ ] **Step 6: Commit**

```bash
git add adt/unittest.go adt/unittest_test.go testdata/
git commit -m "feat: adt ABAP unit test runner"
```

---

## Task 10: Transport Requests

**Files:**
- Create: `adt/transport.go`
- Create: `adt/transport_test.go`
- Create: `testdata/transport_list_response.xml`

- [ ] **Step 1: Create testdata fixture**

Create `testdata/transport_list_response.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/transport">
  <tm:workbenchRequests>
    <tm:workbenchRequest tm:number="DEVK900123" tm:owner="DEVELOPER" tm:shortDescription="Feature transport" tm:status="D"/>
    <tm:workbenchRequest tm:number="DEVK900124" tm:owner="DEVELOPER" tm:shortDescription="Released transport" tm:status="L"/>
  </tm:workbenchRequests>
</tm:root>
```

- [ ] **Step 2: Write failing tests**

Create `adt/transport_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestGetTransportRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/cts/transportrequests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?>
<tm:root xmlns:tm="http://www.sap.com/cts/transport">
  <tm:workbenchRequests>
    <tm:workbenchRequest tm:number="DEVK900123" tm:owner="DEVELOPER" tm:shortDescription="Feature transport" tm:status="D"/>
  </tm:workbenchRequests>
</tm:root>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
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
	if transports[0].Status != "D" {
		t.Errorf("status: got %q", transports[0].Status)
	}
}

func TestAddToTransport(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	err := client.AddToTransport(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "DEVK900123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "/sap/bc/adt/cts/transportrequests/DEVK900123/abaptransportcomponents"
	if gotPath != expected {
		t.Errorf("path: got %q, want %q", gotPath, expected)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./adt/... -v -run "TestTransport|TestAddToTransport"
```

- [ ] **Step 4: Implement adt/transport.go**

```go
package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type xmlTransportRoot struct {
	XMLName           xml.Name              `xml:"root"`
	WorkbenchRequests []xmlTransportRequest `xml:"workbenchRequests>workbenchRequest"`
}

type xmlTransportRequest struct {
	Number      string `xml:"number,attr"`
	Owner       string `xml:"owner,attr"`
	Description string `xml:"shortDescription,attr"`
	Status      string `xml:"status,attr"`
}

func (c *httpClient) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	params := url.Values{}
	if user != "" {
		params.Set("user", user)
	}
	if status != "" {
		params.Set("status", status)
	}
	path := "/sap/bc/adt/cts/transportrequests"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportRequests: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var root xmlTransportRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("GetTransportRequests parsing: %w", err)
	}

	result := make([]TransportRequest, len(root.WorkbenchRequests))
	for i, r := range root.WorkbenchRequests {
		result[i] = TransportRequest{
			Number:      r.Number,
			Owner:       r.Owner,
			Description: r.Description,
			Status:      r.Status,
		}
	}
	return result, nil
}

type xmlTransportComponent struct {
	XMLName   xml.Name `xml:"adtcore:objectReference"`
	NSCore    string   `xml:"xmlns:adtcore,attr"`
	ObjectURI string   `xml:"adtcore:uri,attr"`
}

func (c *httpClient) AddToTransport(ctx context.Context, objectURI, transport string) error {
	body, err := xml.Marshal(xmlTransportComponent{
		NSCore:    "http://www.sap.com/adt/core",
		ObjectURI: objectURI,
	})
	if err != nil {
		return fmt.Errorf("marshal transport component: %w", err)
	}

	path := "/sap/bc/adt/cts/transportrequests/" + transport + "/abaptransportcomponents"
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": "application/xml"},
	)
	if err != nil {
		return fmt.Errorf("AddToTransport: %w", err)
	}
	defer resp.Body.Close()
	return checkResponse(resp)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./adt/... -v -run "TestTransport|TestAddToTransport"
```

- [ ] **Step 6: Run all adt tests**

```bash
go test ./adt/... -v
```
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add adt/transport.go adt/transport_test.go testdata/
git commit -m "feat: adt transport request listing and assignment"
```

---

## Task 11: MCP Tool Handlers — Source

**Files:**
- Create: `tools/register.go`
- Create: `tools/source.go`
- Create: `tools/source_test.go`

- [ ] **Step 1: Write failing tests for source tools**

Create `tools/source_test.go`:

```go
package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mockClient is a test double for adt.Client.
type mockClient struct {
	getSourceFn  func(ctx context.Context, uri string) (*adt.SourceResult, error)
	setSourceFn  func(ctx context.Context, uri, source, etag string) error
}

func (m *mockClient) GetSource(ctx context.Context, uri string) (*adt.SourceResult, error) {
	return m.getSourceFn(ctx, uri)
}
func (m *mockClient) SetSource(ctx context.Context, uri, source, etag string) error {
	return m.setSourceFn(ctx, uri, source, etag)
}
func (m *mockClient) ActivateObject(ctx context.Context, uri string) (*adt.ActivationResult, error) { return nil, nil }
func (m *mockClient) SearchObjects(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error) { return nil, nil }
func (m *mockClient) WhereUsed(ctx context.Context, uri string) ([]adt.ObjectInfo, error) { return nil, nil }
func (m *mockClient) BrowsePackage(ctx context.Context, pkg string) ([]adt.ObjectInfo, error) { return nil, nil }
func (m *mockClient) GetObjectInfo(ctx context.Context, uri string) (*adt.ObjectInfo, error) { return nil, nil }
func (m *mockClient) SyntaxCheck(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) { return nil, nil }
func (m *mockClient) RunUnitTests(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) { return nil, nil }
func (m *mockClient) GetTransportRequests(ctx context.Context, user, status string) ([]adt.TransportRequest, error) { return nil, nil }
func (m *mockClient) AddToTransport(ctx context.Context, uri, transport string) error { return nil }

func newTestServer(client adt.Client) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAll(s, client)
	return s
}

func callTool(t *testing.T, s *server.MCPServer, toolName string, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args
	result, err := s.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool(%q): %v", toolName, err)
	}
	return result
}

func TestGetSourceTool(t *testing.T) {
	mock := &mockClient{
		getSourceFn: func(ctx context.Context, uri string) (*adt.SourceResult, error) {
			if uri != "/sap/bc/adt/programs/programs/ZTEST" {
				t.Errorf("unexpected uri: %q", uri)
			}
			return &adt.SourceResult{Source: "REPORT ZTEST.", ETag: `"abc123"`}, nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "get_source", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
	})

	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// Response should contain source and etag
	text := result.Content[0].(mcp.TextContent).Text
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parsing response JSON: %v", err)
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
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
	})

	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestSetSourceTool(t *testing.T) {
	var gotURI, gotSource, gotETag string
	mock := &mockClient{
		setSourceFn: func(ctx context.Context, uri, source, etag string) error {
			gotURI, gotSource, gotETag = uri, source, etag
			return nil
		},
	}
	s := newTestServer(mock)

	result := callTool(t, s, "set_source", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
		"source":     "REPORT ZTEST.\nNEW.",
		"etag":       `"abc123"`,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	if gotURI != "/sap/bc/adt/programs/programs/ZTEST" || gotSource != "REPORT ZTEST.\nNEW." || gotETag != `"abc123"` {
		t.Errorf("got uri=%q source=%q etag=%q", gotURI, gotSource, gotETag)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./tools/... -v -run "TestGetSource|TestSetSource"
```

- [ ] **Step 3: Implement tools/register.go**

```go
package tools

import (
	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client) {
	registerSourceTools(s, client)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
}
```

- [ ] **Step 4: Implement tools/source.go**

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSourceTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("get_source",
		mcp.WithDescription("Read ABAP source code from SAP. Returns source text and ETag for optimistic locking."),
		mcp.WithString("object_uri",
			mcp.Required(),
			mcp.Description("ADT object URI, e.g. /sap/bc/adt/programs/programs/ZREPORT"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		result, err := client.GetSource(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(map[string]string{
			"source": result.Source,
			"etag":   result.ETag,
		})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("set_source",
		mcp.WithDescription("Write ABAP source code to SAP. Requires the ETag returned by get_source to prevent lost updates."),
		mcp.WithString("object_uri",
			mcp.Required(),
			mcp.Description("ADT object URI"),
		),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("New ABAP source code"),
		),
		mcp.WithString("etag",
			mcp.Required(),
			mcp.Description("ETag value from get_source, passed verbatim including quotes"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		source, _ := req.Params.Arguments["source"].(string)
		etag, _ := req.Params.Arguments["etag"].(string)
		if err := client.SetSource(ctx, uri, source, etag); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Source updated successfully"), nil
	})
}

// errorResult converts an error to an MCP error result with the SAP error message.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())},
		},
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./tools/... -v -run "TestGetSource|TestSetSource"
```

- [ ] **Step 6: Commit**

```bash
git add tools/
git commit -m "feat: MCP tools for source read/write + tool registration scaffold"
```

---

## Task 12: MCP Tool Handlers — Activate, Search, Repository

**Files:**
- Create: `tools/activate.go`
- Create: `tools/search.go`
- Create: `tools/repository.go`
- Create: `tools/activate_test.go`

- [ ] **Step 1: Write failing test for activate**

Create `tools/activate_test.go`:

```go
package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
)

func TestActivateObjectTool(t *testing.T) {
	mock := &mockClient{}
	mock2 := *mock
	mock2.activateFn = func(ctx context.Context, uri string) (*adt.ActivationResult, error) {
		return &adt.ActivationResult{
			Success: true,
			Messages: []adt.ActivationMessage{},
		}, nil
	}
	s := newTestServer(&mock2)

	result := callTool(t, s, "activate_object", map[string]interface{}{
		"object_uri": "/sap/bc/adt/programs/programs/ZTEST",
	})

	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	var resp map[string]interface{}
	json.Unmarshal([]byte(result.Content[0].(interface{ GetText() string }).GetText()), &resp)
}
```

Note: The mockClient in `tools/source_test.go` needs an `activateFn` field. Update `mockClient` in `tools/source_test.go` to add the field and call it:

```go
type mockClient struct {
	getSourceFn    func(ctx context.Context, uri string) (*adt.SourceResult, error)
	setSourceFn    func(ctx context.Context, uri, source, etag string) error
	activateFn     func(ctx context.Context, uri string) (*adt.ActivationResult, error)
	searchFn       func(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error)
	whereUsedFn    func(ctx context.Context, uri string) ([]adt.ObjectInfo, error)
	browsePackageFn func(ctx context.Context, pkg string) ([]adt.ObjectInfo, error)
	getObjectFn    func(ctx context.Context, uri string) (*adt.ObjectInfo, error)
	syntaxCheckFn  func(ctx context.Context, uri string) ([]adt.SyntaxMessage, error)
	runTestsFn     func(ctx context.Context, uri string, timeout int) (*adt.TestResult, error)
	getTransportFn func(ctx context.Context, user, status string) ([]adt.TransportRequest, error)
	addTransportFn func(ctx context.Context, uri, transport string) error
}

// All interface methods call their fn field if non-nil, otherwise return safe zero values.
func (m *mockClient) ActivateObject(ctx context.Context, uri string) (*adt.ActivationResult, error) {
	if m.activateFn != nil { return m.activateFn(ctx, uri) }
	return &adt.ActivationResult{Success: true}, nil
}
func (m *mockClient) SearchObjects(ctx context.Context, q, t string, n int) ([]adt.ObjectInfo, error) {
	if m.searchFn != nil { return m.searchFn(ctx, q, t, n) }
	return nil, nil
}
func (m *mockClient) WhereUsed(ctx context.Context, uri string) ([]adt.ObjectInfo, error) {
	if m.whereUsedFn != nil { return m.whereUsedFn(ctx, uri) }
	return nil, nil
}
func (m *mockClient) BrowsePackage(ctx context.Context, pkg string) ([]adt.ObjectInfo, error) {
	if m.browsePackageFn != nil { return m.browsePackageFn(ctx, pkg) }
	return nil, nil
}
func (m *mockClient) GetObjectInfo(ctx context.Context, uri string) (*adt.ObjectInfo, error) {
	if m.getObjectFn != nil { return m.getObjectFn(ctx, uri) }
	return &adt.ObjectInfo{}, nil
}
func (m *mockClient) SyntaxCheck(ctx context.Context, uri string) ([]adt.SyntaxMessage, error) {
	if m.syntaxCheckFn != nil { return m.syntaxCheckFn(ctx, uri) }
	return nil, nil
}
func (m *mockClient) RunUnitTests(ctx context.Context, uri string, timeout int) (*adt.TestResult, error) {
	if m.runTestsFn != nil { return m.runTestsFn(ctx, uri, timeout) }
	return &adt.TestResult{}, nil
}
func (m *mockClient) GetTransportRequests(ctx context.Context, user, status string) ([]adt.TransportRequest, error) {
	if m.getTransportFn != nil { return m.getTransportFn(ctx, user, status) }
	return nil, nil
}
func (m *mockClient) AddToTransport(ctx context.Context, uri, transport string) error {
	if m.addTransportFn != nil { return m.addTransportFn(ctx, uri, transport) }
	return nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./tools/... -v -run "TestActivate"
```

- [ ] **Step 3: Implement tools/activate.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerActivateTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("activate_object",
		mcp.WithDescription("Activate an ABAP object in SAP. Returns success status and any activation messages."),
		mcp.WithString("object_uri",
			mcp.Required(),
			mcp.Description("ADT object URI"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		result, err := client.ActivateObject(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 4: Implement tools/search.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSearchTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("search_objects",
		mcp.WithDescription("Search for ABAP repository objects by name. Supports wildcards, e.g. ZREPORT*."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query, e.g. ZREPORT*")),
		mcp.WithString("object_type", mcp.Description("Filter by type, e.g. PROG/P for programs, CLAS/OC for classes")),
		mcp.WithNumber("max_results", mcp.Description("Maximum number of results (default: 50)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		objType, _ := req.Params.Arguments["object_type"].(string)
		maxResults := 50
		if n, ok := req.Params.Arguments["max_results"].(float64); ok {
			maxResults = int(n)
		}
		results, err := client.SearchObjects(ctx, query, objType, maxResults)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("where_used",
		mcp.WithDescription("Find all ABAP objects that use the given object."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		results, err := client.WhereUsed(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 5: Implement tools/repository.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerRepositoryTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("browse_package",
		mcp.WithDescription("List all ABAP objects in a package."),
		mcp.WithString("package_name", mcp.Required(), mcp.Description("Package name, e.g. ZPACKAGE")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg, _ := req.Params.Arguments["package_name"].(string)
		results, err := client.BrowsePackage(ctx, pkg)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("get_object_info",
		mcp.WithDescription("Get metadata for an ABAP repository object."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		info, err := client.GetObjectInfo(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(info)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 6: Run all tools tests**

```bash
go test ./tools/... -v
```

- [ ] **Step 7: Commit**

```bash
git add tools/
git commit -m "feat: MCP tools for activate, search, where-used, repository"
```

---

## Task 13: MCP Tool Handlers — SyntaxCheck, UnitTests, Transport

**Files:**
- Create: `tools/syntaxcheck.go`
- Create: `tools/unittest.go`
- Create: `tools/transport.go`

- [ ] **Step 1: Implement tools/syntaxcheck.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSyntaxCheckTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("syntax_check",
		mcp.WithDescription("Run ABAP syntax check on an object. Returns list of syntax messages with line/column info."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		msgs, err := client.SyntaxCheck(ctx, uri)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(msgs)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 2: Implement tools/unittest.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerUnitTestTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("run_unit_tests",
		mcp.WithDescription("Run ABAP Unit Tests for an object. Returns test results with pass/fail counts."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
		mcp.WithNumber("timeout_seconds", mcp.Description("Test execution timeout in seconds (default: 30)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		timeout := 30
		if t, ok := req.Params.Arguments["timeout_seconds"].(float64); ok {
			timeout = int(t)
		}
		result, err := client.RunUnitTests(ctx, uri, timeout)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(out)), nil
	})
}
```

- [ ] **Step 3: Implement tools/transport.go**

```go
package tools

import (
	"context"
	"encoding/json"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTransportTools(s *server.MCPServer, client adt.Client) {
	s.AddTool(mcp.NewTool("get_transport_requests",
		mcp.WithDescription("List CTS transport requests on the configured SAP system. Status: D=modifiable, L=released."),
		mcp.WithString("user", mcp.Description("Filter by owner username")),
		mcp.WithString("status", mcp.Description("Filter by status: D (modifiable) or L (released)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user, _ := req.Params.Arguments["user"].(string)
		status, _ := req.Params.Arguments["status"].(string)
		transports, err := client.GetTransportRequests(ctx, user, status)
		if err != nil {
			return errorResult(err), nil
		}
		out, _ := json.Marshal(transports)
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("add_to_transport",
		mcp.WithDescription("Assign an ABAP object to a CTS transport request."),
		mcp.WithString("object_uri", mcp.Required(), mcp.Description("ADT object URI")),
		mcp.WithString("transport", mcp.Required(), mcp.Description("Transport request number, e.g. DEVK900123")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri, _ := req.Params.Arguments["object_uri"].(string)
		transport, _ := req.Params.Arguments["transport"].(string)
		if err := client.AddToTransport(ctx, uri, transport); err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText("Object added to transport successfully"), nil
	})
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v
```
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add tools/
git commit -m "feat: MCP tools for syntax check, unit tests, transport"
```

---

## Task 14: main.go — Wire Everything Together

**Files:**
- Create: `main.go`

- [ ] **Step 1: Write main.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
	"github.com/dachner/sapadt-mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("SAP_CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	client := adt.NewClient(cfg)

	s := server.NewMCPServer(
		"SAP ADT MCP Server",
		version,
		server.WithToolCapabilities(true),
	)
	tools.RegisterAll(s, client)

	return server.NewStdioServer(s).Listen(nil, nil)
}
```

- [ ] **Step 2: Build and verify**

```bash
go build -o sapadt-mcp .
```
Expected: binary created without errors

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -v
```
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: main entry point, wire config + adt client + MCP server"
```

---

## Task 15: Final Build Verification

- [ ] **Step 1: Run all tests with race detector**

```bash
go test -race ./...
```
Expected: PASS (no race conditions)

- [ ] **Step 2: Verify build for all platforms**

```bash
make build-all
```
Expected: binaries created in `dist/` for linux-amd64, windows-amd64, darwin-amd64, darwin-arm64

- [ ] **Step 3: Verify binary runs**

```bash
./sapadt-mcp --help 2>&1 || echo "binary runs (expected usage error)"
```

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: verify all builds pass"
```

---

## Summary

After completing all 15 tasks, the project will have:
- Full `adt/` package with 10 SAP ADT operations, tested against `httptest.Server`
- Full `tools/` package with 11 MCP tools, tested with mock `adt.Client`
- `config/` package with YAML + env var loading
- `main.go` wiring everything for stdio MCP
- Cross-platform binaries via GoReleaser + GitHub Actions
- All code covered by unit tests, no real SAP system required
