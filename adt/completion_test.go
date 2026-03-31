package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestGetCompletions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/abapsource/codecompletion/proposal" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("line") != "5" {
			t.Errorf("line: got %q", r.URL.Query().Get("line"))
		}
		if r.URL.Query().Get("column") != "10" {
			t.Errorf("column: got %q", r.URL.Query().Get("column"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<codecompletion:completions xmlns:codecompletion="http://www.sap.com/adt/codecompletion">
  <codecompletion:completion codecompletion:text="METHOD" codecompletion:description="ABAP Keyword"/>
  <codecompletion:completion codecompletion:text="MESSAGE" codecompletion:description="ABAP Keyword"/>
</codecompletion:completions>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	items, err := client.GetCompletions(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.", 5, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Text != "METHOD" {
		t.Errorf("item[0].Text: got %q", items[0].Text)
	}
}
