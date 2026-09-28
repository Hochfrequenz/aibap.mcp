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
	badRequestHint     = "Bad request — the server rejected the request. Check the syntax, required parameters, or the CSRF token."
	serverErrorHint    = "SAP server error. Retry once — if it persists, check SM21 (system log) or ST22 (short dumps)."
	transportHint      = "A transport request may be required. Use `create_transport` or `get_transport_requests` to find one."
	inactiveHint       = "An object is inactive — activate it with `activate_objects` (including its dependencies) before releasing the transport or retrying."
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
	// adt.isLockHandleParameterNotFound in adtler) — matched on the literal
	// parameter name for the same reason adtler does: no structured T100KEY
	// data yet (adtler#56).
	corrNrMissingHint = "Missing `transport` parameter (400 `ExceptionParameterNotFound`, SADT_RESOURCE/017 — this object is not `$TMP`-local). Use `get_transport_requests` to find one, or `create_transport` to make a new one, then retry with `transport=<request>`."
	// ownAccessConflictHint (#378 finding 2): SAP's ExceptionResourceNoAccess
	// (403, EU/510) is worded as "User X is currently editing", but the named
	// user is frequently the CALLING user's own stale enqueue from an earlier
	// session, not a real concurrent editor. matchHint has no notion of "the
	// active user" to compare against, so the hint names both recovery paths
	// and lets the caller decide by comparing the username in the message
	// above against themselves.
	ownAccessConflictHint = "Resource access denied (403 `ExceptionResourceNoAccess` / EU-510) — despite the \"currently editing\" wording, the named user is often your own stale lock from an earlier session, not a real concurrent editor. If that user is you, call `unlock_object` to drop the stale lock and retry; otherwise wait, or check SM12 for the lock owner."
)

// lockOwnerRe matches the trailing "... of user <NAME>" in a CTS lock message
// (e.g. "Object R3TR PROG Z_FOO is already locked in request ZZZK900001 of
// user SMITH"). Same message-scraping trade-off as adt.ADTError's
// ctsRequestRe: SAP gives no structured field for this until adtler#56 lands.
var lockOwnerRe = regexp.MustCompile(`(?i)of user (\S+)`)

// lockingOwnerOf extracts the user named in a "... locked in request <TR> of
// user <owner>" message, unwrapping to the underlying *adt.ADTError.
func lockingOwnerOf(err error) (string, bool) {
	var adtErr *adt.ADTError
	if !errors.As(err, &adtErr) {
		return "", false
	}
	m := lockOwnerRe.FindStringSubmatch(adtErr.Message)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// exceptionTypeResourceSaveFailure is SAP's Type id for the #378 finding 4
// condition. adtler does not export a named constant for it (unlike the
// adt.ExceptionType* constants used elsewhere in this file), so it is
// declared locally rather than inline in matchHint.
const exceptionTypeResourceSaveFailure = "ExceptionResourceSaveFailure"

// transportSaveConflictHint (#378 finding 4): SAP reports a transport
// ownership conflict on write as a bare 500 ExceptionResourceSaveFailure /
// CTS_WBO_API-020 — spec-wise this should be 409/423, but it is what SAP
// actually returns on both S/4 and ECC, and adtler has no named ErrorKind for
// it (it falls through to the generic ErrorServerError). The message carries
// the same "locked in request <TR> of user <owner>" shape as the 409 CTS-lock
// case, so it reuses lockingTransportOf/lockingOwnerOf rather than a new
// regex.
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
//   - The same CTS-lock conflict reported as a bare 500 (ExceptionResourceSaveFailure,
//     #378 finding 4) gets the same treatment, since adtler classifies it as the
//     generic ErrorServerError rather than a distinct kind.
//   - A 405 "… does not support method DELETE" (e.g. Gateway VIT objects) gets
//     the no-delete-handler hint instead of the generic method-not-allowed one.
//   - ExceptionResourceNoAccess (403, EU/510, #378 finding 2) gets a hint
//     naming the likely-stale-own-lock cause instead of the generic
//     authorization hint.
//   - ExceptionParameterNotFound naming corrNr (400, #378 finding 1) gets a
//     hint pointing at create_transport/get_transport_requests instead of the
//     generic bad-request hint.
//   - A 400 that mentions a transport gets the more specific transport hint
//     instead of the generic bad-request hint.
//   - Errors that carry no ADT Type or status — plain Go errors such as the
//     ReleaseTransport "… is inactive" failure, or our own English
//     "already exists" messages — are matched on localised text as a last
//     resort.
//
// The last two bullets (and the 405 refinement) match on localised message
// text, so they are language-fragile: they silently miss on non-English
// systems and degrade to the kind-based hint. That tradeoff is accepted for
// conditions with no clean Type (#406, #404).
func matchHint(err error) string {
	kind := adt.ClassifyError(err)
	errText := strings.ToLower(err.Error())

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
	// classifies as the generic ErrorServerError. Gate on the exception Type,
	// not just the message shape: an unrelated 500 (e.g. from
	// release_transport) that happens to name its own request in the message
	// must NOT get a "retry with transport=X" hint that just points back at
	// the same request.
	if kind == adt.ErrorServerError && exceptionTypeOf(err) == exceptionTypeResourceSaveFailure {
		if tr, ok := lockingTransportOf(err); ok {
			owner, _ := lockingOwnerOf(err)
			return transportSaveConflictHint(tr, owner)
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
	// forbiddenHint that a bare 403 would otherwise get.
	if kind == adt.ErrorForbidden && exceptionTypeOf(err) == adt.ExceptionTypeResourceNoAccess {
		return ownAccessConflictHint
	}

	// #378 finding 1: ExceptionParameterNotFound naming corrNr specifically
	// beats both the generic transport-mention check below and the
	// catch-all bad-request hint.
	if kind == adt.ErrorBadRequest && exceptionTypeOf(err) == adt.ExceptionTypeParameterNotFound && strings.Contains(errText, "corrnr") {
		return corrNrMissingHint
	}

	// Transport-specific 400 beats the generic bad-request hint.
	if kind == adt.ErrorBadRequest && strings.Contains(errText, "transport") {
		return transportHint
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
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) {
		return adtErr.LockingTransport()
	}
	return "", false
}

// exceptionTypeOf returns the SAP exception Type of the underlying
// *adt.ADTError, or "" if err wraps none. Used to key on exception Types that
// adt.ClassifyError does not (yet) assign a distinct ErrorKind — see #378.
func exceptionTypeOf(err error) string {
	var adtErr *adt.ADTError
	if errors.As(err, &adtErr) {
		return adtErr.Type
	}
	return ""
}
