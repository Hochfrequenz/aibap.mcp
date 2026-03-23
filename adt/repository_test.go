package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func TestBrowsePackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/repository/nodestructure" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		if q.Get("parent_name") != "ZPACKAGE" {
			t.Errorf("parent_name: got %q", q.Get("parent_name"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT" adtcore:type="PROG/P" adtcore:name="ZREPORT" adtcore:description="My Report" adtcore:packageName="ZPACKAGE"/>
</adtcore:objectReferences>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	results, err := client.BrowsePackage(context.Background(), "ZPACKAGE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ZREPORT" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestGetObjectInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/programs/programs/ZREPORT" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<adtcore:objectReference xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:uri="/sap/bc/adt/programs/programs/ZREPORT"
  adtcore:type="PROG/P"
  adtcore:name="ZREPORT"
  adtcore:description="My Report"
  adtcore:packageName="ZPACKAGE"/>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/programs/programs/ZREPORT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZREPORT" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "PROG/P" {
		t.Errorf("type: got %q", info.Type)
	}
}
