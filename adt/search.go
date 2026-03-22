package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
)

type xmlObjectReferences struct {
	XMLName    xml.Name             `xml:"objectReferences"`
	References []xmlObjectReference `xml:"objectReference"`
}

type xmlObjectReference struct {
	URI         string `xml:"uri,attr"`
	Type        string `xml:"type,attr"`
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	PackageName string `xml:"packageName,attr"`
}

func parseObjectReferences(data []byte) ([]ObjectInfo, error) {
	var refs xmlObjectReferences
	if err := xml.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("parsing object references: %w", err)
	}
	result := make([]ObjectInfo, len(refs.References))
	for i, r := range refs.References {
		result[i] = ObjectInfo{
			URI:         r.URI,
			Type:        r.Type,
			Name:        r.Name,
			Description: r.Description,
			PackageName: r.PackageName,
		}
	}
	return result, nil
}

func (c *httpClient) SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("objectName", query)
	if objectType != "" {
		params.Set("objectType", objectType)
	}
	if maxResults > 0 {
		params.Set("maxResults", strconv.Itoa(maxResults))
	}
	path := "/sap/bc/adt/repository/informationsystem/search?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("SearchObjects: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}

func (c *httpClient) WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("adtObjectUri", objectURI)
	path := "/sap/bc/adt/repository/informationsystem/usageReferences?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("WhereUsed: %w", err)
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}
