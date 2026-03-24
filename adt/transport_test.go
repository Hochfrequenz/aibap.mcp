package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestGetTransportRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/cts/transportrequests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<tm:root xmlns:tm="http://www.sap.com/cts/transport">
  <tm:workbenchRequests>
    <tm:workbenchRequest tm:number="DEVK900123" tm:owner="DEVELOPER" tm:shortDescription="Feature transport" tm:status="D"/>
  </tm:workbenchRequests>
</tm:root>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	transports, err := client.GetTransportRequests(context.Background(), "", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(transports))
	}
	if transports[0].Number != "DEVK900123" {
		t.Errorf("number: got %q", transports[0].Number)
	}
	if transports[0].Status != "D" {
		t.Errorf("status: got %q", transports[0].Status)
	}
}

func TestAddToTransport(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.AddToTransport(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "DEVK900123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "/sap/bc/adt/cts/transportrequests/DEVK900123/abaptransportcomponents"
	if gotPath != expected {
		t.Errorf("path: got %q, want %q", gotPath, expected)
	}
}
