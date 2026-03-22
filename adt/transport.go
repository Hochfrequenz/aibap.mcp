package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type xmlTransportRoot struct {
	XMLName           xml.Name              `xml:"root"`
	WorkbenchRequests []xmlTransportRequest `xml:"workbenchRequests>workbenchRequest"`
}

type xmlTransportRequest struct {
	Number      string `xml:"number,attr"`
	Owner       string `xml:"owner,attr"`
	Description string `xml:"shortDescription,attr"`
	Status      string `xml:"status,attr"`
}

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

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportRequests: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var root xmlTransportRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("GetTransportRequests parsing: %w", err)
	}

	result := make([]TransportRequest, len(root.WorkbenchRequests))
	for i, r := range root.WorkbenchRequests {
		result[i] = TransportRequest{
			Number:      r.Number,
			Owner:       r.Owner,
			Description: r.Description,
			Status:      r.Status,
		}
	}
	return result, nil
}

type xmlTransportComponent struct {
	XMLName   xml.Name `xml:"adtcore:objectReference"`
	NSCore    string   `xml:"xmlns:adtcore,attr"`
	ObjectURI string   `xml:"adtcore:uri,attr"`
}

func (c *httpClient) AddToTransport(ctx context.Context, objectURI, transport string) error {
	body, err := xml.Marshal(xmlTransportComponent{
		NSCore:    "http://www.sap.com/adt/core",
		ObjectURI: objectURI,
	})
	if err != nil {
		return fmt.Errorf("marshal transport component: %w", err)
	}

	path := "/sap/bc/adt/cts/transportrequests/" + transport + "/abaptransportcomponents"
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": "application/xml"},
	)
	if err != nil {
		return fmt.Errorf("AddToTransport: %w", err)
	}
	defer resp.Body.Close()
	return checkResponse(resp)
}
