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
// Created automatically by TestMain via setupFixtures.
const testReportURI = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT"

// integrationConfig builds a SAPConfig from environment variables.
func integrationConfig() config.SAPConfig {
	return config.SAPConfig{
		Host:          strings.TrimSpace(os.Getenv("SAP_INTEGRATION_HOST")),
		User:          strings.TrimSpace(os.Getenv("SAP_INTEGRATION_USER")),
		Password:      os.Getenv("SAP_INTEGRATION_PASSWORD"),
		Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
		TLSSkipVerify: true,
	}
}

// newIntegrationClient creates a real ADT client from environment variables.
// Do not log the returned client or its config — they contain credentials.
func newIntegrationClient(t *testing.T) adt.Client {
	t.Helper()
	cfg := integrationConfig()
	if cfg.Host == "" {
		t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
	}
	if cfg.User == "" {
		t.Fatal("SAP_INTEGRATION_USER must be set when SAP_INTEGRATION_HOST is set")
	}
	if cfg.Password == "" {
		t.Fatal("SAP_INTEGRATION_PASSWORD must be set when SAP_INTEGRATION_HOST is set")
	}
	return adt.NewClient(cfg)
}
