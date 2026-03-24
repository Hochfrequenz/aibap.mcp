package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func (c *httpClient) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	params := url.Values{}
	params.Set("parent_type", "DEVC/K")
	params.Set("parent_name", packageName)
	path := "/sap/bc/adt/repository/nodestructure?" + params.Encode()

	resp, err := c.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{
			"Accept":       "application/vnd.sap.as+xml",
			"Content-Type": contentTypeXML,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("BrowsePackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseNodeStructure(data)
}

type xmlNodeStructure struct {
	XMLName xml.Name               `xml:"abap"`
	Nodes   []xmlNodeStructureNode `xml:"values>DATA>TREE_CONTENT>SEU_ADT_REPOSITORY_OBJ_NODE"`
}

type xmlNodeStructureNode struct {
	ObjectType  string `xml:"OBJECT_TYPE"`
	ObjectName  string `xml:"OBJECT_NAME"`
	ObjectURI   string `xml:"OBJECT_URI"`
	Description string `xml:"DESCRIPTION"`
}

func parseNodeStructure(data []byte) ([]ObjectInfo, error) {
	var ns xmlNodeStructure
	if err := xml.Unmarshal(data, &ns); err != nil {
		return nil, fmt.Errorf("parsing node structure: %w", err)
	}
	result := make([]ObjectInfo, 0, len(ns.Nodes))
	for _, n := range ns.Nodes {
		if n.ObjectName == "" {
			continue // skip empty root node
		}
		result = append(result, ObjectInfo{
			URI:         n.ObjectURI,
			Type:        n.ObjectType,
			Name:        n.ObjectName,
			Description: n.Description,
		})
	}
	return result, nil
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
	var ref xmlObjectReference
	if err := xml.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("GetObjectInfo parsing: %w", err)
	}

	info := ObjectInfo(ref)
	return &info, nil
}
