package tools_test

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// TestGetMessageClass_MissingName verifies that an omitted "message_class" is
// rejected locally with a clear message instead of forwarding an empty
// string to adtler. See #386.
func TestGetMessageClass_MissingName(t *testing.T) {
	called := false
	mock := &mockClient{
		getMessageClassFn: func(_ context.Context, _ string) (*adt.MessageClassInfo, error) {
			called = true
			return &adt.MessageClassInfo{}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_message_class", map[string]interface{}{})
	if !result.IsError {
		t.Fatal("expected a missing message_class to be rejected")
	}
	if called {
		t.Error("the call should not have reached adtler with message_class missing")
	}
}

func TestGetMessageClass_ReadsWithoutAskingTheClient(t *testing.T) {
	called := false
	var gotName string
	mock := &mockClient{
		getMessageClassFn: func(_ context.Context, name string) (*adt.MessageClassInfo, error) {
			called = true
			gotName = name
			return &adt.MessageClassInfo{Name: name}, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "get_message_class", map[string]interface{}{
		"message_class": "ZFOO",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if !called {
		t.Fatal("expected getMessageClassFn to be called")
	}
	if gotName != "ZFOO" {
		t.Errorf("message_class reached the client as %q, want ZFOO", gotName)
	}
}

// TestSearchMessages_MissingQuery verifies that an omitted "query" is
// rejected locally with a clear message instead of forwarding an empty
// string to adtler. See #386.
func TestSearchMessages_MissingQuery(t *testing.T) {
	called := false
	mock := &mockClient{
		searchMessagesFn: func(_ context.Context, _ string, _ int) ([]adt.MessageSearchResult, error) {
			called = true
			return nil, nil
		},
	}
	s := newTestServer(mock)
	result := callTool(t, s, "search_messages", map[string]interface{}{})
	if !result.IsError {
		t.Fatal("expected a missing query to be rejected")
	}
	if called {
		t.Error("the call should not have reached adtler with query missing")
	}
}

// TestSetMessages_MissingRequiredParams verifies that an omitted required
// parameter is rejected locally with a clear message instead of forwarding an
// empty string to adtler. See #386.
func TestSetMessages_MissingRequiredParams(t *testing.T) {
	fullArgs := map[string]interface{}{
		"message_class": "ZFOO",
		"messages":      `[{"number":"001","text":"Hello"}]`,
	}
	for _, missing := range []string{"message_class", "messages"} {
		t.Run(missing, func(t *testing.T) {
			called := false
			mock := &mockClient{
				setMessagesFn: func(_ context.Context, _, _ string, _ []adt.Message) error {
					called = true
					return nil
				},
			}
			args := map[string]interface{}{}
			for k, v := range fullArgs {
				if k != missing {
					args[k] = v
				}
			}
			s := newTestServer(mock)
			result := callTool(t, s, "set_messages", args)
			if !result.IsError {
				t.Fatalf("expected a missing %q to be rejected", missing)
			}
			if called {
				t.Errorf("the call should not have reached adtler with %q missing", missing)
			}
		})
	}
}
