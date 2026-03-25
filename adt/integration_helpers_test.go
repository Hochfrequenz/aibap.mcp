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

// testReportURI is the editable test report for lock/write/activate tests.
// Created automatically by TestMain via setupFixtures.
const testReportURI = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT"

// testSynWarnURI is a report with an unused variable, guaranteed to produce syntax warnings.
// Created automatically by TestMain via setupFixtures.
const testSynWarnURI = "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_SYNWARN"

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
