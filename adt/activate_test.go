package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func TestActivateObjectSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/activation/activate" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core"/>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
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
		if r.URL.Path == "/sap/bc/adt/activation/activate" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkl:message adtcore:uri="/sap/bc/adt/programs/programs/ZTEST" chkl:type="E">
    <chkl:shortTextElements><chkl:shortText>Syntax error in line 5</chkl:shortText></chkl:shortTextElements>
  </chkl:message>
</chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.ActivateObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
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
}
