package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

// ShortDumpHeader contains the key fields of a dump (for list view).
type ShortDumpHeader struct {
	RuntimeError string `json:"runtime_error"`
	Program      string `json:"program"`
	User         string `json:"user"`
	Timestamp    string `json:"timestamp"` // ISO 8601
}

// ShortDump contains full parsed dump details.
type ShortDump struct {
	ShortDumpHeader
	SourceLink    string `json:"source_link,omitempty"`
	Header        string `json:"header"`
	WhatHappened  string `json:"what_happened"`
	ErrorAnalysis string `json:"error_analysis"`
	AbortLocation string `json:"abort_location"`
	CallStack     string `json:"call_stack"`
}

func (c *httpClient) fetchDumpFeed(ctx context.Context, from, to string) ([]byte, error) {
	params := url.Values{}
	if from != "" {
		params.Set("from", from)
	}
	if to != "" {
		params.Set("to", to)
	}
	path := "/sap/bc/adt/runtime/dumps"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRead(ctx, path, map[string]string{
		"Accept": "application/atom+xml;type=feed",
	})
	if err != nil {
		return nil, fmt.Errorf("fetchDumpFeed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// ListShortDumps returns dump headers only (lightweight, for listing).
func (c *httpClient) ListShortDumps(ctx context.Context, from, to, user string) ([]ShortDumpHeader, error) {
	data, err := c.fetchDumpFeed(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return parseShortDumpHeaders(data, user)
}

// GetShortDumps returns full dump details including parsed HTML sections.
func (c *httpClient) GetShortDumps(ctx context.Context, from, to, user string) ([]ShortDump, error) {
	data, err := c.fetchDumpFeed(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return parseShortDumpFeed(data, user)
}

func parseShortDumpHeaders(data []byte, filterUser string) ([]ShortDumpHeader, error) {
	entries, err := parseDumpEntries(data)
	if err != nil {
		return nil, err
	}
	var headers []ShortDumpHeader
	for _, e := range entries {
		if filterUser != "" && !strings.EqualFold(e.Author.Name, filterUser) {
			continue
		}
		h := ShortDumpHeader{
			User:      e.Author.Name,
			Timestamp: e.Published,
		}
		for _, cat := range e.Categories {
			if strings.Contains(cat.Label, "Laufzeitfehler") || strings.Contains(cat.Label, "Runtime Error") {
				h.RuntimeError = cat.Term
			}
			if strings.Contains(cat.Label, "Programm") || strings.Contains(cat.Label, "Program") {
				h.Program = cat.Term
			}
		}
		headers = append(headers, h)
	}
	return headers, nil
}

type dumpEntry struct {
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Categories []struct {
		Term  string `xml:"term,attr"`
		Label string `xml:"label,attr"`
	} `xml:"category"`
	Published string `xml:"published"`
	Summary   struct {
		Text string `xml:",chardata"`
	} `xml:"summary"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
}

func parseDumpEntries(data []byte) ([]dumpEntry, error) {
	var feed struct {
		Entries []dumpEntry `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing dump feed: %w", err)
	}
	return feed.Entries, nil
}

func parseShortDumpFeed(data []byte, filterUser string) ([]ShortDump, error) {
	entries, err := parseDumpEntries(data)
	if err != nil {
		return nil, err
	}

	var dumps []ShortDump
	for _, e := range entries {
		if filterUser != "" && !strings.EqualFold(e.Author.Name, filterUser) {
			continue
		}

		d := ShortDump{
			ShortDumpHeader: ShortDumpHeader{
				User:      e.Author.Name,
				Timestamp: e.Published,
			},
		}

		for _, cat := range e.Categories {
			if strings.Contains(cat.Label, "Laufzeitfehler") || strings.Contains(cat.Label, "Runtime Error") {
				d.RuntimeError = cat.Term
			}
			if strings.Contains(cat.Label, "Programm") || strings.Contains(cat.Label, "Program") {
				d.Program = cat.Term
			}
		}

		// Extract source link from atom:link with ADT URI
		for _, l := range e.Links {
			if strings.Contains(l.Href, "/sap/bc/adt/") && !strings.Contains(l.Href, "runtime/dumps") {
				d.SourceLink = l.Href
			}
		}

		// Parse HTML summary into sections
		if e.Summary.Text != "" {
			d.Header = extractSection(e.Summary.Text, "HEADER")
			d.WhatHappened = extractSection(e.Summary.Text, "WHATHAPPENED")
			d.ErrorAnalysis = extractSection(e.Summary.Text, "ERROR")
			d.AbortLocation = extractSection(e.Summary.Text, "TERMINATION")
			d.CallStack = extractSection(e.Summary.Text, "STACK")

			// Extract source link from HTML if not found in atom:link
			if d.SourceLink == "" {
				if m := reSourceLink.FindStringSubmatch(e.Summary.Text); m != nil {
					d.SourceLink = m[1]
				}
			}
		}

		dumps = append(dumps, d)
	}
	return dumps, nil
}

var (
	reSectionSplit = regexp.MustCompile(`<h4 id="(\w+)">`)
	reHTMLTag      = regexp.MustCompile(`<[^>]*>`)
	reMultiSpace   = regexp.MustCompile(`[ \t]+`)
	reMultiNewline = regexp.MustCompile(`\n{3,}`)
	reSourceLink   = regexp.MustCompile(`href="(adt://[^"]*(?:classes|programs|includes|interfaces)[^"]*)"`)
)

// extractSection extracts the text content of a named HTML section.
func extractSection(html, sectionID string) string {
	// Split by h4 headers
	parts := reSectionSplit.Split(html, -1)
	ids := reSectionSplit.FindAllStringSubmatch(html, -1)

	for i, match := range ids {
		if match[1] == sectionID && i+1 < len(parts) {
			return htmlToText(parts[i+1])
		}
	}
	return ""
}

// htmlToText converts HTML to plain text.
func htmlToText(html string) string {
	// Replace block-level tags with newlines before stripping
	s := strings.ReplaceAll(html, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "</tr>", "\n")
	s = strings.ReplaceAll(s, "</td>", " | ")
	s = strings.ReplaceAll(s, "</th>", " | ")

	// Strip HTML tags
	s = reHTMLTag.ReplaceAllString(s, "")

	// Decode HTML entities after tag stripping
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")

	// Clean up whitespace
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = reMultiNewline.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	return s
}
