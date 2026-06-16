package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// SAP exception Type identifiers from <exc:exception><type id="…"/>. These
// are the GET_TYPE / co_type constants of the CX_ADT_RES_* classes, read
// directly from the live ABAP source on both S4U and HFQ (issue #406). They
// are stable across SAP releases and locales — far safer to match against
// than the localised <message> text, which differs by logon language (S4U
// answers in English, HFQ in German). adtler exposes a few as
// adt.ExceptionType* constants; the rest are declared here per adtler's
// "compare against bare strings for IDs not listed" guidance.
const (
	excTypeAlreadyExists       = "ExceptionResourceAlreadyExists" // 400 on S4U, 405 on HFQ
	excTypeLockConflict        = "ExceptionResourceLockConflict"  // 409, S4U only
	excTypeInvalidEtag         = "ExceptionResourceInvalidEtag"   // 412
	excTypeNotAcceptable       = "ExceptionResourceNotAcceptable" // 406
	excTypeUnsupportedMedia    = "ExceptionUnsupportedMediaType"  // 415
	excTypeUnprocessableEntity = "ExceptionUnprocessableEntity"   // 422
	excTypeNotAllowed          = "ExceptionNotAllowed"            // 405, S4U only (absent on HFQ)
)

// etagMismatchHint is shared by the ExceptionResourceInvalidEtag and
// ExceptionPreconditionFailed Types and the 412 status-code fallback.
const etagMismatchHint = "ETag mismatch — the object was modified since your lock was acquired. Re-lock the object and retry the write."

// lockConflictHint is shared by the ExceptionResourceLockConflict Type and
// the 409 status-code fallback. 409 is a save/lock conflict, not an
// "already exists" condition — the latter has its own Type (and is a 400 on
// S4U / 405 on HFQ).
const lockConflictHint = "Save conflict — another process holds a conflicting lock. Use `get_transport_requests` to check the locking transport, or `unlock_object` if the lock is stale."

// alreadyExistsHint is shared by the ExceptionResourceAlreadyExists Type
// (a 400 on S4U, a 405 on HFQ) and the English plain-text fallback.
const alreadyExistsHint = "Object already exists. Use `search_objects` to find it, or choose a different name."

type hintRule struct {
	excType     string // "" = match any exception Type; exact, case-insensitive
	statusCode  int    // 0 = match any status code
	textPattern string // "" = match any text; checked case-insensitive
	hint        string
}

// hintRules is evaluated top-to-bottom, first match wins, in three tiers:
//
//	Tier 1 (excType)     — the SAP-stable, language- and status-code-independent
//	                       exception identifier. Preferred for everything that
//	                       carries a modern <exc:exception> envelope.
//	Tier 2 (statusCode)  — fallback for errors with no Type (legacy
//	                       <ExceptionText> bodies, HTML pages, plain text).
//	                       Status codes are language-independent.
//	Tier 3 (textPattern) — last resort, for conditions that carry no clean
//	                       Type. NOTE: these match localised text and so are
//	                       language-fragile (they silently miss on German
//	                       systems). Kept only for the activation case, which
//	                       has no dedicated Type. See issue #406.
var hintRules = []hintRule{
	// Tier 1 — by exception Type.
	{excType: excTypeLockConflict, hint: lockConflictHint},
	{excType: excTypeAlreadyExists, hint: alreadyExistsHint},
	{excType: excTypeInvalidEtag, hint: etagMismatchHint},
	{excType: adt.ExceptionTypePreconditionFailed, hint: etagMismatchHint},
	{excType: excTypeNotAcceptable, hint: "Content negotiation failed (406) — the server cannot produce the requested Accept type. Check the Accept header, or try the other system's API version."},
	{excType: excTypeUnsupportedMedia, hint: "Unsupported media type (415) — the request Content-Type is not accepted. Check the Content-Type header."},
	{excType: excTypeUnprocessableEntity, hint: "Request rejected due to semantic errors (422) — check that all required fields and parameter values are valid."},
	{excType: excTypeNotAllowed, hint: "Method not allowed (405) — this operation is not supported for this resource."},

	// Tier 2 — by status code (Type-free fallbacks).
	{statusCode: 423, hint: "Object is locked. Use `unlock_object` if it's your own lock, or `get_transport_requests` to find the locking transport."},
	{statusCode: 404, hint: "Object not found. Check the URI spelling or use `search_objects` to find it."},
	{statusCode: 403, hint: "Authorization error. Check that the ADT user has the required S_DEVELOP authorizations."},
	{statusCode: 412, hint: etagMismatchHint},
	{statusCode: 409, hint: lockConflictHint},
	{statusCode: 400, textPattern: "transport", hint: "A transport request may be required. Use `create_transport` or `get_transport_requests` to find one."},
	{statusCode: 400, hint: "Bad request — the server rejected the request. Check the syntax, required parameters, or the CSRF token."},
	{statusCode: 500, hint: "SAP server error. Retry once — if it persists, check SM21 (system log) or ST22 (short dumps)."},

	// Tier 3 — by localised text (language-fragile, last resort). These
	// only reliably fire for our own English-language plain errors;
	// localised SAP messages are handled by the Tier-1 Type rules above.
	{textPattern: "already exists", hint: alreadyExistsHint},
	{textPattern: "inactive", hint: "Activation failed — dependent objects may be inactive. Use `activate_objects` with all dependencies."},
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

func matchHint(err error) string {
	var adtErr *adt.ADTError
	statusCode := 0
	excType := ""
	if errors.As(err, &adtErr) {
		statusCode = adtErr.StatusCode
		excType = adtErr.Type
	}
	errText := strings.ToLower(err.Error())

	for _, rule := range hintRules {
		if rule.excType != "" && !strings.EqualFold(rule.excType, excType) {
			continue
		}
		if rule.statusCode != 0 && rule.statusCode != statusCode {
			continue
		}
		if rule.textPattern != "" && !strings.Contains(errText, strings.ToLower(rule.textPattern)) {
			continue
		}
		return rule.hint
	}
	return ""
}
