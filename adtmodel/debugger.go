package adtmodel

import "encoding/xml"

// XML types for ABAP debugger ADT REST endpoints.

// BreakpointsRequest is the XML body for POST /sap/bc/adt/debugger/breakpoints.
// Verified: 2026-03-24 against hfq.sap.msp.local:8100.
// Derived from Simple Transformation TPDA_ADT_BREAKPOINTS_REQUEST.
type BreakpointsRequest struct {
	XMLName       xml.Name            `xml:"dbg:breakpoints"`
	NSDebug       string              `xml:"xmlns:dbg,attr"`
	NSCore        string              `xml:"xmlns:adtcore,attr"`
	Scope         string              `xml:"scope,attr,omitempty"`
	DebuggingMode string              `xml:"debuggingMode,attr,omitempty"`
	RequestUser   string              `xml:"requestUser,attr,omitempty"`
	TerminalID    string              `xml:"terminalId,attr,omitempty"`
	IdeID         string              `xml:"ideId,attr,omitempty"`
	Breakpoints   []BreakpointRequest `xml:"breakpoint"`
}

// BreakpointRequest is a single breakpoint in a set-breakpoint request.
type BreakpointRequest struct {
	Kind string `xml:"kind,attr"`
	URI  string `xml:"uri,attr"`  // adtcore:uri with #start=line,col
	Type string `xml:"type,attr"` // adtcore:type e.g. PROG/P
	Name string `xml:"name,attr"` // adtcore:name e.g. ZREPORT
}

// BreakpointsResponse is the XML response from POST /sap/bc/adt/debugger/breakpoints.
type BreakpointsResponse struct {
	XMLName     xml.Name             `xml:"breakpoints"`
	Breakpoints []BreakpointResponse `xml:"breakpoint"`
}

// BreakpointResponse is a single breakpoint in a response.
type BreakpointResponse struct {
	Kind           string `xml:"kind,attr"`
	ID             string `xml:"id,attr"`
	ErrorMessage   string `xml:"errorMessage,attr"`
	URI            string `xml:"uri,attr"`
	Type           string `xml:"type,attr"`
	Name           string `xml:"name,attr"`
	NonAbapFlavour string `xml:"nonAbapFlavour,attr"`
}
