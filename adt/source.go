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

func (c *httpClient) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	headers := map[string]string{
		"Content-Type": "text/plain; charset=utf-8",
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
