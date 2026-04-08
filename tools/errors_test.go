package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMatchHint_ADTError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string // substring that must appear in the hint, "" = no hint
	}{
		{"423 lock", &adt.ADTError{StatusCode: 423, Message: "User SMITH is editing Z_REPORT"}, "unlock_object"},
		{"404 not found", &adt.ADTError{StatusCode: 404, Message: "Object not found"}, "search_objects"},
		{"403 forbidden", &adt.ADTError{StatusCode: 403, Message: "Forbidden"}, "S_DEVELOP"},
		{"400 transport", &adt.ADTError{StatusCode: 400, Message: "transport required for package ZDEV"}, "create_transport"},
		{"400 other", &adt.ADTError{StatusCode: 400, Message: "invalid parameter"}, ""},
		{"409 conflict", &adt.ADTError{StatusCode: 409, Message: "resource already exists"}, "already exists"},
		{"500 server", &adt.ADTError{StatusCode: 500, Message: "internal error"}, "SM21"},
		{"200 no hint", &adt.ADTError{StatusCode: 200, Message: "ok"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			if tt.wantHint == "" {
				if hint != "" {
					t.Errorf("expected no hint, got: %s", hint)
				}
			} else {
				if !strings.Contains(hint, tt.wantHint) {
					t.Errorf("hint should contain %q, got: %s", tt.wantHint, hint)
				}
			}
		})
	}
}

func TestMatchHint_PlainError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		{"already exists", fmt.Errorf("object ZTABLE already exists"), "already exists"},
		{"inactive", fmt.Errorf("activation failed: object is inactive"), "activate_objects"},
		{"random error", fmt.Errorf("something went wrong"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			if tt.wantHint == "" {
				if hint != "" {
					t.Errorf("expected no hint, got: %s", hint)
				}
			} else {
				if !strings.Contains(hint, tt.wantHint) {
					t.Errorf("hint should contain %q, got: %s", tt.wantHint, hint)
				}
			}
		})
	}
}

func TestErrorResult_WithHint(t *testing.T) {
	err := &adt.ADTError{StatusCode: 423, Message: "User SMITH is editing Z_REPORT"}
	result := errorResult(err)
	if !result.IsError {
		t.Fatal("expected IsError")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "SAP ADT error 423") {
		t.Errorf("should contain original error, got: %s", text)
	}
	if !strings.Contains(text, "Hint:") {
		t.Errorf("should contain hint, got: %s", text)
	}
}

func TestErrorResult_WithoutHint(t *testing.T) {
	err := fmt.Errorf("some unknown error")
	result := errorResult(err)
	text := result.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "Hint:") {
		t.Errorf("should not contain hint for unknown error, got: %s", text)
	}
	if !strings.Contains(text, "some unknown error") {
		t.Errorf("should contain original error, got: %s", text)
	}
}
