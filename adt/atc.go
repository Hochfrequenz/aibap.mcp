package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

func (c *httpClient) GetATCCustomizing(ctx context.Context) (*ATCCustomizingResult, error) {
	resp, err := c.doRead(ctx, "/sap/bc/adt/atc/customizing", map[string]string{
		"Accept": "application/vnd.sap.atc.customizing-v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("GetATCCustomizing: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetATCCustomizing reading body: %w", err)
	}

	var cust adtmodel.ATCCustomizing
	if err := xml.Unmarshal(data, &cust); err != nil {
		return nil, fmt.Errorf("GetATCCustomizing parsing: %w", err)
	}

	result := &ATCCustomizingResult{
		Properties: make(map[string]string, len(cust.Properties)),
	}
	for _, p := range cust.Properties {
		result.Properties[p.Name] = p.Value
		if p.Name == "systemCheckVariant" {
			result.SystemCheckVariant = p.Value
		}
	}
	return result, nil
}

func (c *httpClient) RunATCCheck(ctx context.Context, objectURIs []string) (*ATCResult, error) {
	// Build object references XML.
	var refs strings.Builder
	for _, uri := range objectURIs {
		fmt.Fprintf(&refs, `<adtcore:objectReference adtcore:uri="%s"/>`, uri)
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<atc:run xmlns:atc="http://www.sap.com/adt/atc" `+
		`xmlns:adtcore="http://www.sap.com/adt/core" maximumVerdicts="100">`+
		`<objectSets>`+
		`<objectSet kind="inclusive">`+
		`%s`+
		`</objectSet>`+
		`</objectSets>`+
		`</atc:run>`, refs.String())

	// Step 1: Trigger ATC run.
	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/atc/runs?clientWait=false",
		strings.NewReader(body),
		map[string]string{
			"Content-Type": "application/vnd.sap.atc.run.parameters.v1+xml",
			"Accept":       "application/vnd.sap.atc.run.result.v1+xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("RunATCCheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("RunATCCheck reading run result: %w", err)
	}

	var runResult adtmodel.ATCWorklistRun
	if err := xml.Unmarshal(data, &runResult); err != nil {
		return nil, fmt.Errorf("RunATCCheck parsing run result: %w", err)
	}

	if runResult.WorklistID == "" {
		return nil, fmt.Errorf("RunATCCheck: no worklist ID in response")
	}

	// Step 2: Fetch worklist results.
	wlResp, err := c.doRead(ctx, "/sap/bc/adt/atc/worklists/"+runResult.WorklistID, map[string]string{
		"Accept": "application/atc.worklist.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("RunATCCheck fetching worklist: %w", err)
	}
	defer func() { _ = wlResp.Body.Close() }()
	if err := checkResponse(wlResp); err != nil {
		return nil, err
	}

	wlData, err := io.ReadAll(wlResp.Body)
	if err != nil {
		return nil, fmt.Errorf("RunATCCheck reading worklist: %w", err)
	}

	var worklist adtmodel.ATCWorklist
	if err := xml.Unmarshal(wlData, &worklist); err != nil {
		return nil, fmt.Errorf("RunATCCheck parsing worklist: %w", err)
	}

	result := &ATCResult{WorklistID: worklist.ID}
	for _, obj := range worklist.ObjectSets {
		for _, f := range obj.Findings {
			result.Findings = append(result.Findings, ATCFinding{
				ObjectURI:    obj.URI,
				Priority:     f.Priority,
				CheckID:      f.CheckID,
				CheckTitle:   f.CheckTitle,
				MessageTitle: f.MessageTitle,
				Location:     f.Location,
			})
		}
	}
	return result, nil
}
