package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

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

// BatchSyntaxCheck runs syntax checks on multiple objects concurrently.
// Workers controls parallelism (clamped to 1–20).
func (c *httpClient) BatchSyntaxCheck(ctx context.Context, objectURIs []string, workers int) []ObjectSyntaxResult {
	if workers < 1 {
		workers = 10
	}
	if workers > 20 {
		workers = 20
	}

	results := make([]ObjectSyntaxResult, len(objectURIs))

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, uri := range objectURIs {
		if ctx.Err() != nil {
			results[i] = ObjectSyntaxResult{ObjectURI: uri, Error: ctx.Err().Error()}
			continue
		}
		wg.Add(1)
		go func(idx int, objectURI string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			msgs, err := c.SyntaxCheck(ctx, objectURI)
			if err != nil {
				results[idx] = ObjectSyntaxResult{ObjectURI: objectURI, Error: err.Error()}
				return
			}
			results[idx] = ObjectSyntaxResult{ObjectURI: objectURI, Messages: msgs}
		}(i, uri)
	}
	wg.Wait()
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
