package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func TestGetSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			w.Header().Set("ETag", `"etag-abc123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("REPORT ZTEST.\nWRITE 'Hello'."))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
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

func TestGetIncludeSource(t *testing.T) {
	// ZCL_TEST padded to 30 chars = ZCL_TEST=======================
	wantPath := "/sap/bc/adt/oo/classes/zcl_test/includes/testclasses"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == wantPath {
			w.Header().Set("ETag", `"etag-incl"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("CLASS lcl_test DEFINITION FOR TESTING.\nENDCLASS."))
			return
		}
		t.Logf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.GetIncludeSource(context.Background(), "/sap/bc/adt/oo/classes/zcl_test", "testclasses")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != "CLASS lcl_test DEFINITION FOR TESTING.\nENDCLASS." {
		t.Errorf("source: got %q", result.Source)
	}
	if result.ETag != `"etag-incl"` {
		t.Errorf("etag: got %q", result.ETag)
	}
}

func TestSetIncludeSource(t *testing.T) {
	wantPath := "/sap/bc/adt/oo/classes/zcl_test/includes/testclasses"
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		gotMethod = r.Method
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("ETag", `"etag-new"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	newETag, err := client.SetIncludeSource(context.Background(),
		"/sap/bc/adt/oo/classes/zcl_test", "testclasses",
		"CLASS lcl_test DEFINITION FOR TESTING.\nENDCLASS.", "", "", `"etag-old"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method: got %q, want PUT", gotMethod)
	}
	if gotPath != wantPath {
		t.Errorf("path: got %q, want %q", gotPath, wantPath)
	}
	if gotBody != "CLASS lcl_test DEFINITION FOR TESTING.\nENDCLASS." {
		t.Errorf("body: got %q", gotBody)
	}
	if newETag != `"etag-new"` {
		t.Errorf("etag: got %q", newETag)
	}
}

func TestSetIncludeSource_NoETag(t *testing.T) {
	var gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotIfMatch = r.Header.Get("If-Match")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	// Empty etag = initial write on empty include
	_, err := client.SetIncludeSource(context.Background(),
		"/sap/bc/adt/oo/classes/zcl_test", "testclasses",
		"CLASS lcl_test DEFINITION FOR TESTING.\nENDCLASS.", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotIfMatch != "" {
		t.Errorf("If-Match should be empty for initial write, got %q", gotIfMatch)
	}
}

func TestSetSource(t *testing.T) {
	var gotMethod, gotIfMatch, gotContentType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			gotMethod = r.Method
			gotIfMatch = r.Header.Get("If-Match")
			gotContentType = r.Header.Get("Content-Type")
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.SetSource(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ZTEST.\nNEW CODE.", "", "", `"etag-abc123"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method: got %q, want PUT", gotMethod)
	}
	if gotIfMatch != `"etag-abc123"` {
		t.Errorf("If-Match: got %q", gotIfMatch)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want %q", gotContentType, "text/plain; charset=utf-8")
	}
	if gotBody != "REPORT ZTEST.\nNEW CODE." {
		t.Errorf("body: got %q", gotBody)
	}
}
