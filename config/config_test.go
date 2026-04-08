package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/config"
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
