//go:build integration

package adt_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

// Test fixture URIs — all objects live in package Z_ADT_MCP_TEST (S4) or $TMP (ECC).
// Created automatically by TestMain via setupFixtures. See also fixtures_integration_test.go.
const (
	testReportURI    = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT"
	testSynWarnURI   = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_SYNWARN"
	testInterfaceURI = "/sap/bc/adt/oo/interfaces/ZIF_ADT_MCP_TEST"
	testClassURI     = "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_UNITS"
	testClassNoTests = "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_NOUNITS"
)

// integrationConfig builds a SAPSystem. It tries, in order:
//  1. JSON config file (same as MCP server) + SAP_INTEGRATION_SYSTEM env var
//  2. Fallback env vars: SAP_INTEGRATION_HOST, SAP_INTEGRATION_USER, etc.
//
// JSON paths searched: SAP_CONFIG_FILE env var, ~/.config/sap-mcp/systems.json
func integrationConfig() config.SAPSystem {
	// Try JSON config first
	if cfg, ok := integrationConfigFromFile(); ok {
		return cfg
	}
	// Fallback to legacy env vars
	return config.SAPSystem{
		Host:          strings.TrimSpace(os.Getenv("SAP_INTEGRATION_HOST")),
		User:          strings.TrimSpace(os.Getenv("SAP_INTEGRATION_USER")),
		Password:      os.Getenv("SAP_INTEGRATION_PASSWORD"),
		Client:        os.Getenv("SAP_INTEGRATION_CLIENT"),
		TLSSkipVerify: true,
	}
}

func integrationConfigFromFile() (config.SAPSystem, bool) {
	paths := []string{os.Getenv("SAP_CONFIG_FILE")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.config/sap-mcp/systems.json")
	}

	var cfg *config.AppConfig
	for _, p := range paths {
		if p == "" {
			continue
		}
		var err error
		cfg, err = config.Load(p)
		if err == nil {
			break
		}
	}
	if cfg == nil {
		return config.SAPSystem{}, false
	}

	// Pick system: SAP_INTEGRATION_SYSTEM env var, or default_system from YAML
	systemName := os.Getenv("SAP_INTEGRATION_SYSTEM")
	if systemName == "" {
		systemName = cfg.DefaultSystem
	}

	// Check whitelist — only run tests against explicitly allowed systems
	if !cfg.IsTestSystem(systemName) {
		return config.SAPSystem{}, false
	}

	sys, ok := cfg.Systems[systemName]
	if !ok {
		return config.SAPSystem{}, false
	}
	sys.TLSSkipVerify = true
	return sys, true
}

// newIntegrationClient creates a real ADT client from environment variables.
// Do not log the returned client or its config — they contain credentials.
func newIntegrationClient(t *testing.T) adt.Client {
	t.Helper()
	cfg := integrationConfig()
	if cfg.Host == "" {
		t.Skip("No SAP config found — set SAP_CONFIG_FILE or SAP_INTEGRATION_HOST")
	}
	if cfg.User == "" {
		t.Fatal("SAP user not configured — check YAML config or SAP_INTEGRATION_USER")
	}
	if cfg.Password == "" {
		t.Fatal("SAP password not configured — check YAML config or SAP_INTEGRATION_PASSWORD")
	}
	return adt.NewClient(cfg)
}

// setupDisposableReport creates a $TMP program with the given name and initial
// source, activates it, and registers cleanup to delete it after the test.
// Returns the object URI. No transport is needed for $TMP objects.
func setupDisposableReport(t *testing.T, client adt.Client, name, initialSource string) string {
	t.Helper()
	ctx := context.Background()
	objectURI := "/sap/bc/adt/programs/programs/" + name

	err := client.CreateObject(ctx, "PROG", name, "$TMP",
		fmt.Sprintf("Integration test (%s)", time.Now().Format("2006-01-02")), "")
	if err != nil {
		// Object may already exist from a previous aborted run — try to use it.
		if _, infoErr := client.GetObjectInfo(ctx, objectURI); infoErr != nil {
			t.Fatalf("CreateObject %s failed and object does not exist: %v", name, err)
		}
		t.Logf("object %s already exists, reusing", name)
	}

	// Set initial source so the object is in a known state.
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject for setup of %s failed: %v", name, err)
	}
	src, err := client.GetSource(ctx, objectURI)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		t.Fatalf("GetSource for setup of %s failed: %v", name, err)
	}
	_, err = client.SetSource(ctx, objectURI, initialSource, lockHandle, "", src.ETag)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, lockHandle)
		t.Fatalf("SetSource for setup of %s failed: %v", name, err)
	}
	_ = client.UnlockObject(ctx, objectURI, lockHandle)

	// Activate so tests start from a known-good active state.
	result, err := client.ActivateObjects(ctx, []string{objectURI})
	if err != nil {
		t.Fatalf("ActivateObjects for setup of %s failed: %v", name, err)
	}
	if !result.Success {
		t.Fatalf("activation of %s failed: %d messages", name, len(result.Messages))
	}

	t.Cleanup(func() {
		if err := client.DeleteObject(context.Background(), objectURI, "", ""); err != nil {
			t.Logf("WARNING: cleanup failed to delete %s: %v", name, err)
		}
	})

	return objectURI
}
