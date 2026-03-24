package tools_test

import (
	"fmt"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
)

type mockSelector struct {
	selectFn     func(name string) (string, error)
	activeNameFn func() string
}

func (m *mockSelector) Select(name string) (string, error) {
	if m.selectFn != nil {
		return m.selectFn(name)
	}
	return "Active system: " + name + " (http://example.com)", nil
}

func (m *mockSelector) ActiveName() string {
	if m.activeNameFn != nil {
		return m.activeNameFn()
	}
	return "dev"
}

func TestSelectSystemSuccess(t *testing.T) {
	selector := &mockSelector{
		selectFn: func(name string) (string, error) {
			return "Active system: " + name + " (https://prod:8000)", nil
		},
	}
	s := newTestServerWithSelector(&mockClient{}, selector, adt.NewLockMap())
	result := callTool(t, s, "select_system", map[string]any{"system": "prod"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result)
	}
}

func TestSelectSystemUnknown(t *testing.T) {
	selector := &mockSelector{
		selectFn: func(name string) (string, error) {
			return "", fmt.Errorf("unknown system %q", name)
		},
	}
	s := newTestServerWithSelector(&mockClient{}, selector, adt.NewLockMap())
	result := callTool(t, s, "select_system", map[string]any{"system": "nonexistent"})
	if !result.IsError {
		t.Error("expected error result for unknown system")
	}
}
