package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func parseObjectReferences(data []byte) ([]ObjectInfo, error) {
	var refs adtxml.ObjectReferences
	if err := xml.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("parsing object references: %w", err)
	}
	result := make([]ObjectInfo, len(refs.References))
	for i, r := range refs.References {
		result[i] = ObjectInfo{
			URI: r.URI, Type: r.Type, Name: r.Name,
			Description: r.Description, PackageName: r.PackageName,
		}
	}
	return result, nil
}

func (c *httpClient) SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("operation", "quickSearch")
	params.Set("query", query)
	if objectType != "" {
		params.Set("objectType", objectType)
	}
	if maxResults > 0 {
		params.Set("maxResults", strconv.Itoa(maxResults))
	}
	path := "/sap/bc/adt/repository/informationsystem/search?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": contentTypeXML})
	if err != nil {
		return nil, fmt.Errorf("SearchObjects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}

func (c *httpClient) WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("uri", objectURI)
	path := "/sap/bc/adt/repository/informationsystem/usageReferences?" + params.Encode()

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<usageReferenceRequest xmlns="http://www.sap.com/adt/ris/usageReferences" xmlns:adtcore="%s">`+
		`<adtcore:objectReference adtcore:uri="%s"/>`+
		`</usageReferenceRequest>`, nsADTCore, objectURI)

	resp, err := c.doMutate(ctx, "POST", path,
		strings.NewReader(body),
		map[string]string{
			"Content-Type": "application/vnd.sap.adt.repository.usagereferences.request.v1+xml",
			"Accept":       "application/vnd.sap.adt.repository.usagereferences.result.v1+xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("WhereUsed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseUsageReferences(data)
}

func parseUsageReferences(data []byte) ([]ObjectInfo, error) {
	var result adtxml.UsageReferenceResult
	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing usage references: %w", err)
	}
	refs := make([]ObjectInfo, len(result.References))
	for i, r := range result.References {
		refs[i] = ObjectInfo{
			URI:         r.URI,
			Type:        r.ADTObject.Type,
			Name:        r.ADTObject.Name,
			Description: r.ADTObject.Description,
			PackageName: r.ADTObject.PackageRef.Name,
		}
	}
	return refs, nil
}
