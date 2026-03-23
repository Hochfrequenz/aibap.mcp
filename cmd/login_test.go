package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dachner/mcp-server-abap/auth"
)

func TestRunLoginFullFlow(t *testing.T) {
	// Set up a mock OAuth2 token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/sec/oauth2/token" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	// Create a temp config file pointing at the mock server.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := fmt.Sprintf(`default_system: DEV
systems:
  DEV:
    host: %s
    client: "100"
    tls_skip_verify: true
`, tokenServer.URL)

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Override the token path so we write to a temp file.
	tokenPath := filepath.Join(tmpDir, "tokens.json")
	origDefaultTokenPath := auth.DefaultTokenPath
	auth.DefaultTokenPath = func() string { return tokenPath }
	defer func() { auth.DefaultTokenPath = origDefaultTokenPath }()

	// Override the browser opener to simulate the user's browser hitting the callback.
	origOpenBrowserFn := openBrowserFn
	defer func() { openBrowserFn = origOpenBrowserFn }()

	openBrowserFn = func(url string) error {
		// The authorize URL contains the redirect_uri. Extract it and hit
		// the callback with a fake code.
		go func() {
			// Small delay to ensure the callback server is ready.
			time.Sleep(50 * time.Millisecond)

			// Parse redirect_uri from the authorize URL query params.
			idx := strings.Index(url, "?")
			if idx < 0 {
				return
			}
			// Find redirect_uri param.
			params := url[idx+1:]
			var redirectURI string
			for _, p := range strings.Split(params, "&") {
				if strings.HasPrefix(p, "redirect_uri=") {
					redirectURI, _ = strings.CutPrefix(p, "redirect_uri=")
					// URL-decode it.
					redirectURI = strings.ReplaceAll(redirectURI, "%3A", ":")
					redirectURI = strings.ReplaceAll(redirectURI, "%2F", "/")
					break
				}
			}
			if redirectURI == "" {
				return
			}

			// Simulate the browser redirect to the callback with a code.
			http.Get(redirectURI + "?code=test-auth-code") //nolint:errcheck
		}()
		return nil
	}

	// Run the login flow.
	if err := RunLogin(configPath, ""); err != nil {
		t.Fatalf("RunLogin() error: %v", err)
	}

	// Verify the token was saved.
	store := auth.NewTokenStore(tokenPath)
	token, err := store.TokenForSystem("DEV")
	if err != nil {
		t.Fatalf("TokenForSystem() error: %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("access_token = %q, want %q", token.AccessToken, "test-access-token")
	}
	if token.RefreshToken != "test-refresh-token" {
		t.Errorf("refresh_token = %q, want %q", token.RefreshToken, "test-refresh-token")
	}
	if token.IsExpired() {
		t.Error("token should not be expired")
	}
}

func TestRunLoginInvalidSystem(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `default_system: DEV
systems:
  DEV:
    host: https://example.com
    client: "100"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := RunLogin(configPath, "NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for nonexistent system, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

func TestRunLoginBasicAuthSystem(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `default_system: DEV
systems:
  DEV:
    host: https://example.com
    client: "100"
    user: myuser
    password: mypassword
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := RunLogin(configPath, "DEV")
	if err == nil {
		t.Fatal("expected error for basic auth system, got nil")
	}
	if !strings.Contains(err.Error(), "basic auth") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "basic auth")
	}
	if !strings.Contains(err.Error(), "login not needed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "login not needed")
	}
}
