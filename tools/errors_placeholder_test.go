package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMatchHint_UnresolvedPlaceholder pins that adtler's refusal of an
// unresolved ${env:VAR} credential gets a configuration hint, not an
// authentication or server-error hint (#575). adtler refuses before sending
// anything, so the hint must not suggest a failed logon or a retry.
func TestMatchHint_UnresolvedPlaceholder(t *testing.T) {
	err := fmt.Errorf("GetSource: adt: password is an unresolved ${env:SAP_PASSWORD} placeholder: %w", adt.ErrUnresolvedPlaceholder)
	hint := matchHint(err)
	if hint != unresolvedPlaceholderHint {
		t.Fatalf("hint: got %q, want unresolvedPlaceholderHint", hint)
	}
}

// TestErrorResult_UnresolvedPlaceholderFromClient drives a real adtler client
// whose password is a placeholder, so the test fails if adtler stops wrapping
// the sentinel or starts sending the request anyway.
func TestErrorResult_UnresolvedPlaceholderFromClient(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := adt.NewClient(config.SAPSystem{Host: srv.URL, Client: "100", User: "DEVUSER", Password: "${env:SAP_PASSWORD}"})
	_, err := client.GetSource(context.Background(), "/sap/bc/adt/programs/programs/z_test")
	if err == nil {
		t.Fatal("GetSource succeeded, want the placeholder refusal")
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("requests sent: got %d, want 0 (each would count as a failed SAP logon)", n)
	}
	text := errorResult(err).Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Hint: "+unresolvedPlaceholderHint) {
		t.Errorf("error result should carry the configuration hint, got: %s", text)
	}
}
