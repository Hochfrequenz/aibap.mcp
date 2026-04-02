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
		source: "REPORT z_adt_mcp_test_report.\n" +
			"DATA: lv_test TYPE string.\n" +
			"lv_test = 'Hello debugger'.\n" +
			"WRITE: / lv_test.\n" +
			"\n" +
			"CLASS lcl_test DEFINITION FOR TESTING RISK LEVEL HARMLESS DURATION SHORT.\n" +
			"  PRIVATE SECTION.\n" +
			"    METHODS test_hello FOR TESTING.\n" +
			"ENDCLASS.\n" +
			"\n" +
			"CLASS lcl_test IMPLEMENTATION.\n" +
			"  METHOD test_hello.\n" +
			"    DATA: lv_val TYPE string.\n" +
			"    lv_val = 'test'.\n" +
			"    cl_abap_unit_assert=>assert_equals( act = lv_val exp = 'test' ).\n" +
			"  ENDMETHOD.\n" +
			"ENDCLASS.\n",
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
		source: "CLASS zcl_adt_mcp_test_nounits DEFINITION PUBLIC FINAL CREATE PUBLIC.\n" +
			"  PUBLIC SECTION.\n" +
			"    INTERFACES zif_adt_mcp_test.\n" +
			"ENDCLASS.\n" +
			"\n" +
			"CLASS zcl_adt_mcp_test_nounits IMPLEMENTATION.\n" +
			"  METHOD zif_adt_mcp_test~get_name.\n" +
			"    rv_name = 'nounits'.\n" +
			"  ENDMETHOD.\n" +
			"ENDCLASS.\n",
	},
	{
		objType:     "CLAS",
		name:        "ZCL_ADT_MCP_TEST_UNITS",
		description: "MCP Server Test Class (with unit tests)",
		objectURI:   "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_UNITS",
		// Test methods require a separate test include (/includes/testclasses)
		// which we cannot set via SetSource. On S4 with Z_ADT_MCP_TEST package,
		// the test include is maintained via abapGit. On ECC ($TMP), only the
		// main source is set — RunUnitTests_WithTests will find no tests.
		source: "CLASS zcl_adt_mcp_test_units DEFINITION PUBLIC FINAL CREATE PUBLIC.\n" +
			"  PUBLIC SECTION.\n" +
			"    INTERFACES zif_adt_mcp_test.\n" +
			"ENDCLASS.\n" +
			"\n" +
			"CLASS zcl_adt_mcp_test_units IMPLEMENTATION.\n" +
			"  METHOD zif_adt_mcp_test~get_name.\n" +
			"    rv_name = 'ZCL_ADT_MCP_TEST_UNITS'.\n" +
			"  ENDMETHOD.\n" +
			"ENDCLASS.\n",
	},
}

// TestMain runs setup before all integration tests and teardown after.
// Fixtures created during setup are deleted after all tests complete.
func TestMain(m *testing.M) {
	cfg := integrationConfig()
	if cfg.Host == "" {
		// Not an integration run — just execute tests normally (they'll skip).
		os.Exit(m.Run())
	}

	client := adt.NewClient(cfg)
	ctx := context.Background()

	fmt.Println("=== Integration test setup: ensuring fixtures exist ===")
	created, transport, err := setupFixtures(ctx, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SETUP FAILED: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Teardown: delete objects we created (reverse order to handle dependencies).
	if len(created) > 0 {
		fmt.Println("=== Integration test teardown: deleting created fixtures ===")
		for i := len(created) - 1; i >= 0; i-- {
			f := created[i]
			if err := client.DeleteObject(ctx, f.objectURI, "", transport); err != nil {
				fmt.Printf("  [delete failed] %s %s: %v\n", f.objType, f.name, err)
			} else {
				fmt.Printf("  [deleted] %s %s\n", f.objType, f.name)
			}
		}
		// Release the transport so objects don't stay locked for future runs.
		if transport != "" {
			if err := client.ReleaseTransport(ctx, transport); err != nil {
				fmt.Printf("  [transport release failed] %s: %v\n", transport, err)
			} else {
				fmt.Printf("  [transport released] %s\n", transport)
			}
		}
	}

	os.Exit(code)
}

// fixturePackages lists packages to try when creating fixtures, in order of preference.
// Z_ADT_MCP_TEST is the dedicated test package on S4; $TMP is the fallback for ECC
// or when Z_ADT_MCP_TEST doesn't exist.
var fixturePackages = []string{"Z_ADT_MCP_TEST", "$TMP"}

// setupFixtures creates any missing test objects, sets their source, and activates them.
// Returns the fixtures that were created (for teardown).
func setupFixtures(ctx context.Context, client adt.Client) ([]fixtureObject, string, error) {
	// Verify that the companion test package exists on the SAP system.
	// We only warn (not fail) so that tests which don't need the package can still run.
	if _, err := client.BrowsePackage(ctx, testPackage); err != nil {
		fmt.Printf("WARNING: package %s not found — some integration tests will fail. See https://github.com/Hochfrequenz/Z_ADT_MCP_TEST\n", testPackage)
	}

	// For non-local packages, create a transport to hold the new objects.
	// The transport is released at the end so objects don't stay locked.
	var fixtureTransport string
	for _, pkg := range fixturePackages {
		if pkg == "$TMP" {
			continue
		}
		tr, err := client.CreateTransport(ctx, "K", "DUM", "MCP integration test fixtures", pkg)
		if err == nil {
			fixtureTransport = tr
			fmt.Printf("  [transport] %s for package %s\n", tr, pkg)
			break
		}
	}

	var created []fixtureObject
	var createdURIs []string
	for _, f := range testFixtures {
		// Check if object already exists.
		_, err := client.GetObjectInfo(ctx, f.objectURI)
		if err == nil {
			fmt.Printf("  [exists] %s %s\n", f.objType, f.name)
			continue
		}

		// Try each package in order — Z_ADT_MCP_TEST first (S4), $TMP as fallback.
		var createErr error
		var usedPkg string
		for _, pkg := range fixturePackages {
			transport := ""
			if pkg != "$TMP" {
				transport = fixtureTransport
			}
			createErr = client.CreateObject(ctx, f.objType, f.name, pkg, f.description, transport)
			if createErr == nil {
				usedPkg = pkg
				break
			}
		}
		if createErr != nil {
			if _, infoErr := client.GetObjectInfo(ctx, f.objectURI); infoErr == nil {
				fmt.Printf("  [exists, create skipped] %s %s: %v\n", f.objType, f.name, createErr)
				continue
			}
			fmt.Printf("  [unavailable] %s %s: %v\n", f.objType, f.name, createErr)
			continue
		}
		fmt.Printf("  [created] %s %s in package %s\n", f.objType, f.name, usedPkg)

		// Set source: lock → get etag → set source → unlock.
		if err := setFixtureSource(ctx, client, f); err != nil {
			return created, fixtureTransport, fmt.Errorf("set source %s: %w", f.name, err)
		}
		fmt.Printf("  [source set] %s\n", f.name)

		created = append(created, f)
		createdURIs = append(createdURIs, f.objectURI)
	}

	// Activate newly created objects.
	if len(createdURIs) > 0 {
		result, err := client.ActivateObjects(ctx, createdURIs)
		if err != nil {
			return created, fixtureTransport, fmt.Errorf("activate: %w", err)
		}
		if !result.Success {
			for _, msg := range result.Messages {
				fmt.Printf("  [activation %s] %s: %s\n", msg.Type, msg.Text, msg.ObjectURI)
			}
			return created, fixtureTransport, fmt.Errorf("activation failed with %d messages", len(result.Messages))
		}
		fmt.Printf("  [activated] %d objects\n", len(createdURIs))
	}

	return created, fixtureTransport, nil
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
