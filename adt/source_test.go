package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func TestGetSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			w.Header().Set("ETag", `"etag-abc123"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("REPORT ZTEST.\nWRITE 'Hello'."))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.GetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != "REPORT ZTEST.\nWRITE 'Hello'." {
		t.Errorf("source: got %q", result.Source)
	}
	if result.ETag != `"etag-abc123"` {
		t.Errorf("etag: got %q", result.ETag)
	}
}

func TestSetSource(t *testing.T) {
	var gotMethod, gotIfMatch, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/compatibility/product" {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			gotMethod = r.Method
			gotIfMatch = r.Header.Get("If-Match")
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.\nNEW CODE.", `"etag-abc123"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method: got %q, want PUT", gotMethod)
	}
	if gotIfMatch != `"etag-abc123"` {
		t.Errorf("If-Match: got %q", gotIfMatch)
	}
	if gotBody != "REPORT ZTEST.\nNEW CODE." {
		t.Errorf("body: got %q", gotBody)
	}
}
