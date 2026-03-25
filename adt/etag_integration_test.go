//go:build integration

package adt_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

const etagTestReportName = "Z_MCP_ETAG_TEST"

// TestSetSource_StaleETag_ReturnsError verifies that SAP rejects a write when
// the ETag is stale (object modified since the ETag was obtained). This proves
// optimistic concurrency control works end-to-end.
//
// Scenario:
//  1. Lock the test object
//  2. Get source (obtain current ETag)
//  3. Write source with a trivial change (updates the ETag on the server)
//  4. Attempt a second write using the original (now stale) ETag
//  5. Assert the second write fails
//  6. Restore original source and unlock
func TestSetSource_StaleETag_ReturnsError(t *testing.T) {
	client := newIntegrationClient(t)
	initialSource := "REPORT " + strings.ToLower(etagTestReportName) + ".\nWRITE: / 'etag test'.\n"
	objectURI := setupDisposableReport(t, client, etagTestReportName, initialSource)
	ctx := context.Background()

	// Step 1: Lock the test object.
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.UnlockObject(context.Background(), objectURI, lockHandle)
	})

	// Step 2: Get current source and ETag.
	original, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if original.ETag == "" {
		t.Fatal("GetSource returned empty ETag")
	}
	staleETag := original.ETag
	t.Logf("original ETag: %s", staleETag)

	// Step 3: Write a trivial change — this advances the ETag on the server.
	// SAP ETags are timestamp-based with 1-second resolution (observed on NW 7.5x),
	// so we wait >1s to ensure the server timestamp advances between read and write.
	time.Sleep(1100 * time.Millisecond)
	// In ABAP, " starts an inline comment, so this appends a harmless comment line.
	modifiedSource := original.Source + "\n\" stale etag test marker\n"
	_, err = client.SetSource(ctx, objectURI, modifiedSource, lockHandle, "", staleETag)
	if err != nil {
		t.Fatalf("first SetSource failed: %v", err)
	}

	// Re-read to confirm the ETag actually changed.
	afterWrite, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource after write failed: %v", err)
	}
	t.Logf("ETag after write: %s (was: %s)", afterWrite.ETag, staleETag)

	if afterWrite.ETag == staleETag {
		t.Skip("server returned same ETag after write — cannot test stale ETag rejection")
	}

	// Ensure we restore original source regardless of test outcome.
	t.Cleanup(func() {
		src, err := client.GetSource(context.Background(), objectURI)
		if err != nil {
			t.Logf("WARNING: could not read source for restoration: %v", err)
			return
		}
		_, err = client.SetSource(context.Background(), objectURI, original.Source, lockHandle, "", src.ETag)
		if err != nil {
			t.Logf("WARNING: could not restore original source: %v", err)
		}
	})

	// Step 4: Attempt a second write using the STALE ETag — should fail.
	_, err = client.SetSource(ctx, objectURI, original.Source, lockHandle, "", staleETag)
	if err == nil {
		t.Fatal("expected error when writing with stale ETag, but SetSource succeeded")
	}
	t.Logf("stale ETag correctly rejected: %v", err)
}

// TestSetSource_FreshETag_Succeeds verifies that writing with the correct
// (fresh) ETag succeeds. This is the positive counterpart to the stale ETag test.
func TestSetSource_FreshETag_Succeeds(t *testing.T) {
	client := newIntegrationClient(t)
	initialSource := "REPORT " + strings.ToLower(etagTestReportName) + ".\nWRITE: / 'etag test'.\n"
	objectURI := setupDisposableReport(t, client, etagTestReportName, initialSource)
	ctx := context.Background()

	// Lock the object.
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.UnlockObject(context.Background(), objectURI, lockHandle)
	})

	// Get current source and ETag.
	original, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if original.ETag == "" {
		t.Fatal("GetSource returned empty ETag")
	}

	// Write with the fresh ETag — should succeed.
	// In ABAP, " starts an inline comment.
	modifiedSource := original.Source + "\n\" fresh etag test marker\n"
	newETag, err := client.SetSource(ctx, objectURI, modifiedSource, lockHandle, "", original.ETag)
	if err != nil {
		t.Fatalf("SetSource with fresh ETag failed: %v", err)
	}
	if newETag == "" {
		t.Fatal("SetSource returned empty ETag after successful write")
	}
	t.Logf("write succeeded, new ETag: %s", newETag)

	// Restore original source.
	_, err = client.SetSource(ctx, objectURI, original.Source, lockHandle, "", newETag)
	if err != nil {
		t.Fatalf("restoration SetSource failed: %v", err)
	}
}
