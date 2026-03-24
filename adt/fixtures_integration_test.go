//go:build integration

package adt_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

// fixtureObject describes a test object to be created on the SAP system.
type fixtureObject struct {
	objType     string // PROG, CLAS, INTF
	name        string
	description string
	objectURI   string // without /source/main
	source      string
}

// testFixtures defines all objects required by the integration test suite.
// Order matters: interfaces before classes that implement them.
var testFixtures = []fixtureObject{
	{
		objType:     "PROG",
		name:        "Z_ADT_MCP_TEST_REPORT",
		description: "MCP Server Integration Test Report",
		objectURI:   "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_REPORT",
		source:      "REPORT z_adt_mcp_test_report.\nWRITE: / 'Hello from MCP integration test'.\n",
	},
	{
		objType:     "PROG",
		name:        "Z_ADT_MCP_TEST_SYNWARN",
		description: "MCP Server Syntax Warning Test",
		objectURI:   "/sap/bc/adt/programs/programs/Z_ADT_MCP_TEST_SYNWARN",
		source:      "REPORT z_adt_mcp_test_synwarn.\nDATA: lv_unused TYPE string.\nWRITE: / 'test'.\n",
	},
	{
		objType:     "INTF",
		name:        "ZIF_ADT_MCP_TEST",
		description: "MCP Server Integration Test Interface",
		objectURI:   "/sap/bc/adt/oo/interfaces/ZIF_ADT_MCP_TEST",
		source:      "INTERFACE zif_adt_mcp_test PUBLIC.\n  METHODS get_name RETURNING VALUE(rv_name) TYPE string.\nENDINTERFACE.\n",
	},
	{
		objType:     "CLAS",
		name:        "ZCL_ADT_MCP_TEST_NOUNITS",
		description: "MCP Server Test Class (no unit tests)",
		objectURI:   "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_NOUNITS",
		source: `CLASS zcl_adt_mcp_test_nounits DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    INTERFACES zif_adt_mcp_test.
ENDCLASS.

CLASS zcl_adt_mcp_test_nounits IMPLEMENTATION.
  METHOD zif_adt_mcp_test~get_name.
    rv_name = 'nounits'.
  ENDMETHOD.
ENDCLASS.
`,
	},
	{
		objType:     "CLAS",
		name:        "ZCL_ADT_MCP_TEST_UNITS",
		description: "MCP Server Test Class (with unit tests)",
		objectURI:   "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_UNITS",
		source: `CLASS zcl_adt_mcp_test_units DEFINITION PUBLIC FINAL CREATE PUBLIC.
  PUBLIC SECTION.
    INTERFACES zif_adt_mcp_test.
  PRIVATE SECTION.
    METHODS test_pass FOR TESTING.
    METHODS test_fail FOR TESTING.
ENDCLASS.

CLASS zcl_adt_mcp_test_units IMPLEMENTATION.
  METHOD zif_adt_mcp_test~get_name.
    rv_name = 'units'.
  ENDMETHOD.
  METHOD test_pass.
    cl_abap_unit_assert=>assert_equals( act = 1 exp = 1 ).
  ENDMETHOD.
  METHOD test_fail.
    cl_abap_unit_assert=>assert_equals( act = 1 exp = 2 msg = 'intentional fail' ).
  ENDMETHOD.
ENDCLASS.
`,
	},
}

// TestMain runs setup before all integration tests.
// Test fixtures are persistent — they are created if missing but never deleted,
// because deleting objects in transportable packages leaves them locked in the
// transport system and prevents recreation on the next run.
func TestMain(m *testing.M) {
	if os.Getenv("SAP_INTEGRATION_HOST") == "" {
		// Not an integration run — just execute tests normally (they'll skip).
		os.Exit(m.Run())
	}

	client := newIntegrationClientFromEnv()
	ctx := context.Background()

	fmt.Println("=== Integration test setup: ensuring fixtures exist ===")
	if err := setupFixtures(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "SETUP FAILED: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// newIntegrationClientFromEnv creates a client without *testing.T (for TestMain).
func newIntegrationClientFromEnv() adt.Client {
	cfg := integrationConfig()
	return adt.NewClient(cfg)
}

// setupFixtures creates any missing test objects, sets their source, and activates them.
func setupFixtures(ctx context.Context, client adt.Client) error {
	var created []string
	for _, f := range testFixtures {
		// Check if object already exists.
		_, err := client.GetObjectInfo(ctx, f.objectURI)
		if err == nil {
			fmt.Printf("  [exists] %s %s\n", f.objType, f.name)
			continue
		}

		// Create. If it fails (e.g. object locked in a transport from a previous
		// run), warn and continue — tests that need this object will skip/fail
		// individually rather than blocking the entire suite.
		if err := client.CreateObject(ctx, f.objType, f.name, "$TMP", f.description, ""); err != nil {
			if _, infoErr := client.GetObjectInfo(ctx, f.objectURI); infoErr == nil {
				fmt.Printf("  [exists, create skipped] %s %s: %v\n", f.objType, f.name, err)
				continue
			}
			fmt.Printf("  [unavailable] %s %s: %v\n", f.objType, f.name, err)
			continue
		}
		fmt.Printf("  [created] %s %s\n", f.objType, f.name)

		// Set source: lock → get etag → set source → unlock.
		if err := setFixtureSource(ctx, client, f); err != nil {
			return fmt.Errorf("set source %s: %w", f.name, err)
		}
		fmt.Printf("  [source set] %s\n", f.name)

		created = append(created, f.objectURI)
	}

	// Activate newly created objects.
	if len(created) > 0 {
		result, err := client.ActivateObjects(ctx, created)
		if err != nil {
			return fmt.Errorf("activate: %w", err)
		}
		if !result.Success {
			for _, msg := range result.Messages {
				fmt.Printf("  [activation %s] %s: %s\n", msg.Type, msg.Text, msg.ObjectURI)
			}
			return fmt.Errorf("activation failed with %d messages", len(result.Messages))
		}
		fmt.Printf("  [activated] %d objects\n", len(created))
	}

	return nil
}

// setFixtureSource locks an object, writes its source, and unlocks it.
func setFixtureSource(ctx context.Context, client adt.Client, f fixtureObject) error {
	lockHandle, err := client.LockObject(ctx, f.objectURI)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer func() {
		_ = client.UnlockObject(ctx, f.objectURI, lockHandle)
	}()

	src, err := client.GetSource(ctx, f.objectURI)
	if err != nil {
		return fmt.Errorf("get source for etag: %w", err)
	}

	_, err = client.SetSource(ctx, f.objectURI, f.source, lockHandle, "", src.ETag)
	if err != nil {
		return fmt.Errorf("set source: %w", err)
	}
	return nil
}

