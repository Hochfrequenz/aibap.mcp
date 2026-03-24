package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestGeneratePKCE verifies that the generated verifier has a valid length (43-128)
// and that the challenge equals BASE64URL(SHA256(verifier)).
func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE returned error: %v", err)
	}

	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d is out of range [43, 128]", len(verifier))
	}

	// Compute expected challenge
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, expected)
	}
}

// TestGeneratePKCEUniqueness verifies that two calls produce different verifiers.
func TestGeneratePKCEUniqueness(t *testing.T) {
	v1, _, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("first GeneratePKCE error: %v", err)
	}
	v2, _, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("second GeneratePKCE error: %v", err)
	}
	if v1 == v2 {
		t.Error("expected different verifiers from two GeneratePKCE calls, got identical values")
	}
}

// TestAuthorizeURL verifies that the constructed URL contains all expected query parameters.
func TestAuthorizeURL(t *testing.T) {
	host := "https://sap.example.com"
	clientID := "my-client"
	redirectURI := "http://localhost:8080/callback"
	codeChallenge := "test-challenge-abc123"

	rawURL := AuthorizeURL(host, clientID, redirectURI, codeChallenge)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("AuthorizeURL produced invalid URL: %v", err)
	}

	if !strings.HasSuffix(parsed.Path, "/sap/bc/sec/oauth2/authorize") {
		t.Errorf("unexpected path: %q", parsed.Path)
	}

	q := parsed.Query()
	checks := map[string]string{
		"response_type":         "code",
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"code_challenge":        codeChallenge,
		"code_challenge_method": "S256",
	}
	for key, want := range checks {
		if got := q.Get(key); got != want {
			t.Errorf("query param %q: got %q, want %q", key, got, want)
		}
	}
}

// TestExchangeCode starts a fake token endpoint, calls ExchangeCode and verifies
// the returned TokenData and that the server received the correct form parameters.
func TestExchangeCode(t *testing.T) {
	const (
		wantClientID    = "my-client"
		wantCode        = "auth-code-xyz"
		wantVerifier    = "my-verifier"
		wantRedirectURI = "http://localhost:8080/callback"
		expiresIn       = 3600
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, err := url.ParseQuery(string(body))
		if err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}

		checks := map[string]string{
			"grant_type":    "authorization_code",
			"code":          wantCode,
			"code_verifier": wantVerifier,
			"client_id":     wantClientID,
			"redirect_uri":  wantRedirectURI,
		}
		for k, v := range checks {
			if got := form.Get(k); got != v {
				http.Error(w, "unexpected param "+k+": "+got, http.StatusBadRequest)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-tok-1",
			"refresh_token": "refresh-tok-1",
			"expires_in":    expiresIn,
		})
	}))
	defer srv.Close()

	before := time.Now()
	token, err := ExchangeCode(srv.URL, wantClientID, wantCode, wantVerifier, wantRedirectURI, false)
	after := time.Now()

	if err != nil {
		t.Fatalf("ExchangeCode returned error: %v", err)
	}
	if token.AccessToken != "access-tok-1" {
		t.Errorf("AccessToken: got %q, want %q", token.AccessToken, "access-tok-1")
	}
	if token.RefreshToken != "refresh-tok-1" {
		t.Errorf("RefreshToken: got %q, want %q", token.RefreshToken, "refresh-tok-1")
	}
	lo := before.Add(time.Duration(expiresIn) * time.Second)
	hi := after.Add(time.Duration(expiresIn) * time.Second)
	if token.ExpiresAt.Before(lo) || token.ExpiresAt.After(hi) {
		t.Errorf("ExpiresAt %v is outside expected range [%v, %v]", token.ExpiresAt, lo, hi)
	}
}

// TestRefreshToken starts a fake token endpoint, calls RefreshToken and verifies
// the returned TokenData and the server-side form parameters.
func TestRefreshToken(t *testing.T) {
	const (
		wantClientID     = "my-client"
		wantRefreshToken = "old-refresh-token"
		expiresIn        = 1800
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, err := url.ParseQuery(string(body))
		if err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}

		checks := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": wantRefreshToken,
			"client_id":     wantClientID,
		}
		for k, v := range checks {
			if got := form.Get(k); got != v {
				http.Error(w, "unexpected param "+k+": "+got, http.StatusBadRequest)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-tok",
			"refresh_token": "new-refresh-tok",
			"expires_in":    expiresIn,
		})
	}))
	defer srv.Close()

	before := time.Now()
	token, err := RefreshToken(srv.URL, wantClientID, wantRefreshToken, false)
	after := time.Now()

	if err != nil {
		t.Fatalf("RefreshToken returned error: %v", err)
	}
	if token.AccessToken != "new-access-tok" {
		t.Errorf("AccessToken: got %q, want %q", token.AccessToken, "new-access-tok")
	}
	if token.RefreshToken != "new-refresh-tok" {
		t.Errorf("RefreshToken: got %q, want %q", token.RefreshToken, "new-refresh-tok")
	}
	lo := before.Add(time.Duration(expiresIn) * time.Second)
	hi := after.Add(time.Duration(expiresIn) * time.Second)
	if token.ExpiresAt.Before(lo) || token.ExpiresAt.After(hi) {
		t.Errorf("ExpiresAt %v is outside expected range [%v, %v]", token.ExpiresAt, lo, hi)
	}
}

// TestRefreshTokenServerError verifies that a 400 response causes RefreshToken to return an error.
func TestRefreshTokenServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := RefreshToken(srv.URL, "client", "bad-token", false)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}
