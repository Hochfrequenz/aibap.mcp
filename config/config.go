package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type SAPConfig struct {
	Host            string `yaml:"host"`
	Client          string `yaml:"client"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	TLSSkipVerify   bool   `yaml:"tls_skip_verify"`
	OAuth2ClientID  string `yaml:"oauth2_client_id"`
}

// IsOAuth2 returns true when no basic-auth credentials are configured,
// meaning OAuth2 / SSO should be used.
func (c SAPConfig) IsOAuth2() bool {
	return c.User == "" && c.Password == ""
}

// EffectiveOAuth2ClientID returns the configured OAuth2 client ID, or the
// default value "mcp-server-abap" when none is set.
func (c SAPConfig) EffectiveOAuth2ClientID() string {
	if c.OAuth2ClientID != "" {
		return c.OAuth2ClientID
	}
	return "mcp-server-abap"
}

type Config struct {
	DefaultSystem string               `yaml:"default_system"`
	Systems       map[string]SAPConfig `yaml:"systems"`
}

// Load reads config from the given YAML file and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
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
