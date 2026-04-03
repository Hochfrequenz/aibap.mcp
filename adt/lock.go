package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func (c *httpClient) LockObject(ctx context.Context, objectURI string) (string, error) {
	resp, err := c.doMutate(ctx, http.MethodPost,
		objectURI+"?_action=LOCK&accessMode=MODIFY",
		nil,
		map[string]string{"Accept": "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.lock.result"},
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
	// SAP returns asx:abap envelope with LockData
	lockData, unmarshalErr := adtxml.UnmarshalASXData[adtxml.LockData](data)
	if unmarshalErr == nil && lockData.LockHandle != "" {
		return lockData.LockHandle, nil
	}
	// Fallback: if response is not XML, treat entire body as handle
	handle := strings.TrimSpace(string(data))
	if handle == "" {
		return "", fmt.Errorf("LockObject: empty lock handle in response")
	}
	return handle, nil
}

func (c *httpClient) UnlockObject(ctx context.Context, objectURI, lockHandle string) error {
	resp, err := c.doMutate(ctx, http.MethodPost,
		objectURI+"?_action=UNLOCK&lockHandle="+lockHandle,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("UnlockObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}
