package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEffectiveOAuth2ClientID(t *testing.T) {
	f := writeConfig(t, `{
  "default_system": "dev",
  "systems": {
    "dev": {"host": "https://dev.example.com:8000", "client": "100", "oauth2_client_id": "my-custom-client"},
    "staging": {"host": "https://staging.example.com:8000", "client": "200"}
  }
}`)
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := config.EffectiveOAuth2ClientID(cfg.Systems["dev"]); got != "my-custom-client" {
		t.Errorf("EffectiveOAuth2ClientID: got %q, want %q", got, "my-custom-client")
	}
	if got := config.EffectiveOAuth2ClientID(cfg.Systems["staging"]); got != "mcp-server-abap" {
		t.Errorf("EffectiveOAuth2ClientID: got %q, want %q", got, "mcp-server-abap")
	}
}

func TestLoadWithTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{
		"default_system": "dev",
		"tools": ["source", "objects", "debug"],
		"systems": {
			"dev": {"host": "https://example.com", "user": "U", "password": "P", "client": "100"}
		}
	}`), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Tools) != 3 {
		t.Fatalf("Tools: got %d, want 3", len(cfg.Tools))
	}
	if cfg.Tools[0] != "source" {
		t.Errorf("Tools[0]: got %q", cfg.Tools[0])
	}
}

func TestLoadWithoutTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{
		"default_system": "dev",
		"systems": {
			"dev": {"host": "https://example.com", "user": "U", "password": "P", "client": "100"}
		}
	}`), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tools != nil {
		t.Errorf("Tools should be nil, got %v", cfg.Tools)
	}
}

func TestIntegrationTestSystems(t *testing.T) {
	f := writeConfig(t, `{
  "default_system": "dev",
  "integration_test_systems": ["dev", "staging"],
  "systems": {
    "dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "U", "password": "P"},
    "staging": {"host": "https://staging.example.com:8000", "client": "200", "user": "U", "password": "P"},
    "prod": {"host": "https://prod.example.com:8000", "client": "300", "user": "U", "password": "P"}
  }
}`)
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IsTestSystem("dev") {
		t.Error("dev should be a test system")
	}
	if !cfg.IsTestSystem("staging") {
		t.Error("staging should be a test system")
	}
	if cfg.IsTestSystem("prod") {
		t.Error("prod should NOT be a test system")
	}
	ts := cfg.TestSystems()
	if len(ts) != 2 {
		t.Errorf("expected 2 test systems, got %d", len(ts))
	}
}
