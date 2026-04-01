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
