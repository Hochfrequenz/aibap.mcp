package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

func (c *httpClient) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	params := url.Values{}
	if user != "" {
		params.Set("user", user)
	}
	if status != "" {
		params.Set("status", status)
	}
	path := "/sap/bc/adt/cts/transportrequests"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/vnd.sap.adt.transportorganizertree.v1+xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportRequests: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var root adtmodel.TransportRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("GetTransportRequests parsing: %w", err)
	}

	result := make([]TransportRequest, len(root.WorkbenchRequests))
	for i, r := range root.WorkbenchRequests {
		result[i] = TransportRequest{
			Number: r.Number, Owner: r.Owner,
			Description: r.Description, Status: r.Status,
		}
	}
	return result, nil
}

func (c *httpClient) AddToTransport(ctx context.Context, objectURI, transport string) error {
	body, err := xml.Marshal(adtmodel.TransportComponent{
		NSCore:    nsADTCore,
		ObjectURI: objectURI,
	})
	if err != nil {
		return fmt.Errorf("marshal transport component: %w", err)
	}

	path := "/sap/bc/adt/cts/transportrequests/" + transport + "/abaptransportcomponents"
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": contentTypeXML},
	)
	if err != nil {
		return fmt.Errorf("AddToTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}
