package adt_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/auth"
)

// TestNewClientsFromConfigOAuth2MissingClientID verifies that NewClientsFromConfig
// returns an error when an OAuth2 system has no client ID configured and no
// fallback is given.
func TestNewClientsFromConfigOAuth2MissingClientID(t *testing.T) {
	// Create a valid token so we get past the token check.
	tokenDir := t.TempDir()
	orig := auth.DefaultTokenPath
	auth.DefaultTokenPath = func() string { return tokenDir + "/tokens.json" }
	defer func() { auth.DefaultTokenPath = orig }()

	store := auth.NewTokenStore(auth.DefaultTokenPath())
	_ = store.Save("oauth-sys", auth.TokenData{AccessToken: "tok", RefreshToken: "ref"})

	cfg := &sapmcpconfig.Config{
		DefaultSystem: "oauth-sys",
		Systems: map[string]sapmcpconfig.SAPSystem{
			"oauth-sys": {
				Host:   "http://example.com",
				Client: "100",
				// No User/Password => IsOAuth2() == true
				// No OAuth2ClientID
			},
		},
	}

	_, err := adt.NewClientsFromConfig(cfg, "")
	if err == nil {
		t.Fatal("expected error for empty OAuth2 client ID, got nil")
	}
	if !strings.Contains(err.Error(), "oauth2_client_id") {
		t.Errorf("error should mention oauth2_client_id, got: %v", err)
	}
}

// TestNewClientsFromConfigOAuth2FallbackClientID verifies that the fallback
// client ID is used when the system's own OAuth2ClientID is empty.
func TestNewClientsFromConfigOAuth2FallbackClientID(t *testing.T) {
	tokenDir := t.TempDir()
	orig := auth.DefaultTokenPath
	auth.DefaultTokenPath = func() string { return tokenDir + "/tokens.json" }
	defer func() { auth.DefaultTokenPath = orig }()

	store := auth.NewTokenStore(auth.DefaultTokenPath())
	_ = store.Save("oauth-sys", auth.TokenData{AccessToken: "tok", RefreshToken: "ref"})

	cfg := &sapmcpconfig.Config{
		DefaultSystem: "oauth-sys",
		Systems: map[string]sapmcpconfig.SAPSystem{
			"oauth-sys": {
				Host:   "http://example.com",
				Client: "100",
			},
		},
	}

	clients, err := adt.NewClientsFromConfig(cfg, "my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := clients["oauth-sys"]; !ok {
		t.Errorf("expected oauth-sys client, got keys: %v", slices.Sorted(maps.Keys(clients)))
	}
}

// TestNewClientsFromConfigOAuth2MissingToken verifies that NewClientsFromConfig
// returns an error when a system is configured for OAuth2 (no user/password)
// but no token file exists.
func TestNewClientsFromConfigOAuth2MissingToken(t *testing.T) {
	// Point the token store at a path that definitely does not exist.
	orig := auth.DefaultTokenPath
	auth.DefaultTokenPath = func() string { return t.TempDir() + "/nonexistent/tokens.json" }
	defer func() { auth.DefaultTokenPath = orig }()

	cfg := &sapmcpconfig.Config{
		DefaultSystem: "oauth-sys",
		Systems: map[string]sapmcpconfig.SAPSystem{
			"oauth-sys": {
				Host:   "http://example.com",
				Client: "100",
				// No User/Password => IsOAuth2() == true
			},
		},
	}

	_, err := adt.NewClientsFromConfig(cfg, "")
	if err == nil {
		t.Fatal("expected error for OAuth2 system without token, got nil")
	}
	if !strings.Contains(err.Error(), "OAuth2") && !strings.Contains(err.Error(), "login") {
		t.Errorf("error message should mention OAuth2 or login, got: %v", err)
	}
}

// TestNewClientsFromConfigBasicAuth builds clients for a basic-auth system
// and verifies they are returned in the map.
func TestNewClientsFromConfigBasicAuth(t *testing.T) {
	cfg := &sapmcpconfig.Config{
		DefaultSystem: "dev",
		Systems: map[string]sapmcpconfig.SAPSystem{
			"dev":  {Host: "http://dev.example", Client: "100", User: "U", Password: "P"},
			"prod": {Host: "http://prod.example", Client: "200", User: "U", Password: "P"},
		},
	}

	clients, err := adt.NewClientsFromConfig(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d (keys: %v)", len(clients), slices.Sorted(maps.Keys(clients)))
	}
	if _, ok := clients["dev"]; !ok {
		t.Errorf("missing dev client")
	}
	if _, ok := clients["prod"]; !ok {
		t.Errorf("missing prod client")
	}
}
