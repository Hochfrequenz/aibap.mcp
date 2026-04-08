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
	Tools []string `json:"tools"`
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
