package config

import (
	"encoding/json"
	"fmt"
	"os"

	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

// SAPSystem is a type alias for the shared SAPSystem type, keeping backward
// compatibility within this project.
type SAPSystem = sapmcpconfig.SAPSystem

// AppConfig extends the shared Config with project-specific fields.
type AppConfig struct {
	sapmcpconfig.Config
	IntegrationTestSystems []string `json:"integration_test_systems"`
}

// EffectiveOAuth2ClientID returns the OAuth2 client ID for the given system,
// defaulting to "mcp-server-abap" when not configured.
func EffectiveOAuth2ClientID(sys sapmcpconfig.SAPSystem) string {
	if sys.OAuth2ClientID != "" {
		return sys.OAuth2ClientID
	}
	return "mcp-server-abap"
}

// IsTestSystem reports whether the named system should be used for integration tests.
func (c *AppConfig) IsTestSystem(name string) bool {
	if len(c.IntegrationTestSystems) == 0 {
		return name == c.DefaultSystem
	}
	for _, s := range c.IntegrationTestSystems {
		if s == name {
			return true
		}
	}
	return false
}

// TestSystems returns the systems configured for integration testing.
func (c *AppConfig) TestSystems() map[string]SAPSystem {
	result := make(map[string]SAPSystem)
	for _, name := range c.IntegrationTestSystems {
		if sys, ok := c.Systems[name]; ok {
			result[name] = sys
		}
	}
	if len(result) == 0 {
		if sys, ok := c.Systems[c.DefaultSystem]; ok {
			result[c.DefaultSystem] = sys
		}
	}
	return result
}

// Load reads config from the given JSON file and validates it.
func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	// Parse the shared config portion (with validation).
	sharedCfg, err := sapmcpconfig.Parse(data)
	if err != nil {
		return nil, err
	}

	// Parse again to pick up project-specific fields.
	var app AppConfig
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, fmt.Errorf("parsing config file (expected JSON): %w", err)
	}
	app.Config = *sharedCfg
	return &app, nil
}
