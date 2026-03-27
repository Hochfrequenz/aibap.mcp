package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func (c *httpClient) NavigateToDefinition(ctx context.Context, sourceURI string) (string, error) {
	path := "/sap/bc/adt/navigation/target?uri=" + url.QueryEscape(sourceURI)
	resp, err := c.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/xml"})
	if err != nil {
		return "", fmt.Errorf("NavigateToDefinition: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("NavigateToDefinition reading body: %w", err)
	}

	var ref struct {
		URI string `xml:"uri,attr"`
	}
	if err := xml.Unmarshal(data, &ref); err != nil {
		return "", fmt.Errorf("NavigateToDefinition parsing: %w", err)
	}
	return ref.URI, nil
}
