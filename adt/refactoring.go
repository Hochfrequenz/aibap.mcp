package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const refactoringEndpoint = "/sap/bc/adt/refactorings"
const renameRelation = "http://www.sap.com/adt/relations/refactoring/rename"

// RenameResult holds the preview of a rename refactoring.
type RenameResult struct {
	OldName           string
	NewName           string
	AffectedObjects   int
	AffectedLocations int
}

// Rename performs an ABAP rename refactoring: evaluates the symbol at the given
// source position, previews the rename, and executes it. All three steps use the
// same HTTP session for server-side state.
func (c *httpClient) Rename(ctx context.Context, sourceURI string, newName, transport string) (*RenameResult, error) {
	// Step 1: evaluate — get current name and affected locations
	evalXML, err := c.refactoringStep(ctx, "evaluate", map[string]string{
		"rel": renameRelation,
		"uri": sourceURI,
	}, "")
	if err != nil {
		return nil, fmt.Errorf("Rename evaluate: %w", err)
	}

	// Extract old name from evaluate response
	oldName := extractXMLTag(evalXML, "rename:oldName")
	if oldName == "" {
		oldName = extractXMLTag(evalXML, "oldName")
	}
	if oldName == "" {
		return nil, fmt.Errorf("Rename: could not find old name in evaluate response")
	}

	// Set the new name in the evaluate XML
	modifiedXML := strings.Replace(evalXML,
		fmt.Sprintf("<rename:newName>%s</rename:newName>", oldName),
		fmt.Sprintf("<rename:newName>%s</rename:newName>", newName), 1)

	// Step 2: preview — get all text replace deltas with old/new content
	previewXML, err := c.refactoringStep(ctx, "preview", map[string]string{
		"rel": renameRelation,
	}, modifiedXML)
	if err != nil {
		return nil, fmt.Errorf("Rename preview: %w", err)
	}

	// Inject transport if provided
	if transport != "" {
		previewXML = strings.Replace(previewXML,
			"<generic:transport/>",
			fmt.Sprintf("<generic:transport>%s</generic:transport>", transport), 1)
	}

	// Count affected locations for the result
	locations := strings.Count(previewXML, "<generic:textReplaceDelta>")
	objects := strings.Count(previewXML, "<generic:affectedObject ")

	// Step 3: execute — apply the rename
	_, err = c.refactoringStep(ctx, "execute", nil, previewXML)
	if err != nil {
		return nil, fmt.Errorf("Rename execute: %w", err)
	}

	return &RenameResult{
		OldName:           oldName,
		NewName:           newName,
		AffectedObjects:   objects,
		AffectedLocations: locations,
	}, nil
}

func (c *httpClient) refactoringStep(ctx context.Context, step string, extraParams map[string]string, body string) (string, error) {
	params := url.Values{"step": {step}}
	for k, v := range extraParams {
		params.Set(k, v)
	}
	path := refactoringEndpoint + "?" + params.Encode()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	resp, err := c.doMutate(ctx, http.MethodPost, path, bodyReader,
		map[string]string{
			"Content-Type": "application/xml",
			"Accept":       "application/xml",
		})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	return string(data), nil
}
