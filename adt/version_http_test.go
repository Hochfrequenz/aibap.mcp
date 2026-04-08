package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func TestGetVersionHistory_AcceptHeader(t *testing.T) {
	var gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/programs/programs/ZTEST/source/main/versions" {
			gotAccept = r.Header.Get("Accept")
			w.Header().Set("Content-Type", "application/atom+xml;type=feed")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>00001</id>
    <updated>2025-01-15T10:30:00Z</updated>
    <title>Version 1</title>
    <author><name>DEVELOPER</name></author>
    <content src="/sap/bc/adt/programs/programs/ZTEST/source/main/versions/20250115103000/00001/content"/>
  </entry>
</feed>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	versions, err := client.GetVersionHistory(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0].Include != "" {
		t.Errorf("expected empty include for program, got %q", versions[0].Include)
	}

	wantAccept := "application/atom+xml;type=feed"
	if gotAccept != wantAccept {
		t.Errorf("Accept header: got %q, want %q", gotAccept, wantAccept)
	}
}

func TestGetVersionHistory_ClassIncludes(t *testing.T) {
	var gotPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch path {
		case "/sap/bc/adt/oo/classes/ZCL_TEST/includes/definitions/versions":
			gotPaths = append(gotPaths, path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>00001</id>
    <updated>2025-01-15T10:00:00Z</updated>
    <author><name>DEV1</name></author>
    <content src="/sap/bc/adt/oo/classes/ZCL_TEST/includes/definitions/versions/20250115100000/00001/content"/>
  </entry>
</feed>`))
		case "/sap/bc/adt/oo/classes/ZCL_TEST/includes/implementations/versions":
			gotPaths = append(gotPaths, path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>00002</id>
    <updated>2025-01-16T14:00:00Z</updated>
    <author><name>DEV2</name></author>
    <content src="/sap/bc/adt/oo/classes/ZCL_TEST/includes/implementations/versions/20250116140000/00002/content"/>
  </entry>
</feed>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	versions, err := client.GetVersionHistory(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fetch both includes
	if len(gotPaths) != 2 {
		t.Fatalf("expected 2 requests, got %d: %v", len(gotPaths), gotPaths)
	}

	// Should return versions from both includes
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// Definitions include
	if versions[0].Include != "definitions" {
		t.Errorf("version[0] include: got %q, want %q", versions[0].Include, "definitions")
	}
	if versions[0].Author != "DEV1" {
		t.Errorf("version[0] author: got %q, want DEV1", versions[0].Author)
	}

	// Implementations include
	if versions[1].Include != "implementations" {
		t.Errorf("version[1] include: got %q, want %q", versions[1].Include, "implementations")
	}
	if versions[1].Author != "DEV2" {
		t.Errorf("version[1] author: got %q, want DEV2", versions[1].Author)
	}
}

func TestGetVersionHistory_ClassDoesNotUseSourceMain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// /source/main/versions should NOT be called for classes
		if path == "/sap/bc/adt/oo/classes/ZCL_TEST/source/main/versions" {
			t.Error("should not request /source/main/versions for classes")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Serve empty feeds for includes
		if path == "/sap/bc/adt/oo/classes/ZCL_TEST/includes/definitions/versions" ||
			path == "/sap/bc/adt/oo/classes/ZCL_TEST/includes/implementations/versions" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.GetVersionHistory(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
