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
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action:  mcp.ElicitationResponseActionAccept,
			Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}
	if gotClass != "ZCL_MY_RUNNER" {
		t.Errorf("RunClass called with %q, want ZCL_MY_RUNNER", gotClass)
	}
	if el.called != 1 {
		t.Errorf("elicitor called %d times, want 1", el.called)
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
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_NOPE"})

	if !res.IsError {
		t.Fatal("expected error for missing class")
	}
	if runCalled {
		t.Error("RunClass must not be called when the class is missing")
	}
	if el.called != 0 {
		t.Error("elicitor must not be prompted when the class is missing")
	}
}

func TestRunClass_ConfirmationDeclined(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{}, nil
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if !res.IsError {
		t.Fatal("expected error when confirmation declined")
	}
	if runCalled {
		t.Error("RunClass must not be called when confirmation is declined")
	}
	if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "run_class aborted") {
		t.Errorf("expected 'run_class aborted' in error, got: %v", res.Content)
	}
}

func TestRunClass_NilElicitorProceeds(t *testing.T) {
	runCalled := false
	mock := &mockClient{
		runClassFn: func(_ context.Context, className string) (*adt.ClassRunResult, error) {
			runCalled = true
			return &adt.ClassRunResult{ClassName: className, ConsoleOutput: "ran"}, nil
		},
	}
	s := newTestServerWithFallbackElicitor(mock, nil, nil) // nil elicitor

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if res.IsError {
		t.Fatalf("nil elicitor should proceed, got error: %v", res.Content)
	}
	if !runCalled {
		t.Error("RunClass should be called when elicitor is nil (backwards-compat)")
	}
}

func TestRunClass_RunClassError(t *testing.T) {
	mock := &mockClient{
		runClassFn: func(context.Context, string) (*adt.ClassRunResult, error) {
			return nil, &adt.ADTError{StatusCode: 500, Message: "boom"}
		},
	}
	el := &stubElicitor{result: &mcp.ElicitationResult{
		ElicitationResponse: mcp.ElicitationResponse{
			Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirm": true},
		},
	}}
	s := newTestServerWithFallbackElicitor(mock, nil, el)

	res := callTool(t, s, "run_class", map[string]any{"class_name": "ZCL_MY_RUNNER"})

	if !res.IsError {
		t.Fatal("expected error when RunClass fails")
	}
}
