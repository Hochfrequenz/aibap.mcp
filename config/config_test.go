package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/aibap.mcp/config"
)

func TestLoadWithTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"default_system": "dev",
		"tools": ["source", "objects", "debug"],
		"systems": {
			"dev": {"host": "https://example.com", "user": "U", "password": "P", "client": "100"}
		}
	}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
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
	if err := os.WriteFile(path, []byte(`{
		"default_system": "dev",
		"systems": {
			"dev": {"host": "https://example.com", "user": "U", "password": "P", "client": "100"}
		}
	}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tools != nil {
		t.Errorf("Tools should be nil, got %v", cfg.Tools)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Pass sap-mcp-config validation by including required fields, then break
	// the JSON for the second AppConfig parse pass.
	if err := os.WriteFile(path, []byte(`{not valid json`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Empty systems map fails sap-mcp-config validation.
	if err := os.WriteFile(path, []byte(`{"default_system": "dev", "systems": {}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected error for empty systems")
	}
}
