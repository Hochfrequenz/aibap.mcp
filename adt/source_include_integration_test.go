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

	// Class includes only exist if the class was edited in Eclipse/SE24/abapGit.
	// Classes created purely via ADT CreateObject don't have include programs.
	// Skip gracefully if includes don't exist.
	includes := []string{"testclasses", "definitions", "implementations"}
	for _, include := range includes {
		t.Run(include, func(t *testing.T) {
			result, err := client.GetIncludeSource(ctx, testClassURI, include)
			if err != nil {
				if strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), "404") {
					t.Skipf("include %s does not exist (class not edited in Eclipse/SE24): %v", include, err)
				}
				t.Fatalf("GetIncludeSource(%s): %v", include, err)
			}
			t.Logf("%s: %d bytes, etag=%s", include, len(result.Source), result.ETag)
		})
	}
}

func TestSetIncludeSource_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// Check if testclasses include exists
	_, err := client.GetIncludeSource(ctx, testClassURI, "testclasses")
	if err != nil {
		t.Skipf("testclasses include does not exist (class not edited in Eclipse/SE24): %v", err)
	}

	// Lock the class (includes share the class lock)
	lockHandle, err := client.LockObject(ctx, testClassURI)
	if err != nil {
		t.Fatalf("LockObject: %v", err)
	}
	defer func() { _ = client.UnlockObject(ctx, testClassURI, lockHandle) }()

	// Read current source
	original, err := client.GetIncludeSource(ctx, testClassURI, "testclasses")
	if err != nil {
		t.Fatalf("GetIncludeSource: %v", err)
	}
	t.Logf("original testclasses: %d bytes", len(original.Source))

	// Write a test class
	testSource := "CLASS lcl_mcp_test DEFINITION FOR TESTING RISK LEVEL HARMLESS DURATION SHORT.\n" +
		"  PRIVATE SECTION.\n" +
		"    METHODS test_pass FOR TESTING.\n" +
		"ENDCLASS.\n" +
		"\n" +
		"CLASS lcl_mcp_test IMPLEMENTATION.\n" +
		"  METHOD test_pass.\n" +
		"    cl_abap_unit_assert=>assert_equals( act = 1 exp = 1 ).\n" +
		"  ENDMETHOD.\n" +
		"ENDCLASS.\n"

	newETag, err := client.SetIncludeSource(ctx, testClassURI, "testclasses", testSource, lockHandle, "", original.ETag)
	if err != nil {
		t.Fatalf("SetIncludeSource: %v", err)
	}
	t.Logf("set testclasses: new etag=%s", newETag)

	// Verify
	updated, err := client.GetIncludeSource(ctx, testClassURI, "testclasses")
	if err != nil {
		t.Fatalf("GetIncludeSource after set: %v", err)
	}
	if !strings.Contains(updated.Source, "lcl_mcp_test") {
		t.Errorf("expected lcl_mcp_test in source, got: %s", updated.Source)
	}

	// Restore original
	_, err = client.SetIncludeSource(ctx, testClassURI, "testclasses", original.Source, lockHandle, "", updated.ETag)
	if err != nil {
		t.Logf("WARNING: could not restore original testclasses: %v", err)
	}
}
