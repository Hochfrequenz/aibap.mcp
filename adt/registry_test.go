package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func makeRegistryConfig(systems map[string]string, defaultSystem string) *config.Config {
	cfgSystems := make(map[string]config.SAPConfig, len(systems))
	for name, host := range systems {
		cfgSystems[name] = config.SAPConfig{Host: host, Client: "100", User: "U", Password: "P"}
	}
	return &config.Config{DefaultSystem: defaultSystem, Systems: cfgSystems}
}

func TestRegistryDefaultSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"></adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": "http://nowhere"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "dev" {
		t.Errorf("active: got %q, want %q", registry.ActiveName(), "dev")
	}
}

func TestRegistrySelectSwitchesSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL, "prod": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := registry.Select("prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if registry.ActiveName() != "prod" {
		t.Errorf("active after select: got %q", registry.ActiveName())
	}
	if msg == "" {
		t.Error("expected non-empty display message")
	}
}

func TestRegistrySelectUnknownSystem(t *testing.T) {
	cfg := makeRegistryConfig(map[string]string{"dev": "http://dev"}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.Select("nonexistent")
	if err == nil {
		t.Error("expected error for unknown system")
	}
}

func TestRegistryDelegatesGetSource(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/discovery" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		called = true
		w.Header().Set("ETag", "etag123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("REPORT ZTEST."))
	}))
	defer srv.Close()

	cfg := makeRegistryConfig(map[string]string{"dev": srv.URL}, "dev")
	registry, err := adt.NewClientRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = registry.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST/source/main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected GetSource to delegate to underlying client")
	}
}
