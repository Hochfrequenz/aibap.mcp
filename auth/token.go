package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TokenData holds OAuth2 tokens for a single SAP system.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the access token has expired.
func (t TokenData) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// TokenStore manages per-system OAuth2 tokens in a JSON file.
type TokenStore struct {
	path string
}

// NewTokenStore creates a new TokenStore that persists tokens at the given path.
func NewTokenStore(path string) *TokenStore {
	return &TokenStore{path: path}
}

// DefaultTokenPath returns the default path for the token file:
// ~/.config/sap-adt/tokens.json
// It is a variable so callers (e.g. main.go) can override it for backward
// compatibility when the library is embedded in a specific application.
var DefaultTokenPath = func() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "sap-adt", "tokens.json")
}

// Save stores the given token for the named system. It creates any missing
// directories and writes the file with 0600 permissions.
func (s *TokenStore) Save(system string, token TokenData) error {
	tokens, err := s.load()
	if err != nil {
		// If the file doesn't exist yet, start with an empty map.
		tokens = make(map[string]TokenData)
	}

	tokens[system] = token

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}

	return nil
}

// TokenForSystem returns the stored token for the named system.
// It returns an error if the system is not found or the file cannot be read.
func (s *TokenStore) TokenForSystem(system string) (TokenData, error) {
	tokens, err := s.load()
	if err != nil {
		return TokenData{}, err
	}

	token, ok := tokens[system]
	if !ok {
		return TokenData{}, fmt.Errorf("no token found for system %q", system)
	}

	return token, nil
}

// load reads and parses the token file. Returns an error if the file does not
// exist or contains invalid JSON.
func (s *TokenStore) load() (map[string]TokenData, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var tokens map[string]TokenData
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}

	return tokens, nil
}
