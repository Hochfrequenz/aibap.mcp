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

func TestLoadMultiSystem(t *testing.T) {
	f := writeConfig(t, `{
  "default_system": "dev",
  "systems": {
    "dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "DEV_USER", "password": "devpass"},
    "prod": {"host": "https://prod.example.com:8000", "client": "200", "user": "PROD_USER", "password": "prodpass"}
  }
}`)
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
	f := writeConfig(t, `{"default_system": "nonexistent", "systems": {"dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "U", "password": "P"}}}`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for missing default_system")
	}
}

func TestLoadEmptySystems(t *testing.T) {
	f := writeConfig(t, `{"default_system": "", "systems": {}}`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for empty systems")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadEmptyHostReturnsError(t *testing.T) {
	f := writeConfig(t, `{"default_system": "dev", "systems": {"dev": {"host": "", "client": "100", "user": "U", "password": "P"}}}`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for system with empty host")
	}
}

func TestLoadOmittedDefaultSystem(t *testing.T) {
	f := writeConfig(t, `{"systems": {"dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "U", "password": "P"}}}`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error when default_system is omitted")
	}
}

func TestOAuth2Config(t *testing.T) {
	f := writeConfig(t, `{"default_system": "dev", "systems": {"dev": {"host": "https://dev.example.com:8000", "client": "100"}}}`)
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Systems["dev"].IsOAuth2() {
		t.Error("expected IsOAuth2() to return true for system without user/password")
	}
}

func TestBasicAuthConfig(t *testing.T) {
	f := writeConfig(t, `{"default_system": "dev", "systems": {"dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "MYUSER", "password": "secret"}}}`)
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Systems["dev"].IsOAuth2() {
		t.Error("expected IsOAuth2() to return false for system with user/password")
	}
}

func TestPartialCredentialsError(t *testing.T) {
	f := writeConfig(t, `{"default_system": "dev", "systems": {"dev": {"host": "https://dev.example.com:8000", "client": "100", "user": "MYUSER"}}}`)
	_, err := config.Load(f)
	if err == nil {
		t.Error("expected error for system with user but no password")
	}
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
	if got := cfg.Systems["dev"].EffectiveOAuth2ClientID(); got != "my-custom-client" {
		t.Errorf("EffectiveOAuth2ClientID: got %q, want %q", got, "my-custom-client")
	}
	if got := cfg.Systems["staging"].EffectiveOAuth2ClientID(); got != "mcp-server-abap" {
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
