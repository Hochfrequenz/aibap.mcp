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

func TestLoadOmittedDefaultSystem(t *testing.T) {
	f := writeConfig(t, `
systems:
  dev:
    host: "https://dev.example.com:8000"
    client: "100"
    user: "U"
    password: "P"
`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error when default_system is omitted")
	}
}
