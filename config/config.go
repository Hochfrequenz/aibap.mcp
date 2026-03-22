package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type SAPConfig struct {
	Host          string `yaml:"host"`
	Client        string `yaml:"client"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
}

type Config struct {
	SAP SAPConfig `yaml:"sap"`
}

// Load reads config from the given YAML file, then applies environment variable overrides.
// Relative paths are resolved from the process working directory.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SAP_HOST"); v != "" {
		cfg.SAP.Host = v
	}
	if v := os.Getenv("SAP_CLIENT"); v != "" {
		cfg.SAP.Client = v
	}
	if v := os.Getenv("SAP_USER"); v != "" {
		cfg.SAP.User = v
	}
	if v := os.Getenv("SAP_PASSWORD"); v != "" {
		cfg.SAP.Password = v
	}
	if v := os.Getenv("SAP_TLS_SKIP_VERIFY"); strings.EqualFold(v, "true") {
		cfg.SAP.TLSSkipVerify = true
	}
}
