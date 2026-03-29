package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type SAPConfig struct {
	Host           string `json:"host"`
	Client         string `json:"client"`
	User           string `json:"user"`
	Password       string `json:"password"`
	TLSSkipVerify  bool   `json:"tls_skip_verify"`
	OAuth2ClientID string `json:"oauth2_client_id"`
}

func (c SAPConfig) IsOAuth2() bool {
	return c.User == "" && c.Password == ""
}

func (c SAPConfig) EffectiveOAuth2ClientID() string {
	if c.OAuth2ClientID != "" {
		return c.OAuth2ClientID
	}
	return "mcp-server-abap"
}

type Config struct {
	DefaultSystem          string               `json:"default_system"`
	IntegrationTestSystems []string             `json:"integration_test_systems"`
	Systems                map[string]SAPConfig `json:"systems"`
}

func (c *Config) IsTestSystem(name string) bool {
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

func (c *Config) TestSystems() map[string]SAPConfig {
	result := make(map[string]SAPConfig)
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
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file (expected JSON): %w", err)
	}

	if len(cfg.Systems) == 0 {
		return nil, fmt.Errorf("config has no systems defined")
	}
	if _, ok := cfg.Systems[cfg.DefaultSystem]; !ok {
		return nil, fmt.Errorf("default_system %q not found in systems", cfg.DefaultSystem)
	}
	for name, sys := range cfg.Systems {
		if sys.Host == "" {
			return nil, fmt.Errorf("system %q has no host configured", name)
		}
		if (sys.User == "") != (sys.Password == "") {
			return nil, fmt.Errorf("system %q: must have both user and password, or neither (for OAuth2)", name)
		}
	}
	return &cfg, nil
}
