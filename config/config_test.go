package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

// writeConfig writes a single-system config whose user and password are the
// given raw JSON string values, and returns its path.
func writeConfig(t *testing.T, user, password string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"default_system": "dev",
		"systems": {
			"dev": {"host": "https://example.com", "user": "` + user + `", "password": "` + password + `", "client": "100"}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestLoadResolvesEnvPlaceholders pins that config.Load — the only path main.go
// and cmd/login.go use to build a SAPSystem — resolves ${env:VAR} credentials
// before adtler sees them. adtler refuses an unresolved placeholder outright
// (adt.ErrUnresolvedPlaceholder, #575), so a regression here would break every
// Basic-auth system configured with placeholders.
func TestLoadResolvesEnvPlaceholders(t *testing.T) {
	t.Setenv("AIBAP_TEST_SAP_USER", "DEVUSER")
	t.Setenv("AIBAP_TEST_SAP_PASSWORD", "s3cret")
	cfg, err := config.Load(writeConfig(t, "${env:AIBAP_TEST_SAP_USER}", "${env:AIBAP_TEST_SAP_PASSWORD}"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sys := cfg.Systems["dev"]
	if sys.User != "DEVUSER" {
		t.Errorf("User: got %q, want %q", sys.User, "DEVUSER")
	}
	if sys.Password != "s3cret" {
		t.Errorf("Password: got %q, want the resolved value", sys.Password)
	}
}

// TestLoadRejectsUnsetEnvPlaceholder pins that an unset or empty variable fails
// at load time, so the server never starts with a credential adtler would
// refuse on every request (#575).
func TestLoadRejectsUnsetEnvPlaceholder(t *testing.T) {
	t.Setenv("AIBAP_TEST_SAP_USER", "DEVUSER")
	t.Setenv("AIBAP_TEST_SAP_EMPTY", "")
	for _, password := range []string{"${env:AIBAP_TEST_SAP_UNSET}", "${env:AIBAP_TEST_SAP_EMPTY}"} {
		t.Run(password, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, "${env:AIBAP_TEST_SAP_USER}", password))
			if err == nil {
				t.Fatal("Load succeeded, want an error for the unresolvable placeholder")
			}
			if !strings.Contains(err.Error(), "password") {
				t.Errorf("error should name the password field, got: %v", err)
			}
		})
	}
}
