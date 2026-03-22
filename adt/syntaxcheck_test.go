package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/sapadt-mcp/adt"
	"github.com/dachner/sapadt-mcp/config"
)

func TestSyntaxCheckWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/checkruns" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0"?>
<chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist">
  <chkl:message chkl:type="E" chkl:typeText="Error">
    <chkl:shortTextElements><chkl:shortText>Field "FOO" is unknown.</chkl:shortText></chkl:shortTextElements>
    <chkl:line>42</chkl:line>
    <chkl:column>5</chkl:column>
  </chkl:message>
</chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "E" {
		t.Errorf("type: got %q", msgs[0].Type)
	}
	if msgs[0].Line != 42 {
		t.Errorf("line: got %d", msgs[0].Line)
	}
	if msgs[0].Column != 5 {
		t.Errorf("column: got %d", msgs[0].Column)
	}
}

func TestSyntaxCheckClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))
	}))
	defer srv.Close()

	cfg := &config.Config{SAP: config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for clean check, got %d", len(msgs))
	}
}
