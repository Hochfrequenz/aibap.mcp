package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestSearchObjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/informationsystem/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("operation") != "quickSearch" {
			t.Errorf("operation: got %q", q.Get("operation"))
		}
		if q.Get("query") != "ZTEST*" {
			t.Errorf("query: got %q", q.Get("query"))
		}
		if q.Get("maxResults") != "10" {
			t.Errorf("maxResults: got %q", q.Get("maxResults"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZTEST_REPORT" adtcore:type="PROG/P" adtcore:name="ZTEST_REPORT" adtcore:description="Test Report" adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	results, err := client.SearchObjects(context.Background(), "ZTEST*", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "ZTEST_REPORT" {
		t.Errorf("name: got %q", results[0].Name)
	}
	if results[0].PackageName != "ZPACKAGE" {
		t.Errorf("package: got %q", results[0].PackageName)
	}
}

func TestWhereUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/informationsystem/usageReferences" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("adtObjectUri") == "" {
			t.Error("expected adtObjectUri parameter")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZCALLER" adtcore:type="PROG/P" adtcore:name="ZCALLER" adtcore:description="Caller"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	results, err := client.WhereUsed(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ZCALLER" {
		t.Errorf("unexpected results: %+v", results)
	}
}
