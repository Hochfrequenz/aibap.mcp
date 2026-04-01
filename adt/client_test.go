package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

const csrfEndpoint = "/sap/bc/adt/discovery"

func newTestConfig(host string) config.SAPSystem {
	return config.SAPSystem{
		Host:     host,
		Client:   "100",
		User:     "TESTUSER",
		Password: "testpass",
	}
}

func TestCSRFTokenFetchedOnFirstMutate(t *testing.T) {
	var csrfFetched atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case csrfEndpoint:
			csrfFetched.Store(true)
			w.Header().Set("X-CSRF-Token", "test-csrf-token")
			w.Header().Set("Set-Cookie", "sap-session=abc123; Path=/")
			w.WriteHeader(http.StatusOK)
		default:
			if r.Header.Get("X-CSRF-Token") != "test-csrf-token" {
				t.Errorf("expected CSRF token in request, got %q", r.Header.Get("X-CSRF-Token"))
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", "", "", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !csrfFetched.Load() {
		t.Error("expected CSRF preflight request to /sap/bc/adt/discovery")
	}
}

func TestCSRFTokenRefreshedOn403(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "refreshed-token")
			w.WriteHeader(http.StatusOK)
			return
		}
		count := callCount.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", "", "", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (initial + retry), got %d", callCount.Load())
	}
}

func TestReauthOn401(t *testing.T) {
	var authAttempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		attempt := authAttempts.Add(1)
		if attempt == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", "", "", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error after re-auth: %v", err)
	}
}

func TestADTErrorParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><exc:ExceptionText xmlns:exc="http://www.sap.com/adt/exception"><exc:message>Object not found</exc:message></exc:ExceptionText>`))
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", "", "", `"etag123"`)
	if err == nil {
		t.Fatal("expected error")
	}
	adtErr, ok := err.(*adt.ADTError)
	if !ok {
		t.Fatalf("expected *adt.ADTError, got %T: %v", err, err)
	}
	if adtErr.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", adtErr.StatusCode)
	}
	if adtErr.Message != "Object not found" {
		t.Errorf("message: got %q", adtErr.Message)
	}
}

func TestSecureCookieOnHTTPDetected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			// Simulate S4 behavior: set cookie with Secure flag over HTTP
			w.Header().Add("Set-Cookie", "sap-XSRF_S4U_100=abc123; path=/; secure; HttpOnly")
			w.WriteHeader(http.StatusOK)
			return
		}
		// Always return 403 — simulates CSRF failure due to missing cookie
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL) // httptest uses http://, not https://
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", "", "", `"etag123"`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Secure cookies") {
		t.Errorf("expected secure cookie hint in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Errorf("expected https suggestion in error, got: %v", err)
	}
}

func TestBearerAuthHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`REPORT ZTEST.`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, Client: "100"}
	client := adt.NewClientWithToken(cfg, "my-access-token", nil)

	_, err := client.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer my-access-token" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer my-access-token", gotAuth)
	}
}

func TestOAuth2TokenRefreshOn401(t *testing.T) {
	var refreshCalled atomic.Bool
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		count := callCount.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Verify the refreshed token is used
		auth := r.Header.Get("Authorization")
		if auth != "Bearer refreshed-token" {
			t.Errorf("expected refreshed token, got %q", auth)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`REPORT ZTEST.`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, Client: "100"}
	onRefresh := func(oldToken string) (string, error) {
		refreshCalled.Store(true)
		if oldToken != "expired-token" {
			t.Errorf("expected old token %q, got %q", "expired-token", oldToken)
		}
		return "refreshed-token", nil
	}
	client := adt.NewClientWithToken(cfg, "expired-token", onRefresh)

	_, err := client.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !refreshCalled.Load() {
		t.Error("expected onRefresh callback to be called")
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls (initial + retry), got %d", callCount.Load())
	}
}
