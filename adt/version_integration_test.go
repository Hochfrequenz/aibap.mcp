//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetVersionHistory_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	tests := []struct {
		name string
		uri  string
	}{
		{"report", testReportURI},
		{"class", testClassURI},
		{"interface", testInterfaceURI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versions, err := client.GetVersionHistory(ctx, tt.uri)
			if err != nil {
				// Some test fixtures may not have version history on all systems
				// (e.g. report never activated on S4). Skip rather than fail.
				t.Skipf("GetVersionHistory(%s): %v", tt.uri, err)
			}
			if len(versions) == 0 {
				t.Fatalf("expected at least one version for %s", tt.uri)
			}
			t.Logf("found %d version(s)", len(versions))
			for i, v := range versions {
				t.Logf("  [%d] version=%s author=%s transport=%s date=%s include=%s",
					i, v.VersionNumber, v.Author, v.Transport, v.Date, v.Include)
			}

			// Retrieve source of the most recent version
			latest := versions[0]
			if latest.ContentURI == "" {
				t.Fatal("expected content_uri in version entry")
			}
			src, err := client.GetVersionSource(ctx, latest.ContentURI)
			if err != nil {
				t.Fatalf("GetVersionSource: %v", err)
			}
			if src == "" {
				t.Fatal("expected non-empty historical source")
			}
			t.Logf("version source: %d bytes", len(src))
		})
	}
}

func TestGetVersionHistory_ClassHasIncludes_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	versions, err := client.GetVersionHistory(ctx, testClassURI)
	if err != nil {
		t.Fatalf("GetVersionHistory: %v", err)
	}

	hasDefinitions := false
	hasImplementations := false
	for _, v := range versions {
		switch v.Include {
		case "definitions":
			hasDefinitions = true
		case "implementations":
			hasImplementations = true
		}
	}
	if !hasDefinitions {
		t.Error("expected at least one version with include=definitions")
	}
	if !hasImplementations {
		t.Error("expected at least one version with include=implementations")
	}
}

func TestDiffActiveInactive_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	result, err := client.DiffActiveInactive(ctx, testReportURI)
	if err != nil {
		t.Fatalf("DiffActiveInactive: %v", err)
	}
	t.Logf("has_changes=%v active_len=%d inactive_len=%d", result.HasChanges, len(result.Active), len(result.Inactive))
	if result.Active == "" {
		t.Error("expected non-empty active source")
	}
}
