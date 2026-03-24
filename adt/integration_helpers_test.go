//go:build integration

package adt_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

// testReportURI is the editable test report for lock/write/activate tests.
// This object must exist on the SAP system; see testdata/integration_objects.md.
const testReportURI = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT"

// newIntegrationClient creates a real ADT client from environment variables.
// Do not log the returned client or its config — they contain credentials.
func newIntegrationClient(t *testing.T) adt.Client {
	t.Helper()
	host := strings.TrimSpace(os.Getenv("SAP_INTEGRATION_HOST"))
	if host == "" {
		t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
	}
	user := strings.TrimSpace(os.Getenv("SAP_INTEGRATION_USER"))
	if user == "" {
		t.Fatal("SAP_INTEGRATION_USER must be set when SAP_INTEGRATION_HOST is set")
	}
	password := os.Getenv("SAP_INTEGRATION_PASSWORD")
	if password == "" {
		t.Fatal("SAP_INTEGRATION_PASSWORD must be set when SAP_INTEGRATION_HOST is set")
	}
	cfg := config.SAPConfig{
		Host:          host,
		User:          user,
		Password:      password,
		Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
		TLSSkipVerify: true,
	}
	return adt.NewClient(cfg)
}
