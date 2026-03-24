package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
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
// Nested sub-URIs (e.g. /functions/groups/GRP/fmodules/FM) will match
// the parent prefix, which is acceptable for metadata requests.
func acceptHeaderForURI(objectURI string) string {
	bestPrefix := ""
	bestAccept := ""
	for prefix, accept := range objectTypeAcceptHeaders {
		if strings.HasPrefix(objectURI, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestAccept = accept
		}
	}
	if bestAccept != "" {
		return bestAccept + ", application/xml"
	}
	return "application/xml"
}

func (c *httpClient) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	accept := acceptHeaderForURI(objectURI)
	resp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": accept})
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
