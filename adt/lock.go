package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	// SAP returns: <asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>…</LOCK_HANDLE></DATA></asx:values></asx:abap>
	var lockResp struct {
		XMLName xml.Name `xml:"abap"`
		Values  struct {
			Data struct {
				LockHandle string `xml:"LOCK_HANDLE"`
			} `xml:"DATA"`
		} `xml:"values"`
	}
	if err := xml.Unmarshal(data, &lockResp); err == nil && lockResp.Values.Data.LockHandle != "" {
		return lockResp.Values.Data.LockHandle, nil
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
