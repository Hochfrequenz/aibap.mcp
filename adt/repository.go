package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
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
	tc, err := adtxml.UnmarshalASXData[adtxml.PackageTreeContent](data)
	if err != nil {
		return nil, fmt.Errorf("BrowsePackage parsing: %w", err)
	}
	result := make([]ObjectInfo, 0, len(tc.Nodes))
	for _, n := range tc.Nodes {
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

// objectTypeAcceptHeaders maps ADT URI path prefixes to their required Accept headers.
var objectTypeAcceptHeaders = map[string]string{
	"/sap/bc/adt/programs/programs":      "application/vnd.sap.adt.programs.programs.v2+xml",
	"/sap/bc/adt/programs/includes":      "application/vnd.sap.adt.programs.includes.v2+xml",
	"/sap/bc/adt/oo/classes":             "application/vnd.sap.adt.oo.classes.v4+xml",
	"/sap/bc/adt/oo/interfaces":          "application/vnd.sap.adt.oo.interfaces.v5+xml",
	"/sap/bc/adt/functions/groups":       "application/vnd.sap.adt.functions.groups.v3+xml",
	"/sap/bc/adt/ddic/dataelements":      "application/vnd.sap.adt.dataelements.v2+xml",
	"/sap/bc/adt/ddic/domains":           "application/vnd.sap.adt.domains.v2+xml",
	"/sap/bc/adt/ddic/tables":            "application/vnd.sap.adt.tables.v2+xml",
	"/sap/bc/adt/ddic/tabletypes":        "application/vnd.sap.adt.tabletype.v1+xml",
	"/sap/bc/adt/ddic/typegroups":        "application/vnd.sap.adt.ddic.typegroups.v2+xml",
	"/sap/bc/adt/ddic/ddl/sources":       "application/vnd.sap.adt.ddlSource+xml",
	"/sap/bc/adt/ddic/ddlx/sources":      "application/vnd.sap.adt.ddic.ddlx.v1+xml",
	"/sap/bc/adt/ddic/ddla/sources":      "application/vnd.sap.adt.ddic.ddla.v1+xml",
	"/sap/bc/adt/ddic/srvd/sources":      "application/vnd.sap.adt.ddic.srvd.v1+xml",
	"/sap/bc/adt/packages":               "application/vnd.sap.adt.packages.v2+xml",
	"/sap/bc/adt/bo/behaviordefinitions": "application/vnd.sap.adt.blues.v1+xml",
	"/sap/bc/adt/acm/dcl/sources":        "application/vnd.sap.adt.dclSource+xml",
}

// acceptHeaderForURI returns the best Accept header for a given object URI.
// It first checks the ADT discovery cache (populated from /sap/bc/adt/discovery
// during CSRF fetch) for supported content types, then falls back to the
// hardcoded objectTypeAcceptHeaders map.
func (c *httpClient) acceptHeaderForURI(objectURI string) string {
	// Find the best matching prefix from the hardcoded map.
	bestPrefix := ""
	hardcoded := ""
	for prefix, accept := range objectTypeAcceptHeaders {
		if strings.HasPrefix(objectURI, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			hardcoded = accept
		}
	}
	if bestPrefix == "" {
		return contentTypeXML
	}
	// Check if discovery knows this endpoint. If it does, use the first
	// content type the system supports (discovery lists them in preference
	// order). Otherwise fall back to the hardcoded value.
	c.mu.Lock()
	accepted := c.discovery[bestPrefix]
	c.mu.Unlock()
	if len(accepted) > 0 {
		return accepted[0] + ", application/xml"
	}
	return hardcoded + ", application/xml"
}

func (c *httpClient) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	accept := c.acceptHeaderForURI(objectURI)
	resp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": accept})
	if err != nil {
		return nil, fmt.Errorf("GetObjectInfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	return parseGenericObjectInfo(data)
}

// parseGenericObjectInfo extracts ObjectInfo from any ADT object XML response.
// All ADT object types share adtcore:name, adtcore:type, adtcore:description
// attributes on the root element and an <adtcore:packageRef> child element.
func parseGenericObjectInfo(data []byte) (*ObjectInfo, error) {
	var obj struct {
		Name        string `xml:"name,attr"`
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
		PackageRef  struct {
			Name string `xml:"name,attr"`
		} `xml:"packageRef"`
	}
	if err := xml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("GetObjectInfo parsing: %w", err)
	}
	return &ObjectInfo{
		Name:        obj.Name,
		Type:        obj.Type,
		Description: obj.Description,
		PackageName: obj.PackageRef.Name,
	}, nil
}
