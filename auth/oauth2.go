package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GeneratePKCE generates a code_verifier (43 random URL-safe chars) and
// code_challenge = BASE64URL(SHA256(verifier)) as required by RFC 7636.
func GeneratePKCE() (verifier, challenge string, err error) {
	// 32 random bytes -> 43 base64url chars (no padding), satisfying the 43-128 range.
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// AuthorizeURL builds the SAP OAuth2 authorization URL using PKCE (S256 method).
// The returned URL is:
//
//	{host}/sap/bc/sec/oauth2/authorize?response_type=code&client_id=...&redirect_uri=...&code_challenge=...&code_challenge_method=S256
func AuthorizeURL(host, clientID, redirectURI, codeChallenge string) string {
	base := strings.TrimRight(host, "/") + "/sap/bc/sec/oauth2/authorize"

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")

	return base + "?" + q.Encode()
}

// tokenResponse is the JSON shape returned by the SAP token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// httpClient returns an *http.Client that optionally skips TLS certificate verification.
func httpClient(tlsSkipVerify bool) *http.Client {
	if !tlsSkipVerify {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

// postToken posts form values to the SAP token endpoint and returns the parsed TokenData.
func postToken(host string, form url.Values, tlsSkipVerify bool) (TokenData, error) {
	endpoint := strings.TrimRight(host, "/") + "/sap/bc/sec/oauth2/token"

	client := httpClient(tlsSkipVerify)
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenData{}, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return TokenData{}, fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return TokenData{}, fmt.Errorf("decode token response: %w", err)
	}

	return TokenData{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

// ExchangeCode exchanges an authorization code for tokens using the PKCE flow.
// It POSTs to {host}/sap/bc/sec/oauth2/token with grant_type=authorization_code.
func ExchangeCode(host, clientID, code, codeVerifier, redirectURI string, tlsSkipVerify bool) (TokenData, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("client_id", clientID)
	form.Set("redirect_uri", redirectURI)

	return postToken(host, form, tlsSkipVerify)
}

// RefreshToken exchanges a refresh token for new tokens.
// It POSTs to {host}/sap/bc/sec/oauth2/token with grant_type=refresh_token.
func RefreshToken(host, clientID, refreshToken string, tlsSkipVerify bool) (TokenData, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)

	return postToken(host, form, tlsSkipVerify)
}
