package tools

import (
	"fmt"
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
	// ErrorMethodNotAllowed). See mcp-server-abap #406.
	methodNotAllowedHint = "Method not allowed (405) — either the operation is not supported for this resource, or (on ECC) the object already exists. Check with `object_exists` / `search_objects`."
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
)

// hintByKind maps an adt.ErrorKind to the MCP-flavored recovery hint. Kinds
// absent from the map (e.g. adt.ErrorUnknown) get no hint here and fall through
// to the localised-text fallbacks in matchHint.
//
// adt.ErrorInvalidLockHandle maps to lockedHint for now: a stale/invalid lock
// handle still points the user at unlock_object / get_transport_requests, which
// preserves the pre-adt.ClassifyError behavior (a 423 fell through to the
// locked hint via the status code). mcp-server-abap #378 will give it a
// dedicated hint.
var hintByKind = map[adt.ErrorKind]string{
	adt.ErrorLocked:            lockedHint,
	adt.ErrorInvalidLockHandle: lockedHint,
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
// wording by kind, with two consumer-side refinements that adtler's
// protocol-level classification intentionally does not cover:
//
//   - A 400 that mentions a transport gets the more specific transport hint
//     instead of the generic bad-request hint.
//   - Errors that carry no ADT Type or status — plain Go errors such as the
//     ReleaseTransport "… is inactive" failure, or our own English
//     "already exists" messages — are matched on localised text as a last
//     resort. This tier is language-fragile (it silently misses on German
//     systems) and is kept only for conditions with no clean Type (#406).
func matchHint(err error) string {
	kind := adt.ClassifyError(err)
	errText := strings.ToLower(err.Error())

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
