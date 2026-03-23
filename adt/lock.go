package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

func (c *httpClient) LockObject(ctx context.Context, objectURI string) (string, error) {
	resp, err := c.doMutate(ctx, http.MethodPost,
		objectURI+"?_action=LOCK&accessMode=MODIFY",
		nil,
		map[string]string{"Accept": "application/xml"},
	)
	if err != nil {
		return "", fmt.Errorf("LockObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("LockObject reading body: %w", err)
	}
	return string(data), nil
}

func (c *httpClient) UnlockObject(ctx context.Context, objectURI string) error {
	resp, err := c.doMutate(ctx, http.MethodPost,
		objectURI+"?_action=UNLOCK",
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("UnlockObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}
