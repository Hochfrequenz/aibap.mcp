package adt_test

import (
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

const fourLineSource = "line1\nline2\nline3\nline4"

func TestApplyOpsInsert(t *testing.T) {
	source := "line1\nline2\nline3"
	ops := []adt.PatchOp{
		{Type: "insert", AfterLine: 1, Content: "inserted"},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\ninserted\nline2\nline3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsInsertAtZero(t *testing.T) {
	source := "line1\nline2"
	ops := []adt.PatchOp{
		{Type: "insert", AfterLine: 0, Content: "before"},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "before\nline1\nline2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsReplace(t *testing.T) {
	source := fourLineSource
	ops := []adt.PatchOp{
		{Type: "replace", FromLine: 2, ToLine: 3, Content: "new"},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nnew\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsDelete(t *testing.T) {
	source := fourLineSource
	ops := []adt.PatchOp{
		{Type: "delete", FromLine: 2, ToLine: 3},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsSearchReplace(t *testing.T) {
	source := "REPORT ZTEST.\nDATA: lv_x TYPE i."
	ops := []adt.PatchOp{
		{Type: "search_replace", Search: "ZTEST", Replace: "ZNEW"},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "REPORT ZNEW.\nDATA: lv_x TYPE i."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsSearchReplaceAll(t *testing.T) {
	source := "foo bar foo baz foo"
	ops := []adt.PatchOp{
		{Type: "search_replace", Search: "foo", Replace: "qux", All: true},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "qux bar qux baz qux"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsMultiple(t *testing.T) {
	// Two inserts: ops should be sorted descending by after_line so bottom-up application works.
	source := "line1\nline2\nline3"
	ops := []adt.PatchOp{
		{Type: "insert", AfterLine: 1, Content: "after1"},
		{Type: "insert", AfterLine: 2, Content: "after2"},
	}
	got, err := adt.ApplyPatchOps(source, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After sorting desc (after_line 2 first, then 1):
	// Apply after_line=2: line1, line2, after2, line3
	// Apply after_line=1: line1, after1, line2, after2, line3
	want := "line1\nafter1\nline2\nafter2\nline3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOpsOverlapRejected(t *testing.T) {
	source := fourLineSource
	ops := []adt.PatchOp{
		{Type: "replace", FromLine: 1, ToLine: 3, Content: "new1"},
		{Type: "replace", FromLine: 2, ToLine: 4, Content: "new2"},
	}
	_, err := adt.ApplyPatchOps(source, ops)
	if err == nil {
		t.Fatal("expected error for overlapping ops")
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("error should mention overlap, got: %v", err)
	}
}
