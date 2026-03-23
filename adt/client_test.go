package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func newTestConfig(host string) config.SAPConfig {
	return config.SAPConfig{
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
		case "/sap/bc/adt/discovery":
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

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
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
		if r.URL.Path == "/sap/bc/adt/discovery" {
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

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
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
		if r.URL.Path == "/sap/bc/adt/discovery" {
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

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
	if err != nil {
		t.Fatalf("unexpected error after re-auth: %v", err)
	}
}

func TestADTErrorParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/discovery" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<?xml version="1.0"?><exc:ExceptionText xmlns:exc="http://www.sap.com/adt/exception"><exc:message>Object not found</exc:message></exc:ExceptionText>`))
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", `"etag123"`)
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
