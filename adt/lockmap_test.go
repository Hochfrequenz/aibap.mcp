package adt_test

import (
	"fmt"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

func TestLockMapSetAndGet(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")

	state, ok := m.Get("sys:uri1")
	if !ok {
		t.Fatal("expected entry")
	}
	if state.LockHandle != "handle1" || state.ETag != "etag1" {
		t.Errorf("got %+v", state)
	}
}

func TestLockMapGetMissing(t *testing.T) {
	m := adt.NewLockMap()
	_, ok := m.Get("sys:uri1")
	if ok {
		t.Fatal("expected no entry")
	}
}

func TestLockMapUpdateETag(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")
	m.UpdateETag("sys:uri1", "etag2")

	state, _ := m.Get("sys:uri1")
	if state.ETag != "etag2" {
		t.Errorf("etag: got %q", state.ETag)
	}
}

func TestLockMapUpdateETagMissing(t *testing.T) {
	m := adt.NewLockMap()
	m.UpdateETag("sys:missing", "etag2") // should not panic
}

func TestLockMapDelete(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("sys:uri1", "handle1", "etag1")
	m.Delete("sys:uri1")

	_, ok := m.Get("sys:uri1")
	if ok {
		t.Fatal("expected deleted")
	}
}

func TestLockMapConcurrent(t *testing.T) {
	m := adt.NewLockMap()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			key := fmt.Sprintf("sys:uri%d", n)
			m.Set(key, "handle", "etag")
			m.Get(key)
			m.UpdateETag(key, "etag2")
			m.Delete(key)
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
