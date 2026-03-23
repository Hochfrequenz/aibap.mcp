# Multi-System Config — Design Spec

**Date:** 2026-03-23
**Status:** Approved

## Overview

Extend mcp-server-abap to support multiple named SAP systems in a single config file. A new `select_system` MCP tool lets Claude switch the active system between tool calls — enabling workflows like "read source from DEV, apply corresponding change in QA".

## Config Format

`config.yaml` changes from a single `sap:` block to a `systems:` map with a `default_system` key:

```yaml
default_system: dev

systems:
  dev:
    host: "https://dev-system:8000"
    client: "100"
    user: "dev_user"
    password: "secret"
    tls_skip_verify: false
  prod:
    host: "https://prod-system:8000"
    client: "200"
    user: "prod_user"
    password: "secret"
    tls_skip_verify: false
```

**Breaking change:** The old `sap:` top-level key is removed. Users must migrate their `config.yaml`. Documented in README.

**Environment variables:** `SAP_CONFIG_FILE` (path override) is retained. Per-field env overrides (`SAP_HOST`, `SAP_USER`, etc.) are removed — they don't make sense for multi-system configs.

## Architecture

```
config.Load() → Config{ DefaultSystem, Systems map[string]SAPConfig }
                    ↓
main.go: NewClientRegistry(cfg) → ClientRegistry
                    ↓
tools.RegisterAll(s, registry)   ← registry implements adt.Client
                    ↓
select_system tool → registry.Select("prod")
all other tools   → registry delegates to active client
```

## Components

### `config/config.go` (modified)

```go
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
```

`Load()` validates: `systems` must be non-empty, `default_system` must exist in `systems`.

`applyEnvOverrides` is removed (no longer applicable).

### `adt/client.go` (modified)

`NewClient` signature changes from `NewClient(cfg *config.Config) Client` to `NewClient(cfg config.SAPConfig) Client` — accepts a single system's config directly instead of the full multi-system config. This is required because `ClientRegistry` creates one client per system.

### `adt/registry.go` (new)

```go
type ClientRegistry struct {
    mu      sync.RWMutex
    clients map[string]Client
    configs map[string]config.SAPConfig
    active  string
}

func NewClientRegistry(cfg *config.Config) (*ClientRegistry, error)
func (r *ClientRegistry) Select(name string) (string, error) // write lock; returns "Active system: <name> (<host>)"
func (r *ClientRegistry) ActiveName() string
```

`ClientRegistry` implements `adt.Client` by forwarding every method to the currently active client under a read lock. All 11 interface methods are delegated. `Select` takes a write lock — in-flight requests against the previous system complete normally.

`configs` map stores `config.SAPConfig` by value (not pointer) to avoid unsafe map-entry addressing. The display string for `Select` is built from `configs[name].Host` inside the write lock.

Clients are created eagerly at startup (one per system). Error on startup if any system config is invalid.

### `tools/system.go` (new)

Registers the `select_system` tool:

```
select_system(system: string)
→ "Active system: prod (https://prod-system:8000)"
```

Returns an error if the system name is not found in config.

`RegisterAll` is updated to also call `registerSystemTools`.

### `main.go` (modified)

```go
cfg, err := config.Load(configPath)
registry, err := adt.NewClientRegistry(cfg)
tools.RegisterAll(s, registry)
```

### `config.yaml.example` (updated)

Replaced with multi-system format.

### `README.md` (updated)

- New config format documented
- Migration note for users of the old `sap:` format
- `select_system` tool added to tool table

## Error Handling

- Startup: fail fast if `default_system` is missing from `systems`, or if any system host is empty
- `select_system`: return `mcp.CallToolResult{IsError: true}` if system name unknown, listing valid names
- All delegated calls: pass through errors from the active client unchanged

## Testing

- `config/config_test.go`: test Load with multi-system format, missing default_system, empty systems
- `adt/registry_test.go`: test Select switches active client, unknown system returns error, delegation works
- `tools/system_test.go`: test select_system tool success and unknown system error

## Migration

Old `config.yaml`:
```yaml
sap:
  host: "..."
  client: "100"
  user: "..."
  password: "..."
```

New `config.yaml`:
```yaml
default_system: default

systems:
  default:
    host: "..."
    client: "100"
    user: "..."
    password: "..."
```
