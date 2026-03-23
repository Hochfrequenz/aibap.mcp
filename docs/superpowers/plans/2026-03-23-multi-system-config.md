# Multi-System Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add support for multiple named SAP systems in a single config file, switchable at runtime via a `select_system` MCP tool.

**Architecture:** A `ClientRegistry` in `adt/registry.go` implements `adt.Client` by delegating all calls to the currently active named client. A new `select_system` MCP tool calls `registry.Select(name)` to switch systems. The config format changes from `sap:` (single system) to `systems:` (named map) with a `default_system` key.

**Tech Stack:** Go 1.23, `github.com/mark3labs/mcp-go v0.45.0`, `gopkg.in/yaml.v3`

---

## File Map

| File | Change |
|------|--------|
| `config/config.go` | Replace `Config` struct, rewrite `Load()`, remove env overrides |
| `config/config_test.go` | Replace all tests for new format |
| `adt/client.go` | `NewClient(cfg config.SAPConfig)`, `httpClient.cfg config.SAPConfig`, update all `c.cfg.SAP.*` → `c.cfg.*` |
| `adt/client_test.go` | Update `newTestConfig` helper |
| `adt/activate_test.go`, `repository_test.go`, `search_test.go`, `source_test.go`, `syntaxcheck_test.go`, `transport_test.go`, `unittest_test.go` | Update `cfg` construction and `NewClient` call |
| `adt/registry.go` | New: `ClientRegistry`, `NewClientRegistry`, `Select`, `ActiveName`, 11 delegated methods |
| `adt/registry_test.go` | New: tests for Select, unknown system, delegation |
| `tools/register.go` | Add `SystemSelector` interface, update `RegisterAll` signature |
| `tools/system.go` | New: `select_system` tool |
| `tools/system_test.go` | New: tests for `select_system` |
| `tools/source_test.go` | Update `newTestServer` and `mockSelector` |
| `main.go` | Use `NewClientRegistry` |
| `config.yaml.example` | New multi-system format |
| `README.md` | Update config docs, tool table, migration note |

---

## Task 1: Update config package

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`

- [ ] **Step 1: Write failing tests for new config format**

Replace all content of `config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dachner/mcp-server-abap/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLoadMultiSystem(t *testing.T) {
	f := writeConfig(t, `
default_system: dev
systems:
  dev:
    host: "https://dev.example.com:8000"
    client: "100"
    user: "DEV_USER"
    password: "devpass"
    tls_skip_verify: false
  prod:
    host: "https://prod.example.com:8000"
    client: "200"
    user: "PROD_USER"
    password: "prodpass"
`)
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultSystem != "dev" {
		t.Errorf("default_system: got %q", cfg.DefaultSystem)
	}
	if len(cfg.Systems) != 2 {
		t.Fatalf("expected 2 systems, got %d", len(cfg.Systems))
	}
	dev := cfg.Systems["dev"]
	if dev.Host != "https://dev.example.com:8000" {
		t.Errorf("dev host: got %q", dev.Host)
	}
	if dev.User != "DEV_USER" {
		t.Errorf("dev user: got %q", dev.User)
	}
}

func TestLoadMissingDefaultSystem(t *testing.T) {
	f := writeConfig(t, `
default_system: nonexistent
systems:
  dev:
    host: "https://dev.example.com:8000"
    client: "100"
    user: "U"
    password: "P"
`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for missing default_system")
	}
}

func TestLoadEmptySystems(t *testing.T) {
	f := writeConfig(t, `
default_system: ""
systems: {}
`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for empty systems")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./config/... -v -run TestLoad
```
Expected: FAIL (old struct fields don't match)

- [ ] **Step 3: Replace config.go**

Replace all content of `config/config.go`:

```go
package config

import (
	"fmt"
	"os"

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
	DefaultSystem string               `yaml:"default_system"`
	Systems       map[string]SAPConfig `yaml:"systems"`
}

// Load reads config from the given YAML file and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if len(cfg.Systems) == 0 {
		return nil, fmt.Errorf("config has no systems defined")
	}
	if _, ok := cfg.Systems[cfg.DefaultSystem]; !ok {
		return nil, fmt.Errorf("default_system %q not found in systems", cfg.DefaultSystem)
	}

	return &cfg, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./config/... -v
```
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat: multi-system config format (default_system + systems map)"
```

---

## Task 2: Update adt.NewClient signature

**Files:**
- Modify: `adt/client.go`
- Modify: `adt/client_test.go`
- Modify: `adt/activate_test.go`, `adt/repository_test.go`, `adt/search_test.go`, `adt/source_test.go`, `adt/syntaxcheck_test.go`, `adt/transport_test.go`, `adt/unittest_test.go`

- [ ] **Step 1: Update adt/client.go**

Make three changes to `adt/client.go`:

**1. Change httpClient struct field** (line 34):
```go
// Before:
type httpClient struct {
	cfg            *config.Config

// After:
type httpClient struct {
	cfg            config.SAPConfig
```

**2. Change NewClient signature** (lines 42–55):
```go
// Before:
func NewClient(cfg *config.Config) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.SAP.TLSSkipVerify, //nolint:gosec
		},
	}
	return &httpClient{
		cfg: cfg,

// After:
func NewClient(cfg config.SAPConfig) Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec
		},
	}
	return &httpClient{
		cfg: cfg,
```

**3. Replace all `c.cfg.SAP.` with `c.cfg.`** — there are 7 occurrences in `fetchCSRFToken`, `setBasicAuth`, `doRead`, `execMutate`:

In `fetchCSRFToken` (line 60):
```go
// Before: url := c.cfg.SAP.Host + "/sap/bc/adt/discovery"
url := c.cfg.Host + "/sap/bc/adt/discovery"
```

In `setBasicAuth` (lines 81–84):
```go
// Before:
func (c *httpClient) setBasicAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.SAP.User, c.cfg.SAP.Password)
	if c.cfg.SAP.Client != "" {
		req.Header.Set("sap-client", c.cfg.SAP.Client)

// After:
func (c *httpClient) setBasicAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	if c.cfg.Client != "" {
		req.Header.Set("sap-client", c.cfg.Client)
```

In `doRead` (line 96):
```go
// Before: req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.SAP.Host+path, nil)
req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Host+path, nil)
```

In `execMutate` (line 200):
```go
// Before: req, err := http.NewRequestWithContext(ctx, method, c.cfg.SAP.Host+path, body)
req, err := http.NewRequestWithContext(ctx, method, c.cfg.Host+path, body)
```

- [ ] **Step 2: Update adt/client_test.go**

Replace `newTestConfig`:
```go
// Before:
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

// After:
func newTestConfig(host string) config.SAPConfig {
	return config.SAPConfig{
		Host:     host,
		Client:   "100",
		User:     "TESTUSER",
		Password: "testpass",
	}
}
```

- [ ] **Step 3: Update all other adt test files**

In each of `activate_test.go`, `repository_test.go`, `search_test.go`, `source_test.go`, `syntaxcheck_test.go`, `transport_test.go`, `unittest_test.go`, replace the pattern:

```go
// Before (appears twice per file):
cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
client := adt.NewClient(cfg)

// After:
cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
client := adt.NewClient(cfg)
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./adt/... -v
```
Expected: all existing adt tests pass

- [ ] **Step 5: Commit**

```bash
git add adt/client.go adt/client_test.go adt/activate_test.go adt/repository_test.go adt/search_test.go adt/source_test.go adt/syntaxcheck_test.go adt/transport_test.go adt/unittest_test.go
git commit -m "refactor: NewClient accepts config.SAPConfig directly (multi-system prep)"
```

---

## Task 3: Implement ClientRegistry

**Files:**
- Create: `adt/registry.go`
- Create: `adt/registry_test.go`

- [ ] **Step 1: Write failing tests**

Create `adt/registry_test.go`:

```go
package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func makeRegistryConfig(systems map[string]string, defaultSystem string) *config.Config {
	cfgSystems := make(map[string]config.SAPConfig, len(systems))
	for name, host := range systems {
		cfgSystems[name] = config.SAPConfig{Host: host, Client: "100", User: "U", Password: "P"}
	}
	return &config.Config{DefaultSystem: defaultSystem, Systems: cfgSystems}
}

func TestRegistryDefaultSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"></adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": "http://nowhere"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "dev" {
		t.Errorf("active: got %q, want %q", registry.ActiveName(), "dev")
	}
}

func TestRegistrySelectSwitchesSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := registry.Select("prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "prod" {
		t.Errorf("active after select: got %q", registry.ActiveName())
	}
	if msg == "" {
		t.Error("expected non-empty display message")
	}
}

func TestRegistrySelectUnknownSystem(t *testing.T) {
	cfg := makeRegistryConfig(map[string]string{"dev": "http://dev"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.Select("nonexistent")
	if err == nil {
		t.Error("expected error for unknown system")
	}
}

func TestRegistryDelegatesGetSource(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/discovery" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		called = true
		w.Header().Set("ETag", "etag123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("REPORT ZTEST."))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected GetSource to delegate to underlying client")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./adt/... -v -run TestRegistry
```
Expected: FAIL (adt.NewClientRegistry undefined)

- [ ] **Step 3: Implement adt/registry.go**

Create `adt/registry.go`:

```go
package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dachner/mcp-server-abap/config"
)

// ClientRegistry holds multiple named ADT clients and tracks which is active.
// It implements adt.Client by delegating all calls to the currently active client.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]Client
	configs map[string]config.SAPConfig
	active  string
}

// NewClientRegistry creates one Client per system in cfg, with cfg.DefaultSystem active.
func NewClientRegistry(cfg *config.Config) (*ClientRegistry, error) {
	clients := make(map[string]Client, len(cfg.Systems))
	for name, sysCfg := range cfg.Systems {
		clients[name] = NewClient(sysCfg)
	}
	return &ClientRegistry{
		clients: clients,
		configs: cfg.Systems,
		active:  cfg.DefaultSystem,
	}, nil
}

// Select switches the active system. Returns a display string including the system name and host.
// Takes a write lock — in-flight requests against the previous system complete normally.
func (r *ClientRegistry) Select(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[name]; !ok {
		names := make([]string, 0, len(r.clients))
		for n := range r.clients {
			names = append(names, n)
		}
		sort.Strings(names)
		return "", fmt.Errorf("unknown system %q, available: %s", name, strings.Join(names, ", "))
	}
	r.active = name
	return fmt.Sprintf("Active system: %s (%s)", name, r.configs[name].Host), nil
}

// ActiveName returns the name of the currently active system.
func (r *ClientRegistry) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *ClientRegistry) activeClient() Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[r.active]
}

// --- adt.Client delegation (all 11 methods) ---

func (r *ClientRegistry) GetSource(ctx context.Context, objectURI string) (*SourceResult, error) {
	return r.activeClient().GetSource(ctx, objectURI)
}
func (r *ClientRegistry) SetSource(ctx context.Context, objectURI, source, etag string) error {
	return r.activeClient().SetSource(ctx, objectURI, source, etag)
}
func (r *ClientRegistry) ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error) {
	return r.activeClient().ActivateObject(ctx, objectURI)
}
func (r *ClientRegistry) SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error) {
	return r.activeClient().SearchObjects(ctx, query, objectType, maxResults)
}
func (r *ClientRegistry) WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error) {
	return r.activeClient().WhereUsed(ctx, objectURI)
}
func (r *ClientRegistry) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	return r.activeClient().BrowsePackage(ctx, packageName)
}
func (r *ClientRegistry) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	return r.activeClient().GetObjectInfo(ctx, objectURI)
}
func (r *ClientRegistry) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	return r.activeClient().SyntaxCheck(ctx, objectURI)
}
func (r *ClientRegistry) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	return r.activeClient().RunUnitTests(ctx, objectURI, timeoutSeconds)
}
func (r *ClientRegistry) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	return r.activeClient().GetTransportRequests(ctx, user, status)
}
func (r *ClientRegistry) AddToTransport(ctx context.Context, objectURI, transport string) error {
	return r.activeClient().AddToTransport(ctx, objectURI, transport)
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./adt/... -v
```
Expected: all adt tests pass (including 4 new registry tests)

- [ ] **Step 5: Commit**

```bash
git add adt/registry.go adt/registry_test.go
git commit -m "feat: ClientRegistry delegates adt.Client to active named system"
```

---

## Task 4: Add select_system tool

**Files:**
- Create: `tools/system.go`
- Create: `tools/system_test.go`
- Modify: `tools/register.go`
- Modify: `tools/source_test.go` (update `newTestServer`)

- [ ] **Step 1: Write failing tests**

Create `tools/system_test.go`:

```go
package tools_test

import (
	"fmt"
	"testing"
)

type mockSelector struct {
	selectFn     func(name string) (string, error)
	activeNameFn func() string
}

func (m *mockSelector) Select(name string) (string, error) {
	if m.selectFn != nil {
		return m.selectFn(name)
	}
	return "Active system: " + name + " (http://example.com)", nil
}

func (m *mockSelector) ActiveName() string {
	if m.activeNameFn != nil {
		return m.activeNameFn()
	}
	return "dev"
}

func TestSelectSystemSuccess(t *testing.T) {
	selector := &mockSelector{
		selectFn: func(name string) (string, error) {
			return "Active system: " + name + " (https://prod:8000)", nil
		},
	}
	s := newTestServerWithSelector(&mockClient{}, selector)
	result := callTool(t, s, "select_system", map[string]any{"system": "prod"})
	if result["isError"] != nil {
		t.Fatalf("unexpected error: %v", result)
	}
}

func TestSelectSystemUnknown(t *testing.T) {
	selector := &mockSelector{
		selectFn: func(name string) (string, error) {
			return "", fmt.Errorf("unknown system %q", name)
		},
	}
	s := newTestServerWithSelector(&mockClient{}, selector)
	result := callTool(t, s, "select_system", map[string]any{"system": "nonexistent"})
	if result["isError"] == nil {
		t.Error("expected error result for unknown system")
	}
}
```

Update `tools/source_test.go` — replace `newTestServer`:
```go
// Before:
func newTestServer(client adt.Client) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAll(s, client)
	return s
}

// After:
func newTestServer(client adt.Client) *server.MCPServer {
	return newTestServerWithSelector(client, &mockSelector{})
}

func newTestServerWithSelector(client adt.Client, selector tools.SystemSelector) *server.MCPServer {
	s := server.NewMCPServer("test", "0.0.1")
	tools.RegisterAll(s, client, selector)
	return s
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./tools/... -v -run TestSelectSystem
```
Expected: FAIL (tools.SystemSelector undefined, RegisterAll wrong signature)

- [ ] **Step 3: Update tools/register.go**

Replace content:

```go
package tools

import (
	"github.com/dachner/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/server"
)

// SystemSelector can switch the active SAP system.
type SystemSelector interface {
	Select(name string) (string, error)
	ActiveName() string
}

// RegisterAll registers all SAP ADT MCP tools on the given server.
func RegisterAll(s *server.MCPServer, client adt.Client, selector SystemSelector) {
	registerSourceTools(s, client)
	registerActivateTools(s, client)
	registerSearchTools(s, client)
	registerRepositoryTools(s, client)
	registerSyntaxCheckTools(s, client)
	registerUnitTestTools(s, client)
	registerTransportTools(s, client)
	registerSystemTools(s, selector)
}
```

- [ ] **Step 4: Create tools/system.go**

```go
package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSystemTools(s *server.MCPServer, selector SystemSelector) {
	s.AddTool(mcp.NewTool("select_system",
		mcp.WithDescription("Switch the active SAP system for all subsequent tool calls. Returns the active system name and host."),
		mcp.WithString("system",
			mcp.Required(),
			mcp.Description("Name of the system to activate, as defined in config.yaml (e.g. \"dev\", \"prod\")"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("system", "")
		msg, err := selector.Select(name)
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultText(msg), nil
	})
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./tools/... -v
```
Expected: all tools tests pass (including 2 new system tests)

- [ ] **Step 6: Commit**

```bash
git add tools/register.go tools/system.go tools/system_test.go tools/source_test.go
git commit -m "feat: select_system tool + SystemSelector interface"
```

---

## Task 5: Wire main.go and update docs

**Files:**
- Modify: `main.go`
- Modify: `config.yaml.example`
- Modify: `README.md`

- [ ] **Step 1: Update main.go**

Replace content:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
	"github.com/dachner/mcp-server-abap/tools"
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

	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		return fmt.Errorf("creating client registry: %w", err)
	}

	s := server.NewMCPServer("SAP ADT MCP Server", version)
	tools.RegisterAll(s, registry, registry)

	stdioServer := server.NewStdioServer(s)
	ctx := context.Background()
	return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
}
```

- [ ] **Step 2: Run build — expect success**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```
Expected: all tests pass

- [ ] **Step 4: Update config.yaml.example**

Replace content:

```yaml
# mcp-server-abap configuration
# Copy to config.yaml and fill in your SAP system details.
# config.yaml is listed in .gitignore — never commit real credentials.

default_system: dev

systems:
  dev:
    host: "https://your-dev-system:8000"
    client: "100"
    user: "DEVELOPER"
    password: ""
    tls_skip_verify: false   # set true for self-signed certificates

  # Add more systems as needed:
  # prod:
  #   host: "https://your-prod-system:8000"
  #   client: "200"
  #   user: "DEVELOPER"
  #   password: ""
```

- [ ] **Step 5: Update config.yaml (local, not committed)**

Update `config.yaml` to multi-system format:

```yaml
default_system: hfq

systems:
  hfq:
    host: "http://hfq.sap.msp.local:8100"
    client: "100"
    user: "dachnerm"
    password: "Neufrequenz"
    tls_skip_verify: false
```

- [ ] **Step 6: Update README.md**

In README.md make three changes:

**1. Replace the config YAML example** (under "Configuration" section):
```yaml
default_system: dev

systems:
  dev:
    host: "https://your-dev-system:8000"
    client: "100"
    user: "YOUR_USER"
    password: "YOUR_PASSWORD"
    tls_skip_verify: false
  prod:
    host: "https://your-prod-system:8000"
    client: "200"
    user: "YOUR_USER"
    password: "YOUR_PASSWORD"
```

**2. Remove the env vars table** (SAP_HOST, SAP_CLIENT, etc. no longer apply). Keep only `SAP_CONFIG_FILE`.

**3. Add `select_system` to the tool table:**
```
| `select_system` | Switch the active SAP system for subsequent calls |
```

**4. Add a Migration note** after the config section:
```markdown
### Migrating from v0.x

The `sap:` config key has been replaced with `systems:`. Wrap your existing config:

```yaml
# Old:
sap:
  host: "..."

# New:
default_system: default
systems:
  default:
    host: "..."
```
```

- [ ] **Step 7: Rebuild and smoke test**

```bash
go build -o mcp-server-abap.exe .
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"select_system","arguments":{"system":"hfq"}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_objects","arguments":{"query":"Z*","max_results":3}}}' | timeout 15 ./mcp-server-abap.exe
```
Expected: select_system returns `"Active system: hfq (http://hfq.sap.msp.local:8100)"`, search returns results.

- [ ] **Step 8: Commit**

```bash
git add main.go config.yaml.example README.md
git commit -m "feat: wire ClientRegistry in main, update docs for multi-system config"
```

---

## Done

All tasks complete. Push with:

```bash
TOKEN=$(gh auth token) && git -c "url.https://hf-mrdachner:${TOKEN}@github.com/.insteadOf=https://github.com/" push
```
