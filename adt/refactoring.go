package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const refactoringEndpoint = "/sap/bc/adt/refactorings"
const renameRelation = "http://www.sap.com/adt/relations/refactoring/rename"

// RenameResult holds the result of a rename refactoring.
type RenameResult struct {
	OldName         string                 `json:"old_name"`
	NewName         string                 `json:"new_name"`
	AffectedObjects []RenameAffectedObject `json:"affected_objects"`
}

// RenameAffectedObject describes an object affected by the rename.
type RenameAffectedObject struct {
	URI       string           `json:"uri"`
	Type      string           `json:"type"`
	Name      string           `json:"name"`
	Locations []RenameLocation `json:"locations"`
}

// RenameLocation is a single text replacement within an affected object.
type RenameLocation struct {
	Range      string `json:"range"` // e.g. "#start=2,6;end=2,13"
	ContentOld string `json:"content_old"`
	ContentNew string `json:"content_new"`
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

	// Parse affected objects from preview for the result
	affected := parseAffectedObjects(previewXML)

	// Inject transport if provided
	if transport != "" {
		previewXML = strings.Replace(previewXML,
			"<generic:transport/>",
			fmt.Sprintf("<generic:transport>%s</generic:transport>", transport), 1)
	}

	// Step 3: execute — apply the rename
	_, err = c.refactoringStep(ctx, "execute", nil, previewXML)
	if err != nil {
		return nil, fmt.Errorf("Rename execute: %w", err)
	}

	return &RenameResult{
		OldName:         oldName,
		NewName:         newName,
		AffectedObjects: affected,
	}, nil
}

func parseAffectedObjects(previewXML string) []RenameAffectedObject {
	var root struct {
		Objects []struct {
			URI    string `xml:"uri,attr"`
			Type   string `xml:"type,attr"`
			Name   string `xml:"name,attr"`
			Deltas []struct {
				Range      string `xml:"rangeFragment"`
				ContentOld string `xml:"contentOld"`
				ContentNew string `xml:"contentNew"`
			} `xml:"textReplaceDeltas>textReplaceDelta"`
		} `xml:"affectedObjects>affectedObject"`
	}
	_ = xml.Unmarshal([]byte(previewXML), &root)

	var result []RenameAffectedObject
	for _, obj := range root.Objects {
		ao := RenameAffectedObject{
			URI:  obj.URI,
			Type: obj.Type,
			Name: obj.Name,
		}
		for _, d := range obj.Deltas {
			ao.Locations = append(ao.Locations, RenameLocation{
				Range:      d.Range,
				ContentOld: d.ContentOld,
				ContentNew: d.ContentNew,
			})
		}
		result = append(result, ao)
	}
	return result
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
