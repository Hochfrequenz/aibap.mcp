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
// from a live S/4 system — the classification lives in adtler
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

// TestMatchHint_378_WriteErrorClasses pins the four write-error classes from
// issue #378. Structural fixtures (Properties/T100KeyID/T100KeyNo/T100Vars)
// use the shape confirmed live against Z_ADT_MCP_TEST_REPORT on an S/4
// system; sparse fixtures use the shape confirmed live on an ECC system,
// where the same conditions carry little or no T100KEY data. Transport/user
// values are placeholders, not the real values from those captures.
func TestMatchHint_378_WriteErrorClasses(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantHint    string
		notWantHint string
	}{
		{
			name: "400 corrNr missing, S/4 structural (T100Vars[0], no message hint) names create_transport",
			err: &adt.ADTError{StatusCode: 400, Type: "ExceptionParameterNotFound",
				Message:    "Parameter X could not be found.", // deliberately doesn't say "corrNr" — proves the structural path, not the text fallback, matched
				Properties: map[string]string{"T100KEY-ID": "SADT_RESOURCE", "T100KEY-NO": "017", "T100KEY-V1": "corrNr"},
				T100KeyID:  "SADT_RESOURCE", T100KeyNo: "017", T100Vars: [4]string{"corrNr", "", "", ""},
			},
			wantHint:    "create_transport",
			notWantHint: "Bad request —",
		},
		{
			name:        "400 corrNr missing, ECC sparse (no Properties at all) falls back to message text",
			err:         &adt.ADTError{StatusCode: 400, Type: "ExceptionParameterNotFound", Message: "Parameter corrNr wurde nicht gefunden"},
			wantHint:    "create_transport",
			notWantHint: "Bad request —",
		},
		{
			// Message deliberately names a DIFFERENT user than T100Vars[0]: proves
			// the hint's username comes from the structural field, not the message.
			name: "403 EU/510 own-stale-lock, structural (T100Vars, message names a different user) names the T100Vars user",
			err: &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess",
				Message:    "User WRONGNAME is currently editing Z_ADT_MCP_TEST_REPORT",
				Properties: map[string]string{"T100KEY-ID": "EU", "T100KEY-NO": "510", "T100KEY-V1": "SMITH", "T100KEY-V2": "Z_ADT_MCP_TEST_REPORT"},
				T100KeyID:  "EU", T100KeyNo: "510", T100Vars: [4]string{"SMITH", "Z_ADT_MCP_TEST_REPORT", "", ""},
			},
			wantHint:    "unlock_object",
			notWantHint: "S_DEVELOP",
		},
		{
			name:        "403 with ExceptionResourceNoAccess Type but no T100KEY falls back to the generic forbidden hint, not a broken EU/510 one",
			err:         &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "some other 403 with this overloaded Type"},
			wantHint:    "S_DEVELOP",
			notWantHint: "unlock_object",
		},
		{
			name:        "423 invalid lock handle points at lock_object, not unlock_object as the fix",
			err:         &adt.ADTError{StatusCode: 423, Type: "ExceptionResourceInvalidLockHandle", Message: "Resource INCLUDE Z_ADT_MCP_TEST_REPORT is not locked (invalid lock handle: DEADBEEF)"},
			wantHint:    "Call `lock_object`",
			notWantHint: "Call `unlock_object`",
		},
		{
			name: "500 transport-conflict, S/4 structural (no transport/owner in message) names both",
			err: &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceSaveFailure",
				Message:    "Object is already locked", // deliberately carries neither the request ID nor the owner — proves the structural path, not message scraping
				Properties: map[string]string{"T100KEY-ID": "CTS_WBO_API", "T100KEY-NO": "020", "T100KEY-V4": "SMITH", "corrNr": "ZZZK900001"},
				T100KeyID:  "CTS_WBO_API", T100KeyNo: "020", T100Vars: [4]string{"", "", "", "SMITH"},
			},
			wantHint: "transport=ZZZK900001",
		},
		{
			name: "500 transport-conflict, ECC sparse (bare corrNr property, no T100KEY) falls back to message text for the owner",
			err: &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceSaveFailure",
				Message:    "Objekt ... ist bereits in Auftrag YYYK900002 von Benutzer SMITH gesperrt",
				Properties: map[string]string{"corrNr": "YYYK900002"},
			},
			wantHint: "transport=YYYK900002",
		},
		{
			name:        "500 with ExceptionResourceSaveFailure Type but no corrNr property is an unrelated save failure, not finding 4",
			err:         &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceSaveFailure", Message: "some other save failure sharing this overloaded Type"},
			wantHint:    "SM21",
			notWantHint: "transport=",
		},
		{
			// A DIFFERENT, non-empty T100KEY that happens to carry a corrNr
			// property (SAP echoing the caller's own request back, say) must
			// NOT be treated as finding 4 — the corrNr fallback requires
			// T100KeyID=="" (ECC's confirmed-live sparse shape), not just
			// "IsTransportLocked returned false".
			name: "500 ExceptionResourceSaveFailure with an UNRELATED T100KEY plus a corrNr property is not finding 4",
			err: &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceSaveFailure",
				Message:    "some unrelated save failure that happens to echo a corrNr",
				Properties: map[string]string{"T100KEY-ID": "SOME_OTHER", "T100KEY-NO": "999", "corrNr": "ZZZK900099"},
				T100KeyID:  "SOME_OTHER", T100KeyNo: "999",
			},
			wantHint:    "SM21",
			notWantHint: "transport=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := matchHint(tt.err)
			if !strings.Contains(hint, tt.wantHint) {
				t.Errorf("hint should contain %q, got: %s", tt.wantHint, hint)
			}
			if tt.notWantHint != "" && strings.Contains(hint, tt.notWantHint) {
				t.Errorf("hint should NOT contain %q, got: %s", tt.notWantHint, hint)
			}
		})
	}

	// The structural 500 case must also name the owner from T100Vars[3], and
	// must not collide with the unrelated 409 ObjectLockedInTransport hint
	// wording.
	saveConflict := &adt.ADTError{StatusCode: 500, Type: "ExceptionResourceSaveFailure",
		Message:    "Object is already locked",
		Properties: map[string]string{"T100KEY-ID": "CTS_WBO_API", "T100KEY-NO": "020", "T100KEY-V4": "SMITH", "corrNr": "ZZZK900001"},
		T100KeyID:  "CTS_WBO_API", T100KeyNo: "020", T100Vars: [4]string{"", "", "", "SMITH"},
	}
	hint := matchHint(saveConflict)
	if !strings.Contains(hint, "SMITH") {
		t.Errorf("500 transport-conflict hint should name the owner, got: %s", hint)
	}
	if !strings.Contains(hint, "500") {
		t.Errorf("500 transport-conflict hint should note the surprising status code, got: %s", hint)
	}

	// The structural EU/510 case must name the user from T100Vars[0], NOT
	// whatever name happens to appear in the message — proves the hint is
	// actually sourced from the structural field.
	enqueueLock := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess",
		Message:    "User WRONGNAME is currently editing Z_ADT_MCP_TEST_REPORT",
		Properties: map[string]string{"T100KEY-ID": "EU", "T100KEY-NO": "510", "T100KEY-V1": "SMITH", "T100KEY-V2": "Z_ADT_MCP_TEST_REPORT"},
		T100KeyID:  "EU", T100KeyNo: "510", T100Vars: [4]string{"SMITH", "Z_ADT_MCP_TEST_REPORT", "", ""},
	}
	euHint := matchHint(enqueueLock)
	if !strings.Contains(euHint, "SMITH") {
		t.Errorf("EU/510 hint should name the T100Vars[0] user, got: %s", euHint)
	}
	if strings.Contains(euHint, "WRONGNAME") {
		t.Errorf("EU/510 hint should NOT name the message's user — it must be structurally sourced, got: %s", euHint)
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

// TestMatchHint_SystemNotModifiable pins the #490 hint: a 403 ExceptionResourceNoAccess
// whose message says the system change option is "not modifiable" is a system-wide
// configuration state, not an authorization problem. It must NOT get the generic
// S_DEVELOP authorization hint, and a generic 403 must still get that hint.
func TestMatchHint_SystemNotModifiable(t *testing.T) {
	notModifiable := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "SAP system has status 'not modifiable'"}
	if got := matchHint(notModifiable); got != systemNotModifiableHint {
		t.Errorf("got: %s, want: %s", got, systemNotModifiableHint)
	}

	// Wrapped and bare-403 (no Type) variants must match the same way.
	wrapped := fmt.Errorf("lock_object: %w", notModifiable)
	if got := matchHint(wrapped); got != systemNotModifiableHint {
		t.Errorf("wrapped error: got: %s, want: %s", got, systemNotModifiableHint)
	}
	bareNoType := &adt.ADTError{StatusCode: 403, Message: "SAP system has status 'not modifiable'"}
	if got := matchHint(bareNoType); got != systemNotModifiableHint {
		t.Errorf("no-Type 403: got: %s, want: %s", got, systemNotModifiableHint)
	}

	// A generic 403 (e.g. the "currently editing" case, or genuinely missing
	// authorizations) must keep the existing S_DEVELOP hint.
	generic := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "User SMITH is currently editing Z_REPORT"}
	if got := matchHint(generic); got != forbiddenHint {
		t.Errorf("generic 403: got: %s, want: %s", got, forbiddenHint)
	}

	// "not modifiable" alone, without "system has status", must NOT match —
	// SE06 can close a single software component or namespace, whose message
	// wording is unconfirmed; the narrower match avoids mislabeling that case
	// as system-wide (see #490 review discussion).
	componentClosed := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "Software component ZFOO has status 'not modifiable'"}
	if got := matchHint(componentClosed); got != forbiddenHint {
		t.Errorf("component-level not-modifiable: got: %s, want: %s (falls back to generic until wording confirmed)", got, forbiddenHint)
	}

	// A non-403 error mentioning "not modifiable" must not match the branch
	// at all (the kind==ErrorForbidden guard).
	non403 := &adt.ADTError{StatusCode: 500, Message: "system has status 'not modifiable'"}
	if got := matchHint(non403); got == systemNotModifiableHint {
		t.Errorf("non-403 must not get the not-modifiable hint, got: %s", got)
	}

	// German logon language: the exact text observed in the #490 live repro
	// against an ECC system (message class TK, number 102), no T100KEY in the
	// body (ECC's sparser-body pattern, #378).
	german := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: `SAP-System hat den Status "nicht änderbar"`}
	if got := matchHint(german); got != systemNotModifiableHint {
		t.Errorf("German text: got: %s, want: %s", got, systemNotModifiableHint)
	}

	// Structural T100 key match: fires even when the message text is neither
	// of the two confirmed languages, as long as the key is present.
	structural := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "Some other-language rendering", T100KeyID: "TK", T100KeyNo: "102"}
	if got := matchHint(structural); got != systemNotModifiableHint {
		t.Errorf("structural T100 key: got: %s, want: %s", got, systemNotModifiableHint)
	}

	// German component-level negative, mirroring componentClosed above: the
	// German text fallback must require the "sap-system hat den status"
	// prefix too, not "nicht änderbar" alone.
	germanComponentClosed := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: `Softwarekomponente ZFOO hat den Status "nicht änderbar"`}
	if got := matchHint(germanComponentClosed); got != forbiddenHint {
		t.Errorf("German component-level not-modifiable: got: %s, want: %s", got, forbiddenHint)
	}

	// Near-miss T100 key: both halves must match, not just one.
	nearMissKey := &adt.ADTError{StatusCode: 403, Type: "ExceptionResourceNoAccess", Message: "Some other-language rendering", T100KeyID: "TK", T100KeyNo: "103"}
	if got := matchHint(nearMissKey); got != forbiddenHint {
		t.Errorf("near-miss T100 key: got: %s, want: %s", got, forbiddenHint)
	}

	// EU/510 must win precedence even if the message text also happens to
	// contain the not-modifiable wording — the structural EU/510 check in
	// matchHint runs first.
	euWithNotModifiableText := &adt.ADTError{
		StatusCode: 403, Type: "ExceptionResourceNoAccess",
		Message:   `User SMITH is currently editing Z_REPORT (system has status 'not modifiable')`,
		T100KeyID: "EU", T100KeyNo: "510", T100Vars: [4]string{"SMITH", "Z_REPORT", "", ""},
	}
	if got := matchHint(euWithNotModifiableText); got != fmt.Sprintf(ownAccessConflictHintFmt, "SMITH") {
		t.Errorf("EU/510 precedence: got: %s, want: %s", got, fmt.Sprintf(ownAccessConflictHintFmt, "SMITH"))
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
