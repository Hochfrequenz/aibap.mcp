//go:build integration

package adt_test

import (
	"os"
	"testing"

	"github.com/dachner/mcp-server-abap/adt"
	"github.com/dachner/mcp-server-abap/config"
)

func newIntegrationClient(t *testing.T) adt.Client {
	t.Helper()
	host := os.Getenv("SAP_INTEGRATION_HOST")
	if host == "" {
		t.Skip("SAP_INTEGRATION_HOST not set, skipping integration test")
	}
	user := os.Getenv("SAP_INTEGRATION_USER")
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
