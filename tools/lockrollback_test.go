package tools_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

const (
	rollbackURI    = "/sap/bc/adt/programs/programs/ZROLLBACK"
	rollbackErrMsg = "simulated write failure"
	autoHandle     = "auto-h"
)

func writeTempSource(t *testing.T, body string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "src.abap")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return f
}

// #383: a write that auto-acquires a lock and then fails must roll the lock back
// (both server-side via UnlockObject and in the client ledger), instead of
// leaving the object locked (list_locks stuck at 1).
func TestSetSourceFromFile_RollsBackAutoLockOnWriteFailure(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		lockObjectFn: func(_ context.Context, _ string) (string, error) { return autoHandle, nil },
		setSourceFn: func(_ context.Context, _, _, _, _, _ string) (string, error) {
			return "", fmt.Errorf("SAP ADT error 400 (ExceptionParameterNotFound): corrNr")
		},
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	res := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": rollbackURI,
		"file_path":  writeTempSource(t, "REPORT zrollback.\n"),
	})
	if !res.IsError {
		t.Fatal("expected IsError=true (SetSource failed)")
	}
	if !unlocked {
		t.Error("auto-acquired lock was NOT rolled back on write failure (#383)")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); ok {
		t.Error("lock map still holds the auto-acquired lock after rollback")
	}
}

// A caller-supplied lock handle must NOT be released on write failure — the
// caller owns that lock's lifecycle.
func TestSetSourceFromFile_KeepsCallerLockOnFailure(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		setSourceFn:    func(_ context.Context, _, _, _, _, _ string) (string, error) { return "", errors.New(rollbackErrMsg) },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	s := newTestServerWithLockMap(mock, adt.NewLockMap())

	res := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri":  rollbackURI,
		"file_path":   writeTempSource(t, "REPORT z.\n"),
		"lock_handle": "caller-handle",
	})
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if unlocked {
		t.Error("must NOT release a caller-supplied lock on write failure")
	}
}

// A pre-existing tracked lock (from a prior lock_object) must also be preserved.
func TestSetSourceFromFile_KeepsPreExistingLockOnFailure(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		setSourceFn:    func(_ context.Context, _, _, _, _, _ string) (string, error) { return "", errors.New(rollbackErrMsg) },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	lockMap.Set(adt.LockKey("dev", rollbackURI), "pre-existing-handle", "")
	s := newTestServerWithLockMap(mock, lockMap)

	res := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": rollbackURI,
		"file_path":  writeTempSource(t, "REPORT z.\n"),
	})
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if unlocked {
		t.Error("must NOT release a pre-existing tracked lock on write failure")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); !ok {
		t.Error("pre-existing lock was wrongly removed from the map")
	}
}

func TestPatchSource_RollsBackAutoLockOnWriteFailure(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		lockObjectFn: func(_ context.Context, _ string) (string, error) { return autoHandle, nil },
		getSourceFn: func(_ context.Context, _ string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT z.\nLINE.\n", ETag: "e1"}, nil
		},
		setSourceFn:    func(_ context.Context, _, _, _, _, _ string) (string, error) { return "", errors.New(rollbackErrMsg) },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	res := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": rollbackURI,
		"operations": []interface{}{
			map[string]interface{}{"type": "insert", "after_line": float64(1), "content": "* injected"},
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true (SetSource failed)")
	}
	if !unlocked {
		t.Error("patch_source did not roll back the auto-acquired lock on write failure (#383)")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); ok {
		t.Error("lock map still holds the auto-acquired lock after rollback")
	}
}

// set_source_from_file: the rollback must also fire when the failure is at ETag
// resolution (GetSource error; the mock has no FetchETag fallback), before the
// write is even attempted.
func TestSetSourceFromFile_RollsBackWhenETagResolutionFails(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		lockObjectFn:   func(_ context.Context, _ string) (string, error) { return autoHandle, nil },
		getSourceFn:    func(_ context.Context, _ string) (*adt.SourceResult, error) { return nil, errors.New(rollbackErrMsg) },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	res := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": rollbackURI,
		"file_path":  writeTempSource(t, "REPORT zrollback.\n"),
	})
	if !res.IsError {
		t.Fatal("expected IsError=true (ETag resolution failed)")
	}
	if !unlocked {
		t.Error("auto-acquired lock was NOT rolled back when ETag resolution failed (#383)")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); ok {
		t.Error("lock map still holds the auto-acquired lock after rollback")
	}
}

// patch_source: the rollback must fire when GetSource (read-before-patch) fails
// after the auto-lock.
func TestPatchSource_RollsBackWhenGetSourceFails(t *testing.T) {
	var unlocked bool
	mock := &mockClient{
		lockObjectFn:   func(_ context.Context, _ string) (string, error) { return autoHandle, nil },
		getSourceFn:    func(_ context.Context, _ string) (*adt.SourceResult, error) { return nil, errors.New(rollbackErrMsg) },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	res := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": rollbackURI,
		"operations": []interface{}{
			map[string]interface{}{"type": "insert", "after_line": float64(0), "content": "* x"},
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true (GetSource failed)")
	}
	if !unlocked {
		t.Error("patch_source did not roll back the auto-acquired lock when GetSource failed (#383)")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); ok {
		t.Error("lock map still holds the auto-acquired lock after rollback")
	}
}

// patch_source: the rollback must fire when the patch itself fails to apply
// (invalid op), before any write is attempted.
func TestPatchSource_RollsBackWhenPatchApplyFails(t *testing.T) {
	var unlocked bool
	var wrote bool
	mock := &mockClient{
		lockObjectFn: func(_ context.Context, _ string) (string, error) { return autoHandle, nil },
		getSourceFn: func(_ context.Context, _ string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "REPORT zrollback.\n", ETag: "e1"}, nil
		},
		setSourceFn:    func(_ context.Context, _, _, _, _, _ string) (string, error) { wrote = true; return "", nil },
		unlockObjectFn: func(_ context.Context, _, _ string) error { unlocked = true; return nil },
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	// replace lines 99..99 on a 1-line source → ApplyPatchOps errors out of range.
	res := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": rollbackURI,
		"operations": []interface{}{
			map[string]interface{}{"type": "replace", "from_line": float64(99), "to_line": float64(99), "content": "* x"},
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true (patch apply failed)")
	}
	if wrote {
		t.Error("SetSource must not be called when the patch fails to apply")
	}
	if !unlocked {
		t.Error("patch_source did not roll back the auto-acquired lock when the patch failed to apply (#383)")
	}
	if _, ok := lockMap.Get(adt.LockKey("dev", rollbackURI)); ok {
		t.Error("lock map still holds the auto-acquired lock after rollback")
	}
}
