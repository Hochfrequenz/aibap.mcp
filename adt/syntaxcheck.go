package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type xmlCheckRunReports struct {
	XMLName xml.Name            `xml:"checkRunReports"`
	Reports []xmlCheckRunReport `xml:"checkReport"`
}

type xmlCheckRunReport struct {
	Reporter   string            `xml:"reporter,attr"`
	TriggerURI string            `xml:"triggeringUri,attr"`
	Status     string            `xml:"status,attr"`
	StatusText string            `xml:"statusText,attr"`
	Messages   []xmlCheckMessage `xml:"checkMessageList>checkMessage"`
}

type xmlCheckMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText string `xml:"shortText,attr"`
}

func (c *httpClient) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" `+
		`xmlns:adtcore="http://www.sap.com/adt/core">`+
		`<chkrun:checkObject adtcore:uri="%s" chkrun:version="active"/>`+
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
	var reports xmlCheckRunReports
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
