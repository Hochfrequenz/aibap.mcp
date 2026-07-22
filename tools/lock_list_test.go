package tools

import (
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestCollectTrackedLocks(t *testing.T) {
	lockMap := adt.NewLockMap()
	tracker := newSessionLockTracker()

	kA := adt.LockKey("s4u", "/sap/bc/adt/oo/classes/zcl_a")
	kB := adt.LockKey("s4u", "/sap/bc/adt/programs/programs/z_b")
	kC := adt.LockKey("hfq", "/sap/bc/adt/oo/interfaces/zif_c")
	lockMap.Set(kA, "H_A", "E_A")
	lockMap.Set(kB, "H_B", "")
	lockMap.Set(kC, "H_C", "E_C")
	tracker.track(kA)
	tracker.track(kB)
	tracker.track(kC)

	// Tracked but not held in the lock map (explicit-handle write bypasses the
	// map). An honest ledger must exclude it — force_unlock uses the same rule.
	tracker.track(adt.LockKey("s4u", "/sap/bc/adt/oo/classes/zcl_ghost"))

	got := collectTrackedLocks(tracker, lockMap)

	if got.Count != 3 {
		t.Fatalf("Count = %d, want 3", got.Count)
	}
	if len(got.Locks) != 3 {
		t.Fatalf("len(Locks) = %d, want 3", len(got.Locks))
	}

	// Deterministic, sorted by system-qualified key: hfq < s4u; within s4u,
	// /oo/... < /programs/...
	want := []LockInfo{
		{System: "hfq", URI: "/sap/bc/adt/oo/interfaces/zif_c", LockHandle: "H_C", ETag: "E_C"},
		{System: "s4u", URI: "/sap/bc/adt/oo/classes/zcl_a", LockHandle: "H_A", ETag: "E_A"},
		{System: "s4u", URI: "/sap/bc/adt/programs/programs/z_b", LockHandle: "H_B", ETag: ""},
	}
	for i, w := range want {
		if got.Locks[i] != w {
			t.Errorf("Locks[%d] = %+v, want %+v", i, got.Locks[i], w)
		}
	}
}

func TestCollectTrackedLocks_Empty(t *testing.T) {
	got := collectTrackedLocks(newSessionLockTracker(), adt.NewLockMap())
	if got.Count != 0 {
		t.Fatalf("Count = %d, want 0", got.Count)
	}
	// Must be a non-nil empty slice so structuredContent serialises as [] not null.
	if got.Locks == nil {
		t.Error("Locks is nil, want non-nil empty slice")
	}
}

func TestCollectTrackedLocks_NilTracker(t *testing.T) {
	got := collectTrackedLocks(nil, adt.NewLockMap())
	if got.Count != 0 || got.Locks == nil {
		t.Fatalf("nil tracker: got %+v, want empty non-nil", got)
	}
}
