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

// ObjectSyntaxResult holds the syntax check result for a single object.
type ObjectSyntaxResult struct {
	ObjectURI string          `json:"object_uri"`
	Messages  []SyntaxMessage `json:"messages"`
	Error     string          `json:"error,omitempty"`
}

// batchCheckChunkSize is the maximum number of objects per checkruns request.
// SAP endpoints have undocumented request size limits; 10 is a safe default.
const batchCheckChunkSize = 10

// BatchSyntaxCheck runs syntax checks on multiple objects using the native
// batch capability of /sap/bc/adt/checkruns. Objects are sent in chunks
// of batchCheckChunkSize to stay within SAP request size limits.
// Results are correlated back to objects via the report's triggeringUri.
func (c *httpClient) BatchSyntaxCheck(ctx context.Context, objectURIs []string) []ObjectSyntaxResult {
	results := make([]ObjectSyntaxResult, len(objectURIs))
	for start := 0; start < len(objectURIs); start += batchCheckChunkSize {
		end := start + batchCheckChunkSize
		if end > len(objectURIs) {
			end = len(objectURIs)
		}
		chunk := objectURIs[start:end]
		chunkResults := c.batchSyntaxCheckChunk(ctx, chunk)
		copy(results[start:], chunkResults)
	}
	return results
}

// batchSyntaxCheckChunk runs a single batched syntax check request for a chunk of objects.
func (c *httpClient) batchSyntaxCheckChunk(ctx context.Context, objectURIs []string) []ObjectSyntaxResult {
	// Build XML with all objects in a single checkObjectList.
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" `)
	sb.WriteString(`xmlns:adtcore="http://www.sap.com/adt/core">`)
	for _, uri := range objectURIs {
		sb.WriteString(fmt.Sprintf(`<chkrun:checkObject adtcore:uri="%s" chkrun:version="inactive"/>`, uri))
	}
	sb.WriteString(`</chkrun:checkObjectList>`)

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/checkruns",
		strings.NewReader(sb.String()),
		map[string]string{
			"Content-Type": "application/vnd.sap.adt.checkobjects+xml",
			"Accept":       "application/vnd.sap.adt.checkmessages+xml",
		},
	)
	if err != nil {
		// On HTTP-level failure, return the error for all objects.
		results := make([]ObjectSyntaxResult, len(objectURIs))
		for i, uri := range objectURIs {
			results[i] = ObjectSyntaxResult{ObjectURI: uri, Error: fmt.Sprintf("BatchSyntaxCheck: %s", err)}
		}
		return results
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		results := make([]ObjectSyntaxResult, len(objectURIs))
		for i, uri := range objectURIs {
			results[i] = ObjectSyntaxResult{ObjectURI: uri, Error: err.Error()}
		}
		return results
	}

	data, _ := io.ReadAll(resp.Body)
	var reports adtxml.CheckRunReports
	xml.Unmarshal(data, &reports) //nolint:errcheck

	// Index reports by triggeringUri for correlation.
	reportsByURI := make(map[string]*adtxml.CheckRunReport, len(reports.Reports))
	for i := range reports.Reports {
		reportsByURI[reports.Reports[i].TriggerURI] = &reports.Reports[i]
	}

	results := make([]ObjectSyntaxResult, len(objectURIs))
	for i, uri := range objectURIs {
		results[i] = ObjectSyntaxResult{ObjectURI: uri}
		report, ok := reportsByURI[uri]
		if !ok {
			continue // no report = no messages = clean
		}
		for _, m := range report.Messages {
			line, col := parseMessagePosition(m.URI)
			results[i].Messages = append(results[i].Messages, SyntaxMessage{
				Type:   m.Type,
				Text:   m.ShortText,
				Line:   line,
				Column: col,
			})
		}
	}
	return results
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
