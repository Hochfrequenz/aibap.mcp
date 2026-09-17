package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunClass_HappyPath(t *testing.T) {
	var gotClass string
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			gotClass = className
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "hello from abap"}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}
	if gotClass != "ZCL_MY_RUNNER" {
		t.Errorf("RunClass called with %q, want ZCL_MY_RUNNER", gotClass)
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "hello from abap") {
		t.Errorf("console output missing from result: %v", res.Content)
	}
}

func TestRunClass_ClassMissing(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		getObjectFn: func(context.Context, string) (*adt.ObjectInfo, error) {
			return nil, &adt.ADTError{StatusCode: 404, Message: "not found"}
		},
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_NOPE"})

	if !res.IsError {
		t.Fatal("expected error for missing class")
	}
	if runCalled {
		t.Error("RunClass must not be called when the class is missing")
	}
}

func TestRunClass_RunsWithoutAskingTheClient(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "ran"}, nil
		},
	}
	s := newTestServerWithFallback(mock, nil)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("run_class should run without asking, got error: %v", res.Content)
	}
	if !runCalled {
		t.Error("RunClass should be called: this server no longer gates the call itself")
	}
}

func TestRunClass_RunClassError(t *testing.T) {
	mock := &mockClient{
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "boom"}
		},
	}
	s := newTestServerWithFallback(mock, nil)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if !res.IsError {
		t.Fatal("expected error when RunClass fails")
	}
}
