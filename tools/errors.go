package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// Recovery hints, keyed below by adt.ErrorKind. These reference MCP tool names
// (`unlock_object`, `search_objects`, …) and are the one genuinely MCP-side
// concern in error handling — the SAP-stable classification of which exception
// Type / status code means what now lives in adtler (adt.ClassifyError).
const (
	etagMismatchHint     = "ETag mismatch — the object was modified since your lock was acquired. Re-lock the object and retry the write."
	lockConflictHint     = "Save conflict — another process holds a conflicting lock. Use `get_transport_requests` to check the locking transport, or `unlock_object` if the lock is stale."
	alreadyExistsHint    = "Object already exists. Use `search_objects` to find it, or choose a different name."
	lockedHint           = "Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport."
	notAcceptableHint    = "Content negotiation failed (406) — the server cannot produce the requested Accept type. Check the Accept header, or try the other system's API version."
	unsupportedMediaHint = "Unsupported media type (415) — the request Content-Type is not accepted. Check the Content-Type header."
	unprocessableHint    = "Request rejected due to semantic errors (422) — check that all required fields and parameter values are valid."
	// methodNotAllowedHint covers both genuine method-not-allowed (S/4) and the
	// bare-405 case where ECC reports an existing object as a 405, so it names
	// both possibilities (adt.ClassifyError collapses both to
	// ErrorMethodNotAllowed). See #406.
	methodNotAllowedHint = "Method not allowed (405) — either the operation is not supported for this resource, or (on ECC) the object already exists. Check with `object_exists` / `search_objects`."
	// noDeleteHandlerHint: some resources have no DELETE handler and reject it
	// with 405 "... does not support method DELETE" — notably SAP Gateway VIT
	// objects (IWSG/IWOM/OA2S). ADT cannot delete these at all; the correct 406
	// Accept header (adtler#73) gets past the ETag fetch, but the DELETE itself
	// is unsupported over ADT REST. Point the user at a GUI/black-magic path.
	// Matched on localised message text (see matchHint), so it misses on
	// non-English systems and degrades to methodNotAllowedHint there.
	// See #404 (local) and adtler#73 (the 406 Accept-header fix).
	noDeleteHandlerHint = "This object cannot be deleted via ADT — the resource has no DELETE handler (e.g. SAP Gateway VIT objects: IWSG/IWOM/OA2S). Delete it from a GUI (SE80 / SEGW) via the `sapwebgui` MCP, or use a BlackMagic-backed path."
	// creationFailedHint: object-creation endpoints report a name collision as
	// ExceptionResourceCreationFailure (HTTP 500), not as
	// ExceptionResourceAlreadyExists — so name that likely cause first rather
	// than the generic "check ST22" 500 guidance (#406 / #407).
	creationFailedHint = "Object creation failed. The most common cause is that an object with that name already exists — check with `object_exists` or `search_objects`, or choose a different name. Otherwise verify the name, package, and that this object type is supported on this system."
	notFoundHint       = "Object not found. Check the URI spelling or use `search_objects` to find it."
	forbiddenHint      = "Authorization error. Check that the ADT user has the required S_DEVELOP authorizations."
	// systemNotModifiableHint: a 403 ExceptionResourceNoAccess whose message
	// says the system change option is "not modifiable" is a system-wide
	// configuration state, not an authorization problem — no role or profile
	// change fixes it. Matched structurally when the T100 key is present
	// (t100KeyIDSystemChange/t100KeyNoSystemNotModifiable below), else on
	// message text: German confirmed live via the #490 ECC repro, English
	// from the SE91 text of the same T100 key (see matchHint). That repro's
	// body carried no T100KEY at all (message-only, consistent with #378's
	// ECC-sparser-body finding), so the text fallback still matters there
	// even with the key known. See #490.
	systemNotModifiableHint = "The SAP system is closed for changes (system change option is 'not modifiable'). This is a system-wide setting, not an authorization or lock problem: writes to repository objects will fail until an administrator reopens it in SE06 -> System Change Option. Read-only tools are unaffected."
	badRequestHint          = "Bad request — the server rejected the request. Check the syntax, required parameters, or the CSRF token."
	serverErrorHint         = "SAP server error. Retry once — if it persists, check SM21 (system log) or ST22 (short dumps)."
	transportHint           = "A transport request may be required. Use `create_transport` or `get_transport_requests` to find one."
	inactiveHint            = "An object is inactive — activate it with `activate_objects` (including its dependencies) before releasing the transport or retrying."
	// objectLockedInTransportHint names the blocking request (parsed by adtler
	// from the 409 message — see adt.ADTError.LockingTransport) so the caller
	// can act on it directly. The %[1]s verb is the request ID, reused twice.
	// This is a CTS object-directory registration, a different lock domain from
	// the runtime ENQUEUE, so unlock_object/force_unlock/SM12 do NOT clear it —
	// the fix is to write to the request the object is already registered in.
	// The request may belong to another user (as in #442's own repro), so we do
	// not auto-retry; we surface it for the caller to decide. See #442.
	objectLockedInTransportHint = "The object is already registered in open transport request `%[1]s` — a CTS registration, distinct from the runtime lock, so `unlock_object`/`force_unlock`/SM12 will not clear it. Retry the write with `transport=%[1]s`. If `%[1]s` is not yours to use, coordinate with its owner or reassign the object via SE09/SE10."
	// invalidLockHandleHint (#378 finding 3): a stale/expired lock handle
	// (423 ExceptionResourceInvalidLockHandle, SADT_RESOURCE/026, S/4 only —
	// ECC silently accepts a bogus handle, see #377) means there is no real
	// lock to drop, so unlock_object does nothing useful here. The fix is to
	// acquire a fresh handle.
	invalidLockHandleHint = "Invalid or expired lock handle (423 `ExceptionResourceInvalidLockHandle` / SADT_RESOURCE-026) — there is no real lock to release, so `unlock_object` will not help. Call `lock_object` to acquire a fresh handle, then retry the write with it."
	// corrNrMissingHint (#378 finding 1): a 400 ExceptionParameterNotFound
	// naming corrNr specifically means the object is not $TMP-local and a
	// transport is required. This is one of two unrelated conditions that
	// share Type=ExceptionParameterNotFound (the other names lockHandle, see
	// adt.isLockHandleParameterNotFound in adtler).
	corrNrMissingHint = "Missing `transport` parameter (400 `ExceptionParameterNotFound`, SADT_RESOURCE/017 — this object is not `$TMP`-local). Use `get_transport_requests` to find one, or `create_transport` to make a new one, then retry with `transport=<request>`."
	// ownAccessConflictHintFmt (#378 finding 2): SAP's ExceptionResourceNoAccess
	// (403, EU/510) is worded as "User X is currently editing", but the named
	// user is frequently the CALLING user's own stale enqueue from an earlier
	// session, not a real concurrent editor. matchHint has no notion of "the
	// active user" to compare against, so the hint names the user structurally
	// (via adt.ADTError.IsEnqueueLock) and lets the caller decide by comparing
	// it against themselves. %s is that user.
	ownAccessConflictHintFmt = "Resource access denied (403 `ExceptionResourceNoAccess` / EU-510) — despite the \"currently editing\" wording, `%s` is often your own stale lock from an earlier session, not a real concurrent editor. If that's you, call `unlock_object` to drop the stale lock and retry; otherwise wait, or check SM12 for the lock owner."
)

// t100KeyIDSystemChange / t100KeyNoSystemNotModifiable = SE91's TK/102, the
// text of "SAP-System hat den Status \"nicht änderbar\"" / "SAP system has
// status 'not modifiable'". Whether ADT bodies actually carry this T100KEY is
// unconfirmed — the #490 ECC repro had none (see systemNotModifiableHint) —
// hence isSystemNotModifiable's text fallback. Kept local rather than an
// adtler-side predicate like IsEnqueueLock/IsTransportLocked for now; a
// follow-up adtler issue for that once the key is confirmed on a live body
// would fit the existing pattern.
const (
	t100KeyIDSystemChange        = "TK"
	t100KeyNoSystemNotModifiable = "102"
)

// lockOwnerRe matches the trailing "... of user <NAME>" in a CTS lock message
// (e.g. "Object R3TR PROG Z_FOO is already locked in request ZZZK900001 of
// user SMITH"). ECC's #378-finding-4 body carries only a bare "corrNr"
// property with no T100KEY at all (confirmed live against Z_ADT_MCP_TEST —
// unlike S/4, which adt.ADTError.IsTransportLocked covers structurally), so
// the owner still needs this text fallback there.
var lockOwnerRe = regexp.MustCompile(`(?i)of user (\S+)`)

// lockingOwnerOf extracts the user named in a "... locked in request <TR> of
// user <owner>" message, unwrapping to the underlying *adt.ADTError.
func lockingOwnerOf(err error) (string, bool) {
	adtErr, ok := asADTError(err)
	if !ok {
		return "", false
	}
	m := lockOwnerRe.FindStringSubmatch(adtErr.Message)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// transportSaveConflictHint (#378 finding 4): SAP reports a transport
// ownership conflict on write as a bare 500 ExceptionResourceSaveFailure /
// CTS_WBO_API-020 — spec-wise this should be 409/423, but it is what SAP
// actually returns on both S/4 and ECC, and adtler has no named ErrorKind for
// it (it falls through to the generic ErrorServerError).
func transportSaveConflictHint(tr, owner string) string {
	hint := fmt.Sprintf("Save failed — the object is already registered in open transport request `%[1]s` (500 `ExceptionResourceSaveFailure` / CTS_WBO_API-020; SAP reports this CTS registration conflict as a 500 instead of 409/423). Retry the write with `transport=%[1]s`.", tr)
	if owner != "" {
		hint += fmt.Sprintf(" If `%s` is not yours to use, coordinate with its owner (`%s`) or reassign the object via SE09/SE10.", tr, owner)
	}
	return hint
}

// hintByKind maps an adt.ErrorKind to the MCP-flavored recovery hint. Kinds
// absent from the map (e.g. adt.ErrorUnknown) get no hint here and fall through
// to the localised-text fallbacks in matchHint.
var hintByKind = map[adt.ErrorKind]string{
	adt.ErrorLocked:            lockedHint,
	adt.ErrorInvalidLockHandle: invalidLockHandleHint,
	adt.ErrorLockConflict:      lockConflictHint,
	adt.ErrorAlreadyExists:     alreadyExistsHint,
	adt.ErrorEtagMismatch:      etagMismatchHint,
	adt.ErrorNotAcceptable:     notAcceptableHint,
	adt.ErrorUnsupportedMedia:  unsupportedMediaHint,
	adt.ErrorUnprocessable:     unprocessableHint,
	adt.ErrorMethodNotAllowed:  methodNotAllowedHint,
	adt.ErrorCreationFailed:    creationFailedHint,
	adt.ErrorNotFound:          notFoundHint,
	adt.ErrorForbidden:         forbiddenHint,
	adt.ErrorBadRequest:        badRequestHint,
	adt.ErrorServerError:       serverErrorHint,
}

// errorResult converts an error to an MCP error result with the SAP error
// message. If the error matches a known pattern, an actionable hint is
// appended to help the LLM recover.
//
// The text content carries the `"Error: <full error string>"` payload that
// every client has historically consumed. StructuredContent is deliberately
// left unset on the error path: MCP 2025-06-18 /server/tools requires
// structuredContent to conform to the declared outputSchema with no
// exemption for isError=true, so a typed error DTO would contradict every
// tool's declared output shape and be rejected by strict clients. Absence is
// spec-legal; clients extract the wrapped SAP status code — if needed — from
// the `"SAP ADT error N:"` prefix produced by adt.ADTError.Error(), which
// flows into the text content untouched.
func errorResult(err error) *mcp.CallToolResult {
	msg := fmt.Sprintf("Error: %s", err.Error())
	if hint := matchHint(err); hint != "" {
		msg += "\n\nHint: " + hint
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.NewTextContent(msg),
		},
	}
}

// matchHint returns an actionable recovery hint for an error, or "" if none
// applies. It classifies the error via adt.ClassifyError (which prefers the
// SAP-stable exception Type over the HTTP status code) and looks up the hint
// wording by kind, with several consumer-side refinements that adtler's
// protocol-level classification intentionally does not cover:
//
//   - An object locked in another open CTS request (ErrorObjectLockedInTransport)
//     gets a hint naming that request (dynamic, so not in the static map).
//     No structural field for this one — still parsed from the message via
//     adt.ADTError.LockingTransport.
//   - The same CTS-lock conflict reported as a bare 500 (ExceptionResourceSaveFailure,
//     #378 finding 4) gets the same treatment, since adtler classifies it as the
//     generic ErrorServerError rather than a distinct kind. Matched
//     structurally via adt.ADTError.IsTransportLocked (Type + T100KEY), with
//     a fallback to the bare "corrNr" property for ECC's sparser body (see
//     that branch's comment).
//   - A 405 "… does not support method DELETE" (e.g. Gateway VIT objects) gets
//     the no-delete-handler hint instead of the generic method-not-allowed one.
//   - ExceptionResourceNoAccess (403, EU/510, #378 finding 2) gets a hint
//     naming the likely-stale-own-lock cause instead of the generic
//     authorization hint. Matched structurally via
//     adt.ADTError.IsEnqueueLock (Type + T100KEY together, not Type alone).
//   - ExceptionParameterNotFound naming corrNr (400, #378 finding 1) gets a
//     hint pointing at create_transport/get_transport_requests instead of the
//     generic bad-request hint. On S/4 the T100KEY-V1 placeholder names the
//     missing parameter structurally; ECC's body carries no T100KEY for this
//     Type at all (confirmed live), so it falls back to the literal parameter
//     name in the message, same as adtler's own sibling
//     isLockHandleParameterNotFound check for the "lockHandle" variant of
//     this same overloaded Type.
//   - A 400 that mentions a transport gets the more specific transport hint
//     instead of the generic bad-request hint.
//   - A 403 whose message says the system change option is closed ("not
//     modifiable", SAP message class TK/102) gets a hint naming SE06
//     instead of the generic authorization hint — SAP reports this with
//     the same ExceptionResourceNoAccess Type as a genuine auth error, so
//     Type alone cannot distinguish them (#490). Matched on the T100 key
//     when present, else on message text in English and German (the two
//     variants confirmed live so far).
//   - Errors that carry no ADT Type or status — plain Go errors such as the
//     ReleaseTransport "… is inactive" failure, or our own English
//     "already exists" messages — are matched on localised text as a last
//     resort.
//
// The 405 refinement, the not-modifiable text fallback, and the ECC
// fallbacks noted above match on localised message text, so they are
// language-fragile: they silently miss on a logon language none of the
// matched variants cover, and degrade to the kind-based hint. That tradeoff
// is accepted for conditions with no clean structural signal (#406, #404) or
// where SAP's own body is simply sparser (ECC, #378, #490).
func matchHint(err error) string {
	kind := adt.ClassifyError(err)
	errText := strings.ToLower(err.Error())
	adtErr, isADTErr := asADTError(err)

	// Object registered in another open CTS request: name that request so the
	// caller can retarget the write at it. Dynamic (embeds the parsed request
	// ID), so it cannot live in the static hintByKind map.
	if kind == adt.ErrorObjectLockedInTransport {
		if tr, ok := lockingTransportOf(err); ok {
			return fmt.Sprintf(objectLockedInTransportHint, tr)
		}
		// adtler only assigns this kind when a request ID was parsed, so this is
		// effectively unreachable; fall back to the generic conflict hint.
		return lockConflictHint
	}

	// #378 finding 4: the same "locked in request <TR> of user <owner>"
	// conflict, but reported as a bare 500 (ExceptionResourceSaveFailure /
	// CTS_WBO_API-020) instead of 409 — adtler has no ErrorKind for it, so it
	// classifies as the generic ErrorServerError. Gate on the exception Type
	// first (adtler's own IsTransportLocked doc notes this Type is overloaded
	// across unrelated save failures, so Type alone is not enough — an
	// unrelated 500 that happens to name its own request in the message must
	// NOT get a "retry with transport=X" hint pointing back at itself).
	// IsTransportLocked is structural (Type + T100KEY) and covers S/4; ECC's
	// body carries only a bare "corrNr" property with no T100KEY at all
	// (confirmed live). The fallback additionally requires T100KeyID=="" —
	// not just IsTransportLocked returning false — so a save failure that DOES
	// carry a T100KEY, just not the CTS_WBO_API/020 one, is treated as the
	// unrelated condition it is rather than matched on corrNr alone. This
	// doesn't fully eliminate the overload risk IsTransportLocked's doc warns
	// about (a truly T100KEY-less, unrelated save failure that happens to
	// carry its own corrNr would still match), but narrows it to bodies
	// exactly as sparse as ECC's confirmed-live shape.
	if isADTErr && adtErr.Type == adt.ExceptionTypeResourceSaveFailure {
		if tr, owner, ok := adtErr.IsTransportLocked(); ok {
			return transportSaveConflictHint(tr, owner)
		}
		if adtErr.T100KeyID == "" {
			if tr, ok := adtErr.Properties["corrNr"]; ok && tr != "" {
				owner, _ := lockingOwnerOf(err)
				return transportSaveConflictHint(tr, owner)
			}
		}
	}

	// A resource with no DELETE handler (405 "... does not support method
	// DELETE" — e.g. SAP Gateway VIT objects) gets the actionable no-delete
	// hint instead of the generic method-not-allowed one. See #404.
	if kind == adt.ErrorMethodNotAllowed && strings.Contains(errText, "does not support method delete") {
		return noDeleteHandlerHint
	}

	// #378 finding 2: ExceptionResourceNoAccess (403, EU/510) is a distinct,
	// actionable condition — SAP's "currently editing" wording usually means a
	// stale own-lock, not a real conflict — so it must beat the generic
	// forbiddenHint that a bare 403 would otherwise get. IsEnqueueLock checks
	// Type and T100KEY together, so a differently-keyed ExceptionResourceNoAccess
	// can't collide with this.
	if isADTErr {
		if user, _, ok := adtErr.IsEnqueueLock(); ok {
			if user == "" {
				// T100KEY-V1 unset despite matching EU/510 — not observed live,
				// but avoid rendering an empty `` in the hint if SAP ever sends it.
				user = "the named user"
			}
			return fmt.Sprintf(ownAccessConflictHintFmt, user)
		}
	}

	// #378 finding 1: ExceptionParameterNotFound naming corrNr specifically
	// beats both the generic transport-mention check below and the
	// catch-all bad-request hint.
	if kind == adt.ErrorBadRequest && isADTErr && adtErr.Type == adt.ExceptionTypeParameterNotFound {
		if adtErr.T100Vars[0] == "corrNr" || strings.Contains(errText, "corrnr") {
			return corrNrMissingHint
		}
	}

	// Transport-specific 400 beats the generic bad-request hint.
	if kind == adt.ErrorBadRequest && strings.Contains(errText, "transport") {
		return transportHint
	}

	// System change option closed (SAP message class TK, number 102) beats the
	// generic forbidden/authorization hint — no auth change fixes it. Tries
	// the structural T100 key first (works regardless of logon language, if
	// the body carries one — see t100KeyIDSystemChange); falls back to text,
	// narrowly on "system has status"/"sap-system hat den status" together
	// with "not modifiable"/"nicht änderbar", not "not modifiable" alone: SE06
	// can also close a single software component or namespace for changes,
	// and that 403 likely names the component/namespace instead of "system"
	// — wording unconfirmed live, so the narrower match avoids mislabeling
	// that case as system-wide. German confirmed live, English from the SE91
	// text of the same key (#490); other logon languages still fall through
	// to forbiddenHint until observed. Gated on 403 because that's the only
	// status this has been observed on (write/lock paths) — a hypothetical
	// TK/102 on another status (e.g. object creation) wouldn't get this hint.
	if kind == adt.ErrorForbidden && isSystemNotModifiable(adtErr, isADTErr, errText) {
		return systemNotModifiableHint
	}

	if hint, ok := hintByKind[kind]; ok {
		return hint
	}

	// Tier 3: localised-text fallbacks for errors with no ADT Type/status.
	switch {
	case strings.Contains(errText, "already exists"):
		return alreadyExistsHint
	case strings.Contains(errText, "inactive"):
		return inactiveHint
	}
	return ""
}

// lockingTransportOf extracts the CTS request named in a "locked in request
// <TR>" conflict, unwrapping to the underlying *adt.ADTError. The parse itself
// lives in adtler (adt.ADTError.LockingTransport).
func lockingTransportOf(err error) (string, bool) {
	adtErr, ok := asADTError(err)
	if !ok {
		return "", false
	}
	return adtErr.LockingTransport()
}

// asADTError unwraps err to the underlying *adt.ADTError, if any. Shared by
// every structural-matching branch in matchHint (IsTransportLocked,
// IsEnqueueLock, T100Vars, Properties) and the message-scraping fallbacks
// that still need one.
func asADTError(err error) (*adt.ADTError, bool) {
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) {
		return adtErr, true
	}
	return nil, false
}

// isSystemNotModifiable reports whether an error the caller has already
// classified as forbidden (403, kind == adt.ErrorForbidden) is SAP's
// system-change-option-closed condition (message class TK, number 102),
// checked structurally via the T100 key when present, else via message text:
// German confirmed live (#490), English from the SE91 text of the same key.
// See the matchHint call site for the narrower-than-"not modifiable"-alone
// rationale.
func isSystemNotModifiable(adtErr *adt.ADTError, isADTErr bool, errText string) bool {
	if isADTErr && adtErr.T100KeyID == t100KeyIDSystemChange && adtErr.T100KeyNo == t100KeyNoSystemNotModifiable {
		return true
	}
	if strings.Contains(errText, "system has status") && strings.Contains(errText, "not modifiable") {
		return true
	}
	return strings.Contains(errText, "sap-system hat den status") && strings.Contains(errText, "nicht änderbar")
}
