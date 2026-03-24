//go:build integration

package adt_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

const editCycleReportName = "Z_MCP_EDITCYCLE_TEST"
const editCycleReportURI = "/sap/bc/adt/programs/programs/" + editCycleReportName

// setupEditCycleReport creates a disposable $TMP program and registers cleanup.
func setupEditCycleReport(t *testing.T, client adt.Client) string {
	t.Helper()
	ctx := context.Background()

	err := client.CreateObject(ctx, "PROG", editCycleReportName, "$TMP",
		fmt.Sprintf("Edit cycle integration test (%s)", time.Now().Format("2006-01-02")), "")
	if err != nil {
		if _, infoErr := client.GetObjectInfo(ctx, editCycleReportURI); infoErr != nil {
			t.Fatalf("CreateObject %s failed and object does not exist: %v", editCycleReportName, err)
		}
		t.Logf("object %s already exists, reusing", editCycleReportName)
	}

	// Set initial source so the object is in a known state.
	lockHandle, err := client.LockObject(ctx, editCycleReportURI)
	if err != nil {
		t.Fatalf("LockObject for setup failed: %v", err)
	}
	src, err := client.GetSource(ctx, editCycleReportURI)
	if err != nil {
		_ = client.UnlockObject(ctx, editCycleReportURI, lockHandle)
		t.Fatalf("GetSource for setup failed: %v", err)
	}
	initialSource := "REPORT " + strings.ToLower(editCycleReportName) + ".\nWRITE: / 'initial'.\n"
	_, err = client.SetSource(ctx, editCycleReportURI, initialSource, lockHandle, "", src.ETag)
	if err != nil {
		_ = client.UnlockObject(ctx, editCycleReportURI, lockHandle)
		t.Fatalf("SetSource for setup failed: %v", err)
	}
	_ = client.UnlockObject(ctx, editCycleReportURI, lockHandle)

	// Activate the initial source.
	result, err := client.ActivateObjects(ctx, []string{editCycleReportURI})
	if err != nil {
		t.Fatalf("ActivateObjects for setup failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("activation of initial source failed: %d messages", len(result.Messages))
	}

	t.Cleanup(func() {
		if err := client.DeleteObject(context.Background(), editCycleReportURI, "", ""); err != nil {
			t.Logf("WARNING: cleanup failed to delete %s: %v", editCycleReportName, err)
		}
	})

	return editCycleReportURI
}

// TestFullEditCycle_Integration exercises the complete safe edit workflow:
// lock → get source → write → unlock → activate → verify → restore.
// Uses a $TMP object so no transport is needed.
func TestFullEditCycle_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	objectURI := setupEditCycleReport(t, client)
	ctx := context.Background()

	// Step 1: Lock the object.
	lockHandle, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("step 1 (lock): %v", err)
	}
	t.Logf("step 1: locked, handle=%s", lockHandle)

	// Register unlock cleanup in case something fails mid-test.
	unlocked := false
	t.Cleanup(func() {
		if !unlocked {
			_ = client.UnlockObject(context.Background(), objectURI, lockHandle)
		}
	})

	// Step 2: Get current source and ETag.
	original, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("step 2 (get source): %v", err)
	}
	t.Logf("step 2: got source (%d bytes), ETag=%s", len(original.Source), original.ETag)

	// Step 3: Write modified source.
	// In ABAP, " starts an inline comment.
	modifiedSource := "REPORT " + strings.ToLower(editCycleReportName) + ".\nWRITE: / 'modified by edit cycle test'.\n"
	newETag, err := client.SetSource(ctx, objectURI, modifiedSource, lockHandle, "", original.ETag)
	if err != nil {
		t.Fatalf("step 3 (write source): %v", err)
	}
	t.Logf("step 3: wrote source, new ETag=%s", newETag)

	// Step 4: Unlock before activation.
	err = client.UnlockObject(ctx, objectURI, lockHandle)
	if err != nil {
		t.Fatalf("step 4 (unlock): %v", err)
	}
	unlocked = true
	t.Logf("step 4: unlocked")

	// TODO(#18): For objects in transportable packages, check transport
	// requirements here via client.CheckTransport(). $TMP objects skip this.
	// See: https://github.com/Hochfrequenz/mcp-server-abap/issues/18

	// TODO(#19): If transport is required, create one via client.CreateTransport()
	// or use an existing one, then add the object via client.AddToTransport().
	// See: https://github.com/Hochfrequenz/mcp-server-abap/issues/19
	// See: https://github.com/Hochfrequenz/mcp-server-abap/issues/23

	// Step 5: Activate the modified object.
	result, err := client.ActivateObjects(ctx, []string{objectURI})
	if err != nil {
		t.Fatalf("step 5 (activate): %v", err)
	}
	if !result.Success {
		for _, m := range result.Messages {
			t.Logf("  activation message [%s]: %s", m.Type, m.Text)
		}
		t.Fatalf("step 5: activation failed with %d messages", len(result.Messages))
	}
	t.Logf("step 5: activated successfully")

	// Step 6: Verify the source was actually updated.
	updated, err := client.GetSource(ctx, objectURI)
	if err != nil {
		t.Fatalf("step 6 (verify): %v", err)
	}
	if !strings.Contains(updated.Source, "modified by edit cycle test") {
		t.Fatalf("step 6: source does not contain expected text.\ngot: %s", updated.Source)
	}
	t.Logf("step 6: verified source contains modified text")

	// Step 7: Restore original source (lock → write → unlock → activate).
	restoreLock, err := client.LockObject(ctx, objectURI)
	if err != nil {
		t.Fatalf("step 7 (restore lock): %v", err)
	}
	restoreSrc, err := client.GetSource(ctx, objectURI)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, restoreLock)
		t.Fatalf("step 7 (restore get source): %v", err)
	}
	_, err = client.SetSource(ctx, objectURI, original.Source, restoreLock, "", restoreSrc.ETag)
	if err != nil {
		_ = client.UnlockObject(ctx, objectURI, restoreLock)
		t.Fatalf("step 7 (restore write): %v", err)
	}
	_ = client.UnlockObject(ctx, objectURI, restoreLock)

	restoreResult, err := client.ActivateObjects(ctx, []string{objectURI})
	if err != nil {
		t.Fatalf("step 7 (restore activate): %v", err)
	}
	if !restoreResult.Success {
		t.Logf("step 7: WARNING: restore activation had issues")
	}
	t.Logf("step 7: restored original source and activated")
}
