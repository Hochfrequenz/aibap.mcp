//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestGetVersionHistory_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	versions, err := client.GetVersionHistory(ctx, testReportURI)
	if err != nil {
		t.Fatalf("GetVersionHistory: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one version for test report")
	}
	t.Logf("found %d version(s)", len(versions))
	for i, v := range versions {
		t.Logf("  [%d] version=%s author=%s transport=%s date=%s", i, v.VersionNumber, v.Author, v.Transport, v.Date)
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
