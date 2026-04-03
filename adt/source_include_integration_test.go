//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
)

func TestGetIncludeSource_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	includes := []string{"testclasses", "definitions", "implementations"}
	for _, include := range includes {
		t.Run(include, func(t *testing.T) {
			result, err := client.GetIncludeSource(ctx, testClassURI, include)
			if err != nil {
				if strings.Contains(err.Error(), "404") {
					t.Skipf("include %s not available: %v", include, err)
				}
				t.Fatalf("GetIncludeSource(%s): %v", include, err)
			}
			t.Logf("%s: %d bytes, etag=%s", include, len(result.Source), result.ETag)
		})
	}
}

func TestCreateAndWriteTestInclude_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Lock the class
	lockHandle, err := client.LockObject(ctx, testClassURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	defer func() { _ = client.UnlockObject(ctx, testClassURI, lockHandle) }()

	// Try writing to definitions include with lockHandle as header
	defSrc := "*MCP test local definitions\n"
	newETag, err := client.SetIncludeSource(ctx, testClassURI, "definitions", defSrc, lockHandle, "", "")
	if err != nil {
		t.Fatalf("SetIncludeSource(definitions): %v", err)
	}
	t.Logf("set definitions: etag=%s", newETag)

	// Read back
	result, err := client.GetIncludeSource(ctx, testClassURI, "definitions")
	if err != nil {
		t.Fatalf("GetIncludeSource(definitions): %v", err)
	}
	if !strings.Contains(result.Source, "MCP test") {
		t.Errorf("expected MCP test in source, got: %s", result.Source)
	}
	t.Logf("definitions: %d bytes", len(result.Source))

	// Try creating test include
	t.Log("Creating test include...")
	err = client.CreateTestInclude(ctx, testClassURI, lockHandle, "")
	if err != nil {
		t.Logf("CreateTestInclude: %v", err)
	} else {
		t.Log("CreateTestInclude: OK")

		// Write test class source
		testSource := "CLASS lcl_mcp_test DEFINITION FOR TESTING RISK LEVEL HARMLESS DURATION SHORT.\n" +
			"  PRIVATE SECTION.\n    METHODS test_pass FOR TESTING.\nENDCLASS.\n\n" +
			"CLASS lcl_mcp_test IMPLEMENTATION.\n  METHOD test_pass.\n" +
			"    cl_abap_unit_assert=>assert_equals( act = 1 exp = 1 ).\n  ENDMETHOD.\nENDCLASS.\n"
		_, err = client.SetIncludeSource(ctx, testClassURI, "testclasses", testSource, lockHandle, "", "")
		if err != nil {
			t.Logf("SetIncludeSource(testclasses): %v", err)
		} else {
			t.Log("SetIncludeSource(testclasses): OK")
		}
	}
}
