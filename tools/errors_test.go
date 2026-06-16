package tools

import (
	"errors"
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
		{"400 catch-all (no type, no transport)", &adt.ADTError{StatusCode: 400, Message: "invalid parameter"}, "Bad request"},
		{"409 lock conflict fallback (no type)", &adt.ADTError{StatusCode: 409, Message: "resource already exists"}, "Save conflict"},
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

// TestMatchHint_ByExceptionType pins the Tier-1 matching on the
// language- and system-independent adt.ADTError.Type identifier. All
// Type IDs and their status codes were read from the live ABAP source
// (GET_HTTP_STATUS) on both S4U and HFQ — see issue #406.
func TestMatchHint_ByExceptionType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		// 423 matched by Type (adtler-exported constant), not just status.
		{"resource locked", &adt.ADTError{StatusCode: 423, Type: "ExceptionResourceLocked", Message: "Ressource ist gesperrt"}, "unlock_object"},

		// 409 on S4U is a save/lock conflict, NOT "already exists".
		{"lock conflict", &adt.ADTError{StatusCode: 409, Type: "ExceptionResourceLockConflict", Message: "Ressource konnte nicht gesichert werden"}, "Save conflict"},

		// "Already exists" returns 400 on S4U and 405 on HFQ — the Type
		// rule must produce the same hint regardless of status code, and
		// regardless of the message language.
		{"already exists S4U (400, English)", &adt.ADTError{StatusCode: 400, Type: "ExceptionResourceAlreadyExists", Message: "Resource CLASS ZFOO already exists"}, "already exists"},
		{"already exists HFQ (405, German)", &adt.ADTError{StatusCode: 405, Type: "ExceptionResourceAlreadyExists", Message: "Ressource CLASS ZFOO existiert bereits"}, "already exists"},

		// ETag/precondition: two distinct classes, one hint.
		{"invalid etag", &adt.ADTError{StatusCode: 412, Type: "ExceptionResourceInvalidEtag", Message: "eTag differs"}, "ETag mismatch"},
		{"precondition failed", &adt.ADTError{StatusCode: 412, Type: "ExceptionPreconditionFailed", Message: "Vorbedingung fehlgeschlagen"}, "ETag mismatch"},

		// Content negotiation.
		{"not acceptable", &adt.ADTError{StatusCode: 406, Type: "ExceptionResourceNotAcceptable", Message: "not acceptable"}, "negotiation"},
		{"unsupported media type", &adt.ADTError{StatusCode: 415, Type: "ExceptionUnsupportedMediaType", Message: "unsupported"}, "Content-Type"},

		// Semantic.
		{"unprocessable entity", &adt.ADTError{StatusCode: 422, Type: "ExceptionUnprocessableEntity", Message: "semantic errors"}, "semantic"},

		// Genuine method-not-allowed (S4U only) — distinct Type from
		// "already exists", so it must NOT produce the already-exists hint.
		{"method not allowed", &adt.ADTError{StatusCode: 405, Type: "ExceptionNotAllowed", Message: "not allowed"}, "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			if !strings.Contains(hint, tt.wantHint) {
				t.Errorf("hint should contain %q, got: %s", tt.wantHint, hint)
			}
		})
	}
}

// TestMatchHint_StatusCodeFallback pins Tier-2 matching: when Type is
// empty (legacy <ExceptionText> envelopes, HTML error pages, plain
// bodies) the matcher falls back to the status code, which is
// language-independent.
func TestMatchHint_StatusCodeFallback(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		{"412 no type", &adt.ADTError{StatusCode: 412, Message: "precondition failed"}, "ETag mismatch"},
		{"409 no type", &adt.ADTError{StatusCode: 409, Message: "conflict"}, "Save conflict"},
		{"405 no type (ambiguous fallback)", &adt.ADTError{StatusCode: 405, Message: "method not allowed"}, "Method not allowed"},
		{"400 transport beats catch-all", &adt.ADTError{StatusCode: 400, Message: "transport required"}, "create_transport"},
		{"400 catch-all", &adt.ADTError{StatusCode: 400, Message: "malformed"}, "Bad request"},
		// A Type adtler knows but hintRules does not list must fall
		// through Tier 1 into the Tier-2 status rule.
		{"unlisted type falls through to status", &adt.ADTError{StatusCode: 400, Type: "ExceptionResourceWrongData", Message: "bad data"}, "Bad request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			if !strings.Contains(hint, tt.wantHint) {
				t.Errorf("hint should contain %q, got: %s", tt.wantHint, hint)
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
		// Real ReleaseTransport error text captured live from S4U (issue
		// #406): releasing a request with an inactive object. It is a
		// plain wrapped error, not an adt.ADTError, so only the Tier-3
		// text rule can catch it.
		{"inactive object in transport release", fmt.Errorf("ReleaseTransport S4UK900001 failed: Release of transport request/task S4UK900001 has failed. See Problems view: Object REPS ZFOO is inactive"), "activate_objects"},
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

// TestErrorResult_PinsWireContract asserts the wire contract of
// errorResult. Since #354 the error path intentionally does NOT set
// StructuredContent — MCP 2025-06-18 /server/tools requires it to
// conform to each tool's declared outputSchema, and a typed error DTO
// would contradict every tool's schema. The SAP status code, when
// available, is preserved in the text fallback via adt.ADTError.Error().
// Hints are appended for known error patterns (see hintRules in errors.go).
// Update this test only if the change is intentional and documented in
// the PR that breaks the contract.
func TestErrorResult_PinsWireContract(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantIsError bool
		wantText    string
	}{
		{
			name:        "plain error",
			err:         errors.New("boom"),
			wantIsError: true,
			wantText:    "Error: boom",
		},
		{
			name:        "adt.ADTError — SAP status code surfaces in text via ADTError.Error(), hint appended",
			err:         &adt.ADTError{StatusCode: 404, Message: "not found"},
			wantIsError: true,
			wantText:    "Error: SAP ADT error 404: not found\n\nHint: Object not found. Check the URI spelling or use `search_objects` to find it.",
		},
		{
			name:        "wrapped ADTError preserves wrap context in text, hint appended",
			err:         fmt.Errorf("auto-lock failed: %w", &adt.ADTError{StatusCode: 423, Message: "resource locked"}),
			wantIsError: true,
			wantText:    "Error: auto-lock failed: SAP ADT error 423: resource locked\n\nHint: Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := errorResult(tc.err)

			if r.IsError != tc.wantIsError {
				t.Errorf("IsError = %v, want %v", r.IsError, tc.wantIsError)
			}

			if len(r.Content) != 1 {
				t.Fatalf("Content has %d entries, want 1", len(r.Content))
			}
			tc2, ok := r.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("Content[0] type = %T, want TextContent", r.Content[0])
			}
			if tc2.Text != tc.wantText {
				t.Errorf("text = %q, want %q", tc2.Text, tc.wantText)
			}

			// StructuredContent is intentionally absent on the error path
			// (see errorResult doc comment and issue #354). Guard against
			// a regression that re-introduces a typed error DTO.
			if r.StructuredContent != nil {
				t.Errorf("StructuredContent = %v, want nil (absent on error path, #354)", r.StructuredContent)
			}
		})
	}
}
