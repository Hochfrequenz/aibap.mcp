package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
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

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
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
	cfg := sapmcpconfig.SAPSystem{Host: "http://localhost", User: "U", Password: "P"}
	client := adt.NewClient(cfg)

	err := client.CreateObject(context.Background(), "TABL", "ZTABLE", "ZPACKAGE", "Table", "")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestCreatePackage(t *testing.T) {
	var gotPath, gotMethod, gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.CreatePackage(context.Background(), "Z_MY_PKG", "My Package", "TESTUSER", "HOME", "ZS4U", "DEVK900001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %q, want POST", gotMethod)
	}
	if gotPath != "/sap/bc/adt/packages" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotContentType != "application/vnd.sap.adt.packages.v2+xml" {
		t.Errorf("content-type: got %q", gotContentType)
	}
	if !strings.Contains(gotBody, `adtcore:name="Z_MY_PKG"`) {
		t.Errorf("body missing package name: %s", gotBody)
	}
	if !strings.Contains(gotBody, `adtcore:responsible="TESTUSER"`) {
		t.Errorf("body missing responsible: %s", gotBody)
	}
	if !strings.Contains(gotBody, `pak:name="HOME"`) {
		t.Errorf("body missing softwareComponent: %s", gotBody)
	}
	if !strings.Contains(gotBody, `pak:name="ZS4U"`) {
		t.Errorf("body missing transportLayer: %s", gotBody)
	}
}

func TestCreatePackageWithoutTransport(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.CreatePackage(context.Background(), "z_tmp_pkg", "Temp", "testuser", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("expected no query params for local package, got %q", gotQuery)
	}
}

func TestDeleteObject(t *testing.T) {
	var gotDeletePath, gotDeleteMethod, gotIfMatch, gotCorrNr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodGet {
			// ETag fetch for optimistic locking
			w.Header().Set("ETag", "etag-12345")
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<program:abapProgram xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZTEST" adtcore:type="PROG/P"/>`))
			return
		}
		gotDeletePath = r.URL.Path
		gotDeleteMethod = r.Method
		gotIfMatch = r.Header.Get("If-Match")
		gotCorrNr = r.URL.Query().Get("corrNr")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.DeleteObject(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "", "DEVK900001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDeleteMethod != http.MethodDelete {
		t.Errorf("method: got %q", gotDeleteMethod)
	}
	if gotDeletePath != "/sap/bc/adt/programs/programs/ZTEST" {
		t.Errorf("path: got %q", gotDeletePath)
	}
	if gotIfMatch != "etag-12345" {
		t.Errorf("If-Match: got %q, want %q", gotIfMatch, "etag-12345")
	}
	if gotCorrNr != "DEVK900001" {
		t.Errorf("corrNr: got %q, want %q", gotCorrNr, "DEVK900001")
	}
}
