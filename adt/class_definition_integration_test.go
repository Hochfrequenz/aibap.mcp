//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
)

func TestGetClassDefinition_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.GetClassDefinition(ctx, testClassURI)
	if err != nil {
		t.Fatalf("GetClassDefinition: %v", err)
	}
	if result.Source == "" {
		t.Fatal("empty source")
	}
	if result.ETag == "" {
		t.Fatal("empty ETag")
	}

	// Definition should contain CLASS ... DEFINITION and ENDCLASS
	src := strings.ToUpper(result.Source)
	if !strings.Contains(src, "DEFINITION") {
		t.Error("definition source does not contain DEFINITION keyword")
	}
	if !strings.Contains(src, "ENDCLASS") {
		t.Error("definition source does not contain ENDCLASS")
	}

	// Definition should NOT contain IMPLEMENTATION
	if strings.Contains(src, "IMPLEMENTATION") {
		t.Error("definition source unexpectedly contains IMPLEMENTATION")
	}

	// Compare with full source to verify token savings
	full, err := client.GetSource(ctx, testClassURI)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	savings := 100 - (100*len(result.Source))/len(full.Source)
	t.Logf("definition: %d bytes, full: %d bytes, savings: %d%%",
		len(result.Source), len(full.Source), savings)
	t.Logf("definition source:\n%s", result.Source)
}
