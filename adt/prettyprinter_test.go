package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func TestPrettyPrint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/abapsource/prettyprinter" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		// Simulate formatting: uppercase the source
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("REPORT ZTEST.\n  WRITE 'Hello'."))
		_ = len(body) // use body
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.PrettyPrint(context.Background(), "report ztest.\nwrite 'Hello'.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "REPORT ZTEST.\n  WRITE 'Hello'." {
		t.Errorf("formatted: got %q", result)
	}
}
