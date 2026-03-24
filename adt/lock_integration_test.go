//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestLockUnlock_Integration(t *testing.T) {
	// Known bug: LockObject sends Accept: application/xml but SAP requires
	// application/vnd.sap.as+xml → returns 406 Not Acceptable.
	// This test documents the bug. It will pass once the Accept header is fixed.
	client := newIntegrationClient(t)
	ctx := context.Background()

	lockHandle, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	if lockHandle == "" {
		t.Fatal("LockObject returned empty lock handle")
	}
	t.Logf("lock handle: %s", lockHandle)

	// Ensure unlock even if explicit unlock assertion below fails.
	t.Cleanup(func() {
		_ = client.UnlockObject(context.Background(), testReportURI, lockHandle)
	})

	err = client.UnlockObject(ctx, testReportURI, lockHandle)
	if err != nil {
		t.Fatalf("UnlockObject failed: %v", err)
	}
}

func TestLockObject_ReturnsFunctioningHandle(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	lockHandle, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Fatalf("LockObject failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.UnlockObject(context.Background(), testReportURI, lockHandle)
	})

	src, err := client.GetSource(ctx, testReportURI)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if src.ETag == "" {
		t.Fatal("GetSource returned empty ETag")
	}
	t.Logf("ETag: %s, source length: %d", src.ETag, len(src.Source))

	// Write the same source back — validates that the lock handle works.
	// Known bug: SetSource sends Content-Type: plain/abap but SAP requires
	// text/plain → returns 415 Unsupported Media Type.
	newETag, err := client.SetSource(ctx, testReportURI, src.Source, lockHandle, "", src.ETag)
	if err != nil {
		t.Fatalf("SetSource with lock handle failed: %v", err)
	}
	t.Logf("new ETag after write: %s", newETag)
}

func TestDoubleLock_ReturnsHandleOrError(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	lockHandle, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Fatalf("first LockObject failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.UnlockObject(context.Background(), testReportURI, lockHandle)
	})

	// Second lock on the same object — SAP may return the same handle
	// (re-entrant) or an error. Both are valid behaviors.
	handle2, err := client.LockObject(ctx, testReportURI)
	if err != nil {
		t.Logf("double lock returned error (expected on some systems): %v", err)
	} else {
		t.Logf("double lock returned handle (re-entrant): %s", handle2)
	}
}
