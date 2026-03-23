package config

import (
	"fmt"
	"os"

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
	}

	return &cfg, nil
}
