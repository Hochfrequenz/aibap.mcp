package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func (c *httpClient) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" `+
		`xmlns:adtcore="http://www.sap.com/adt/core">`+
		`<chkrun:checkObject adtcore:uri="%s" chkrun:version="inactive"/>`+
		`</chkrun:checkObjectList>`, objectURI)

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/checkruns",
		strings.NewReader(body),
		map[string]string{
			"Content-Type": "application/vnd.sap.adt.checkobjects+xml",
			"Accept":       "application/vnd.sap.adt.checkmessages+xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("SyntaxCheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var reports adtxml.CheckRunReports
	xml.Unmarshal(data, &reports) //nolint:errcheck

	var result []SyntaxMessage
	for _, report := range reports.Reports {
		for _, m := range report.Messages {
			line, col := parseMessagePosition(m.URI)
			result = append(result, SyntaxMessage{
				Type:   m.Type,
				Text:   m.ShortText,
				Line:   line,
				Column: col,
			})
		}
	}
	return result, nil
}

const inlineSyntaxCheckCT = "application/vnd.sap.adt.functions.abapsource.syntaxcheck.v1+xml"

// ErrInlineSyntaxCheckNotSupported is returned when the system's ADT discovery
// does not advertise the inline syntax check content type.
var ErrInlineSyntaxCheckNotSupported = fmt.Errorf("inline syntax check not supported by this system")

// InlineSyntaxCheck sends source code for syntax checking without saving it first.
// Uses the ADT POST {objectURI}/source/main?_action=CHECK endpoint with ASX-serialized SourceList.
// This is what Eclipse ADT uses internally — a single HTTP call instead of the 5-step
// create→lock→set→check→delete flow.
//
// Returns ErrInlineSyntaxCheckNotSupported if the system does not advertise
// the required content type in its ADT discovery.
func (c *httpClient) InlineSyntaxCheck(ctx context.Context, objectURI, source string) ([]SyntaxMessage, error) {
	if !c.supportsInlineSyntaxCheck(ctx) {
		return nil, ErrInlineSyntaxCheckNotSupported
	}

	sourceURI := objectURI + "/source/main"
	body := buildSourceListASX(sourceURI, source)

	resp, err := c.doMutate(ctx, http.MethodPost,
		sourceURI+"?_action=CHECK",
		strings.NewReader(body),
		map[string]string{
			"Content-Type": inlineSyntaxCheckCT,
			"Accept":       inlineSyntaxCheckCT,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("InlineSyntaxCheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseASXSyntaxCheckResult(data)
}

// supportsInlineSyntaxCheck checks the ADT discovery cache for the inline
// syntax check content type on any endpoint. If the discovery cache is empty
// (no CSRF fetch yet), it triggers one.
func (c *httpClient) supportsInlineSyntaxCheck(ctx context.Context) bool {
	c.mu.Lock()
	if c.discovery == nil && c.csrfToken == "" {
		_ = c.fetchCSRFToken(ctx) // populates discovery as side effect
	}
	defer c.mu.Unlock()
	for _, accepts := range c.discovery {
		for _, ct := range accepts {
			if ct == inlineSyntaxCheckCT {
				return true
			}
		}
	}
	return false
}

// buildSourceListASX constructs the ASX XML body for inline syntax check.
// Format reverse-engineered from Eclipse ADT's SyntaxCheckService/SourceList classes.
func buildSourceListASX(sourceURI, source string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">`)
	sb.WriteString(`<asx:values>`)
	sb.WriteString(`<DATA>`)
	sb.WriteString(`<IDENTIFIER>`)
	sb.WriteString(xmlEscapeString(sourceURI))
	sb.WriteString(`</IDENTIFIER>`)
	sb.WriteString(`<SOURCE>`)
	for _, line := range strings.Split(source, "\n") {
		sb.WriteString(`<item>`)
		sb.WriteString(xmlEscapeString(strings.TrimRight(line, "\r")))
		sb.WriteString(`</item>`)
	}
	sb.WriteString(`</SOURCE>`)
	sb.WriteString(`</DATA>`)
	sb.WriteString(`</asx:values>`)
	sb.WriteString(`</asx:abap>`)
	return sb.String()
}

// xmlEscapeString escapes XML special characters.
func xmlEscapeString(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

// parseASXSyntaxCheckResult parses the ASX response from inline syntax check.
func parseASXSyntaxCheckResult(data []byte) ([]SyntaxMessage, error) {
	// The response is ASX XML with ERRORS_WITH_URI and WARNINGS_WITH_URI sections.
	// Each contains items with LINE, COL, MESSAGE, URI fields.
	type asxItem struct {
		Line    int    `xml:"LINE"`
		Column  int    `xml:"COL"`
		Message string `xml:"MESSAGE"`
		URI     string `xml:"URI"`
	}
	type asxResult struct {
		XMLName  xml.Name  `xml:"abap"`
		Errors   []asxItem `xml:"values>DATA>ERRORS_WITH_URI>item"`
		Warnings []asxItem `xml:"values>DATA>WARNINGS_WITH_URI>item"`
	}

	var result asxResult
	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing syntax check result: %w", err)
	}

	var msgs []SyntaxMessage
	for _, e := range result.Errors {
		msgs = append(msgs, SyntaxMessage{Type: "E", Text: e.Message, Line: e.Line, Column: e.Column})
	}
	for _, w := range result.Warnings {
		msgs = append(msgs, SyntaxMessage{Type: "W", Text: w.Message, Line: w.Line, Column: w.Column})
	}
	return msgs, nil
}

// parseMessagePosition extracts line and column from a checkMessage URI fragment.
// Format: ".../source/main#start=42,5" → line=42, col=5
func parseMessagePosition(uri string) (int, int) {
	idx := strings.Index(uri, "#start=")
	if idx < 0 {
		return 0, 0
	}
	parts := strings.SplitN(uri[idx+7:], ",", 2)
	line, _ := strconv.Atoi(parts[0])
	col := 0
	if len(parts) == 2 {
		col, _ = strconv.Atoi(parts[1])
	}
	return line, col
}
