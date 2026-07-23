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
// (GET_HTTP_STATUS) on both S/4 and R/3 — see issue #406.
func TestMatchHint_ByExceptionType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		// 423 matched by Type (adtler-exported constant), not just status.
		{"resource locked", &adt.ADTError{StatusCode: 423, Type: "ExceptionResourceLocked", Message: "Ressource ist gesperrt"}, "unlock_object"},

		// 409 on S/4 is a save/lock conflict, NOT "already exists".
		{"lock conflict", &adt.ADTError{StatusCode: 409, Type: "ExceptionResourceLockConflict", Message: "Ressource konnte nicht gesichert werden"}, "Save conflict"},

		// "Already exists" returns 400 on S/4 and 405 on R/3 — the Type
		// rule must produce the same hint regardless of status code, and
		// regardless of the message language.
		{"already exists S/4 (400, English)", &adt.ADTError{StatusCode: 400, Type: "ExceptionResourceAlreadyExists", Message: "Resource CLASS ZFOO already exists"}, "already exists"},
		{"already exists R/3 (405, German)", &adt.ADTError{StatusCode: 405, Type: "ExceptionResourceAlreadyExists", Message: "Ressource CLASS ZFOO existiert bereits"}, "already exists"},

		// ETag/precondition: two distinct classes, one hint.
		{"invalid etag", &adt.ADTError{StatusCode: 412, Type: "ExceptionResourceInvalidEtag", Message: "eTag differs"}, "ETag mismatch"},
		{"precondition failed", &adt.ADTError{StatusCode: 412, Type: "ExceptionPreconditionFailed", Message: "Vorbedingung fehlgeschlagen"}, "ETag mismatch"},

		// Content negotiation.
		{"not acceptable", &adt.ADTError{StatusCode: 406, Type: "ExceptionResourceNotAcceptable", Message: "not acceptable"}, "negotiation"},
		{"unsupported media type", &adt.ADTError{StatusCode: 415, Type: "ExceptionUnsupportedMediaType", Message: "unsupported"}, "Content-Type"},

		// Semantic.
		{"unprocessable entity", &adt.ADTError{StatusCode: 422, Type: "ExceptionUnprocessableEntity", Message: "semantic errors"}, "semantic"},

		// Genuine method-not-allowed (S/4 only) — distinct Type from
		// "already exists", so it must NOT produce the already-exists hint.
		{"method not allowed", &adt.ADTError{StatusCode: 405, Type: "ExceptionNotAllowed", Message: "not allowed"}, "not allowed"},

		// Creation failure arrives as HTTP 500 (verified live on S/4+R/3:
		// the PROGRAM-create endpoint reports an existing name this way, not
		// as ExceptionResourceAlreadyExists — issue #406). The Tier-1 Type
		// rule must beat the generic Tier-2 {statusCode: 500} catch-all so the
		// user gets the actionable "already exists / object_exists" hint
		// instead of the misleading "check ST22 short dumps" guidance.
		{"creation failure (S/4, English)", &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceCreationFailure", Message: "A program or include already exists with the name ZFOO"}, "already exists"},
		{"creation failure (R/3, German)", &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceCreationFailure", Message: "Es existiert bereits ein Programm oder Include mit dem Namen ZFOO"}, "object_exists"},
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
		// A Type adt.ClassifyError does not recognise must fall through to
		// the status-code classification (here 400 -> bad request).
		{"unrecognised type falls through to status", &adt.ADTError{StatusCode: 400, Type: "ExceptionSomethingBrandNew", Message: "bad data"}, "Bad request"},
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
		// Real ReleaseTransport error text captured live from S/4 (issue
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

// TestMatchHint_ObjectLockedInTransport pins the #442 hint: a CTS "locked in
// request <TR>" 409 must produce a hint that names the blocking request and
// tells the caller to retry against it. The messages are captured verbatim
// from live S/4 (HF S/4 Mandant 100) — the classification lives in adtler
// (adt.ErrorObjectLockedInTransport + LockingTransport); this asserts the
// wrapper turns it into an actionable, transport-named hint.
func TestMatchHint_ObjectLockedInTransport(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"typed 409 DDLS", &adt.ADTError{StatusCode: 409, Type: "ExceptionResourceLockConflict", Message: "Object R3TR DDLS Z_ADT_MCP_LCK442 is already locked in request S4UK903759 of user KLEINK"}},
		{"typed 409 CINC", &adt.ADTError{StatusCode: 409, Type: "ExceptionResourceLockConflict", Message: "Object LIMU CINC /HFQ/BP_DD_ADRESSE============CCIMP is already locked in request S4UK901974 of user BECKT"}},
		{"wrapped", fmt.Errorf("set source: %w", &adt.ADTError{StatusCode: 409, Type: "ExceptionResourceLockConflict", Message: "Object R3TR DDLS Z_FOO is already locked in request S4UK903759 of user KLEINK"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			// Derive the expected request ID the same way production does, so a
			// new fixture can't silently assert the wrong transport.
			want, ok := lockingTransportOf(tt.err)
			if !ok {
				t.Fatalf("fixture message has no parseable transport: %s", tt.err.Error())
			}
			// The exact request ID from the message must appear...
			if !strings.Contains(hint, want) {
				t.Errorf("hint should name transport %q, got: %s", want, hint)
			}
			// ...and the hint must steer to retrying with that transport, not to
			// the (useless here) unlock_object/SM12 path.
			if !strings.Contains(hint, "transport="+want) {
				t.Errorf("hint should tell caller to retry with transport=%s, got: %s", want, hint)
			}
			if strings.Contains(hint, "Save conflict") {
				t.Errorf("should NOT fall back to the generic lock-conflict hint, got: %s", hint)
			}
		})
	}
}

// TestMatchHint_NoDeleteHandler pins the #404 hint: a 405 "... does not support
// method DELETE" (e.g. SAP Gateway VIT objects) must steer the user to a GUI /
// black-magic path rather than the generic method-not-allowed hint. A generic
// 405 must still get the generic hint.
func TestMatchHint_NoDeleteHandler(t *testing.T) {
	vit := &adt.ADTError{StatusCode: 405, Message: "Resource controller does not support method DELETE"}
	hint := matchHint(vit)
	if !strings.Contains(hint, "cannot be deleted via ADT") {
		t.Errorf("VIT delete 405 should get the no-delete-handler hint, got: %s", hint)
	}
	if !strings.Contains(hint, "sapwebgui") {
		t.Errorf("hint should point at a GUI path, got: %s", hint)
	}

	// A typed VIT 405 (ExceptionNotAllowed) must also match on the message.
	vitTyped := &adt.ADTError{StatusCode: 405, Type: "ExceptionNotAllowed", Message: "Resource controller does not support method DELETE"}
	if !strings.Contains(matchHint(vitTyped), "cannot be deleted via ADT") {
		t.Errorf("typed VIT delete 405 should also get the no-delete-handler hint")
	}

	// A generic 405 (not a delete-unsupported message) keeps the generic hint.
	generic := &adt.ADTError{StatusCode: 405, Message: "method not allowed"}
	if got := matchHint(generic); !strings.Contains(got, "Method not allowed (405)") {
		t.Errorf("generic 405 should keep the generic method-not-allowed hint, got: %s", got)
	}

	// The branch gates on kind AND text: the same message on a non-405 status
	// must NOT produce the no-delete hint.
	non405 := &adt.ADTError{StatusCode: 400, Message: "Resource controller does not support method DELETE"}
	if got := matchHint(non405); strings.Contains(got, "cannot be deleted via ADT") {
		t.Errorf("no-delete hint must require a 405, not just the message; got: %s", got)
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
// Hints are appended for known error patterns (see hintByKind in errors.go).
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
