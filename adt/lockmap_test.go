package adt_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

// mockLocker implements adt.LockClient for testing.
type mockLocker struct {
	handle string
	err    error
	called bool
}

func (m *mockLocker) LockObject(_ context.Context, _ string) (string, error) {
	m.called = true
	return m.handle, m.err
}

func (m *mockLocker) UnlockObject(_ context.Context, _, _ string) error {
	return nil
}

// mockReader implements adt.SourceClient for testing.
type mockReader struct {
	result *adt.SourceResult
	err    error
	called bool
}

func (m *mockReader) GetSource(_ context.Context, _ string) (*adt.SourceResult, error) {
	m.called = true
	return m.result, m.err
}

func (m *mockReader) GetClassDefinition(_ context.Context, _ string) (*adt.SourceResult, error) {
	return &adt.SourceResult{}, nil
}
func (m *mockReader) SetSource(_ context.Context, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockReader) GetIncludeSource(_ context.Context, _, _ string) (*adt.SourceResult, error) {
	return &adt.SourceResult{}, nil
}
func (m *mockReader) SetIncludeSource(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockReader) CreateTestInclude(_ context.Context, _, _, _ string) error {
	return nil
}

func (m *mockReader) PrettyPrint(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockReader) GetCompletions(_ context.Context, _, _ string, _, _ int) ([]adt.CompletionItem, error) {
	return nil, nil
}

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

func TestLockKey(t *testing.T) {
	got := adt.LockKey("DEV", "/sap/bc/adt/programs/programs/ZTEST")
	want := "DEV:/sap/bc/adt/programs/programs/ZTEST"
	if got != want {
		t.Errorf("LockKey = %q, want %q", got, want)
	}
}

func TestResolveLock_ExplicitHandle(t *testing.T) {
	m := adt.NewLockMap()
	locker := &mockLocker{}
	handle, err := m.ResolveLock(context.Background(), locker, "key", "/uri", "explicit-handle")
	if err != nil || handle != "explicit-handle" || locker.called {
		t.Errorf("expected explicit handle returned without calling locker")
	}
}

func TestResolveLock_Cached(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("key", "cached-handle", "")
	locker := &mockLocker{}
	handle, err := m.ResolveLock(context.Background(), locker, "key", "/uri", "")
	if err != nil || handle != "cached-handle" || locker.called {
		t.Errorf("expected cached handle without calling locker")
	}
}

func TestResolveLock_AutoLock(t *testing.T) {
	m := adt.NewLockMap()
	locker := &mockLocker{handle: "new-handle"}
	handle, err := m.ResolveLock(context.Background(), locker, "key", "/uri", "")
	if err != nil || handle != "new-handle" || !locker.called {
		t.Errorf("expected auto-lock to be called")
	}
	// Verify it was stored
	state, ok := m.Get("key")
	if !ok || state.LockHandle != "new-handle" {
		t.Errorf("expected auto-lock handle to be stored in map")
	}
}

func TestResolveLock_AutoLockError(t *testing.T) {
	m := adt.NewLockMap()
	locker := &mockLocker{err: fmt.Errorf("lock failed")}
	_, err := m.ResolveLock(context.Background(), locker, "key", "/uri", "")
	if err == nil {
		t.Error("expected error from auto-lock")
	}
}

func TestResolveETag_Cached(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("key", "handle", "cached-etag")
	reader := &mockReader{}
	etag, err := m.ResolveETag(context.Background(), reader, "key", "/uri")
	if err != nil || etag != "cached-etag" || reader.called {
		t.Errorf("expected cached ETag without calling reader")
	}
}

func TestResolveETag_FetchFromClient(t *testing.T) {
	m := adt.NewLockMap()
	m.Set("key", "handle", "") // no ETag cached
	reader := &mockReader{result: &adt.SourceResult{ETag: "fetched-etag"}}
	etag, err := m.ResolveETag(context.Background(), reader, "key", "/uri")
	if err != nil || etag != "fetched-etag" || !reader.called {
		t.Errorf("expected ETag fetched from client")
	}
	// Verify it was cached
	state, _ := m.Get("key")
	if state.ETag != "fetched-etag" {
		t.Errorf("expected fetched ETag to be cached")
	}
}

func TestResolveETag_ClientError(t *testing.T) {
	m := adt.NewLockMap()
	reader := &mockReader{err: fmt.Errorf("fetch failed")}
	_, err := m.ResolveETag(context.Background(), reader, "key", "/uri")
	if err == nil {
		t.Error("expected error from client")
	}
}
