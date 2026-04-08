package adt

import (
	"fmt"

	"github.com/Hochfrequenz/mcp-server-abap/auth"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

// NewClientsFromConfig builds one Client per system in cfg, ready to pass to
// NewClientRegistry. For OAuth2 systems it loads the stored token via the
// auth package's TokenStore (see auth.DefaultTokenPath) and wires up automatic
// refresh on 401 responses.
//
// defaultOAuth2ClientID is used when a system's own OAuth2ClientID is empty;
// pass "" to require explicit per-system configuration.
func NewClientsFromConfig(cfg *sapmcpconfig.Config, defaultOAuth2ClientID string) (map[string]Client, error) {
	store := auth.NewTokenStore(auth.DefaultTokenPath())
	clients := make(map[string]Client, len(cfg.Systems))
	for name, sysCfg := range cfg.Systems {
		if !sysCfg.IsOAuth2() {
			clients[name] = NewClient(sysCfg)
			continue
		}
		tokenData, err := store.TokenForSystem(name)
		if err != nil {
			return nil, fmt.Errorf("system %q requires OAuth2 login — no stored token found", name)
		}
		clientID := effectiveOAuth2ClientID(sysCfg, defaultOAuth2ClientID)
		if clientID == "" {
			return nil, fmt.Errorf("system %q: OAuth2 requires oauth2_client_id in config or a default client ID", name)
		}
		td := tokenData // closure-mutable copy of the latest known token
		onRefresh := func(_ string) (string, error) {
			newToken, err := auth.RefreshToken(
				sysCfg.Host,
				clientID,
				td.RefreshToken,
				sysCfg.TLSSkipVerify,
			)
			if err != nil {
				return "", fmt.Errorf("token refresh failed for %q: %w", name, err)
			}
			_ = store.Save(name, newToken)
			td = newToken
			return newToken.AccessToken, nil
		}
		clients[name] = NewClientWithToken(sysCfg, tokenData.AccessToken, onRefresh)
	}
	return clients, nil
}

// effectiveOAuth2ClientID returns the OAuth2 client ID for the given system,
// preferring the system's own OAuth2ClientID, then falling back to the provided default.
func effectiveOAuth2ClientID(sys sapmcpconfig.SAPSystem, fallback string) string {
	if sys.OAuth2ClientID != "" {
		return sys.OAuth2ClientID
	}
	return fallback
}
