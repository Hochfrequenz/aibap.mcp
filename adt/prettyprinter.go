package adt

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func (c *httpClient) PrettyPrint(ctx context.Context, source string) (string, error) {
	resp, err := c.doMutate(ctx, "POST",
		"/sap/bc/adt/abapsource/prettyprinter",
		strings.NewReader(source),
		map[string]string{"Content-Type": "text/plain; charset=utf-8"},
	)
	if err != nil {
		return "", fmt.Errorf("PrettyPrint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("PrettyPrint reading body: %w", err)
	}
	return string(data), nil
}
