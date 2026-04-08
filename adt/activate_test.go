package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func TestActivateObjectSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/activation" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			// Empty messages = successful activation
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist"><chkl:properties checkExecuted="false" activationExecuted="false" generationExecuted="true"/></chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObjects(context.Background(), []string{"/sap/bc/adt/programs/programs/ZTEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result.Messages))
	}
}

func TestActivateObjectWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/activation" {
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			// Real SAP activation error response format
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <msg objDescr="Programm ZTEST" type="E" line="1"
       href="/sap/bc/adt/programs/programs/ztest/source/main#start=5,0"
       forceSupported="true">
    <shortText><txt>Syntax error in line 5</txt></shortText>
  </msg>
</chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObjects(context.Background(), []string{"/sap/bc/adt/programs/programs/ZTEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false when error messages present")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Type != "E" {
		t.Errorf("message type: got %q, want E", result.Messages[0].Type)
	}
	if result.Messages[0].Text != "Syntax error in line 5" {
		t.Errorf("message text: got %q", result.Messages[0].Text)
	}
	if result.Messages[0].ObjectURI != "/sap/bc/adt/programs/programs/ztest/source/main#start=5,0" {
		t.Errorf("object URI: got %q", result.Messages[0].ObjectURI)
	}
}
