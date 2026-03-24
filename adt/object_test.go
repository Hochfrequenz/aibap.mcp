package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestCreateObjectProgram(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.CreateObject(context.Background(), "PROG", "ZTEST_NEW", "ZPACKAGE", "Test program", "DEVK900001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %q", gotMethod)
	}
	if gotPath != "/sap/bc/adt/programs/programs" {
		t.Errorf("path: got %q", gotPath)
	}
}

func TestCreateObjectUnsupportedType(t *testing.T) {
	cfg := config.SAPConfig{Host: "http://localhost", User: "U", Password: "P"}
	client := adt.NewClient(cfg)

	err := client.CreateObject(context.Background(), "TABL", "ZTABLE", "ZPACKAGE", "Table", "")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestDeleteObject(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.DeleteObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "DEVK900001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %q", gotMethod)
	}
	if gotPath != "/sap/bc/adt/programs/programs/ZTEST" {
		t.Errorf("path: got %q", gotPath)
	}
}
