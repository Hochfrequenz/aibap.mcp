package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestCheckTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/cts/transportchecks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>ZTEST</OBJECTNAME>
      <OPERATION>I</OPERATION>
      <DEVCLASS>ZPACKAGE</DEVCLASS>
      <RESULT>S</RESULT>
      <RECORDING>X</RECORDING>
      <REQUESTS>
        <CTS_REQUEST>
          <REQ_HEADER>
            <TRKORR>DEVK900001</TRKORR>
            <TRFUNCTION>K</TRFUNCTION>
            <TRSTATUS>D</TRSTATUS>
            <AS4TEXT>My transport</AS4TEXT>
          </REQ_HEADER>
        </CTS_REQUEST>
      </REQUESTS>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.CheckTransport(context.Background(), "R3TR", "PROG", "ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "S" {
		t.Errorf("Result: got %q, want S", result.Result)
	}
	if !result.Recording {
		t.Error("expected Recording=true")
	}
	if result.DevClass != "ZPACKAGE" {
		t.Errorf("DevClass: got %q", result.DevClass)
	}
	if len(result.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(result.Requests))
	}
	if result.Requests[0].Number != "DEVK900001" {
		t.Errorf("transport number: got %q", result.Requests[0].Number)
	}
}

func TestGetTransportRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/cts/transportrequests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.sap.adt.transportorganizertree.v1+xml" {
			t.Errorf("Accept header: got %q, want %q", accept, "application/vnd.sap.adt.transportorganizertree.v1+xml")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<tm:root adtcore:name="DEVELOPER"
  xmlns:tm="http://www.sap.com/cts/adt/tm"
  xmlns:adtcore="http://www.sap.com/adt/core">
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
