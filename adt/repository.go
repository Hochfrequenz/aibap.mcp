package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

func (c *httpClient) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("parent_type", "DEVC/K")
	params.Set("parent_name", packageName)
	path := "/sap/bc/adt/repository/nodestructure?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": contentTypeXML})
	if err != nil {
		return nil, fmt.Errorf("BrowsePackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseObjectReferences(data)
}

func (c *httpClient) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	resp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": contentTypeXML})
	if err != nil {
		return nil, fmt.Errorf("GetObjectInfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var ref adtmodel.ObjectReference
	if err := xml.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("GetObjectInfo parsing: %w", err)
	}

	info := ObjectInfo{
		URI: ref.URI, Type: ref.Type, Name: ref.Name,
		Description: ref.Description, PackageName: ref.PackageName,
	}
	return &info, nil
}
