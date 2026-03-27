package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func (c *httpClient) GetABAPDoc(ctx context.Context, keyword string) (string, error) {
	params := url.Values{}
	if keyword != "" {
		params.Set("keyword", strings.ToUpper(keyword))
		params.Set("context", "ABAP_KEYWORD")
	}
	path := "/sap/bc/adt/docu/abap/langu"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/vnd.sap.adt.docu.v1+html"})
	if err != nil {
		return "", fmt.Errorf("GetABAPDoc: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("GetABAPDoc reading body: %w", err)
	}

	// Strip HTML tags for clean text output.
	text := htmlTagRe.ReplaceAllString(string(data), " ")
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text), nil
}
