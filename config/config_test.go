package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dachner/mcp-server-abap/config"
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
