//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestRunUnitTests_WithTests_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// testClassURI has passing + failing test methods.
	result, err := client.RunUnitTests(ctx, testClassURI, 30)
	if err != nil {
		t.Fatalf("RunUnitTests failed: %v", err)
	}
	t.Logf("passed=%d failed=%d errors=%d test_cases=%d",
		result.Passed, result.Failed, result.Errors, len(result.TestCases))
	for _, tc := range result.TestCases {
		t.Logf("  %s: passed=%v time=%.3fs messages=%v", tc.Name, tc.Passed, tc.ExecutionTime, tc.Messages)
	}

	if result.Passed+result.Failed == 0 {
		t.Error("expected at least one test case, got none — request body format may be wrong")
	}
}

func TestRunUnitTests_NoTests_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// testClassNoTests has no unit tests.
	result, err := client.RunUnitTests(ctx, testClassNoTests, 30)
	if err != nil {
		t.Fatalf("RunUnitTests failed: %v", err)
	}
	t.Logf("passed=%d failed=%d errors=%d test_cases=%d",
		result.Passed, result.Failed, result.Errors, len(result.TestCases))

	if len(result.TestCases) != 0 {
		t.Errorf("expected 0 test cases for class without tests, got %d", len(result.TestCases))
	}
}
