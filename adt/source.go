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

// classIncludeSuffix maps include names to ABAP include suffixes.
var classIncludeSuffix = map[string]string{
	"testclasses":     "CCAU",
	"definitions":     "CCDEF",
	"implementations": "CCIMP",
	"macros":          "CCMAC",
}

// classIncludePath returns the /programs/includes/ path for a class include.
// Class includes are accessed as ABAP programs with a padded name like
// ZCL_MY_CLASS==========CCAU (30-char class name + suffix).
func classIncludePath(objectURI, include string) (string, error) {
	suffix, ok := classIncludeSuffix[include]
	if !ok {
		return "", fmt.Errorf("unknown include type %q (valid: testclasses, definitions, implementations, macros)", include)
	}
	// Extract class name from URI: /sap/bc/adt/oo/classes/zcl_my_class → ZCL_MY_CLASS
	parts := strings.Split(objectURI, "/")
	className := strings.ToUpper(parts[len(parts)-1])
	// Pad to 30 characters with '='
	padded := className
	for len(padded) < 30 {
		padded += "="
	}
	return "/sap/bc/adt/programs/includes/" + padded + suffix, nil
}

func (c *httpClient) GetIncludeSource(ctx context.Context, objectURI, include string) (*SourceResult, error) {
	path, err := classIncludePath(objectURI, include)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRead(ctx, path+"/source/main", map[string]string{"Accept": "text/plain"})
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
	if lockHandle != "" {
		headers["X-SAP-Lock-Handle"] = lockHandle
	}
	writePath := path + "/source/main"
	if transport != "" {
		writePath += "?corrNr=" + url.QueryEscape(transport)
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
