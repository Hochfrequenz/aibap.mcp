package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *httpClient) GetSource(ctx context.Context, objectURI string) (*SourceResult, error) {
	resp, err := c.doRead(ctx, objectURI+"/source/main", map[string]string{"Accept": "text/plain"})
	if err != nil {
		return nil, fmt.Errorf("GetSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetSource reading body: %w", err)
	}
	return &SourceResult{Source: string(body), ETag: resp.Header.Get("ETag")}, nil
}

// validIncludes lists the valid class include types.
var validIncludes = map[string]bool{
	"testclasses": true, "definitions": true, "implementations": true, "macros": true,
}

// classIncludePath returns the OO include path for a class include.
// Path format: /sap/bc/adt/oo/classes/{CLASS}/includes/{type}
// Note: NO /source/main suffix — unlike the main class source.
func classIncludePath(objectURI, include string) (string, error) {
	if !validIncludes[include] {
		return "", fmt.Errorf("unknown include type %q (valid: testclasses, definitions, implementations, macros)", include)
	}
	return objectURI + "/includes/" + include, nil
}

func (c *httpClient) GetIncludeSource(ctx context.Context, objectURI, include string) (*SourceResult, error) {
	path, err := classIncludePath(objectURI, include)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "text/plain"})
	if err != nil {
		return nil, fmt.Errorf("GetIncludeSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetIncludeSource reading body: %w", err)
	}
	return &SourceResult{Source: string(body), ETag: resp.Header.Get("ETag")}, nil
}

func (c *httpClient) SetIncludeSource(ctx context.Context, objectURI, include, source, lockHandle, transport, etag string) (string, error) {
	path, err := classIncludePath(objectURI, include)
	if err != nil {
		return "", err
	}
	headers := map[string]string{
		"Content-Type": "text/plain; charset=utf-8",
		"Accept":       "text/plain",
	}
	if etag != "" {
		headers["If-Match"] = etag
	}
	// Include endpoints expect lockHandle as query parameter, not header.
	params := url.Values{}
	if lockHandle != "" {
		params.Set("lockHandle", lockHandle)
	}
	if transport != "" {
		params.Set("corrNr", transport)
	}
	writePath := path
	if len(params) > 0 {
		writePath += "?" + params.Encode()
	}
	resp, err := c.doMutate(ctx, http.MethodPut, writePath,
		strings.NewReader(source),
		headers,
	)
	if err != nil {
		return "", fmt.Errorf("SetIncludeSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}

// CreateTestInclude creates the test classes include for a class that doesn't have one yet.
// Requires a lock on the parent class. Credit: approach from oisee/vibing-steampunk.
func (c *httpClient) CreateTestInclude(ctx context.Context, objectURI, lockHandle, transport string) error {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<class:abapClassInclude xmlns:class="http://www.sap.com/adt/oo/classes"
  xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:name="dummy" class:includeType="testclasses"/>`

	params := url.Values{}
	params.Set("lockHandle", lockHandle)
	if transport != "" {
		params.Set("corrNr", transport)
	}

	path := objectURI + "/includes?" + params.Encode()
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(body),
		map[string]string{"Content-Type": "application/*"},
	)
	if err != nil {
		return fmt.Errorf("CreateTestInclude: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

func (c *httpClient) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	headers := map[string]string{
		"Content-Type": "text/plain; charset=utf-8",
		"Accept":       "text/plain",
		"If-Match":     etag,
	}
	if lockHandle != "" {
		headers["X-SAP-Lock-Handle"] = lockHandle
	}
	path := objectURI + "/source/main"
	if transport != "" {
		path += "?corrNr=" + url.QueryEscape(transport)
	}
	resp, err := c.doMutate(ctx, http.MethodPut, path,
		strings.NewReader(source),
		headers,
	)
	if err != nil {
		return "", fmt.Errorf("SetSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}
