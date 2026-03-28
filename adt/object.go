package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

var objectTypeMap = map[string]struct {
	endpoint string
	adtType  string
}{
	"PROG": {"/sap/bc/adt/programs/programs", "PROG/P"},
	"CLAS": {"/sap/bc/adt/oo/classes", "CLAS/OC"},
	"INTF": {"/sap/bc/adt/oo/interfaces", "INTF/OI"},
	"FUGR": {"/sap/bc/adt/functions/groups", "FUGR/F"},
}

func (c *httpClient) CreateObject(ctx context.Context, objectType, name, packageName, description, transport string) error {
	info, ok := objectTypeMap[strings.ToUpper(objectType)]
	if !ok {
		supported := make([]string, 0, len(objectTypeMap))
		for k := range objectTypeMap {
			supported = append(supported, k)
		}
		return fmt.Errorf("unsupported object type %q, supported: %s", objectType, strings.Join(supported, ", "))
	}

	var body []byte
	var err error
	pkgRef := adtxml.PackageRef{Name: packageName}
	switch strings.ToUpper(objectType) {
	case "PROG":
		body, err = xml.Marshal(adtxml.CreateProgram{
			NSProgram: "http://www.sap.com/adt/programs/programs", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "CLAS":
		body, err = xml.Marshal(adtxml.CreateClass{
			NSClass: "http://www.sap.com/adt/oo/classes", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "INTF":
		body, err = xml.Marshal(adtxml.CreateInterface{
			NSIntf: "http://www.sap.com/adt/oo/interfaces", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "FUGR":
		body, err = xml.Marshal(adtxml.CreateFunctionGroup{
			NSGroup: "http://www.sap.com/adt/functions/groups", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	}
	if err != nil {
		return fmt.Errorf("CreateObject marshal: %w", err)
	}

	path := info.endpoint
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": contentTypeXML},
	)
	if err != nil {
		return fmt.Errorf("CreateObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

func (c *httpClient) CreatePackage(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error {
	body, err := xml.Marshal(adtxml.CreatePackage{
		NSPak:       "http://www.sap.com/adt/packages",
		NSCore:      nsADTCore,
		Name:        strings.ToUpper(name),
		Type:        "DEVC/K",
		Description: description,
		Responsible: strings.ToUpper(responsible),
		Attributes:  adtxml.PakAttributes{PackageType: "development"},
		Transport: adtxml.PakTransport{
			SoftwareComponent: adtxml.PakNamedItem{Name: softwareComponent},
			TransportLayer:    adtxml.PakNamedItem{Name: transportLayer},
		},
	})
	if err != nil {
		return fmt.Errorf("CreatePackage marshal: %w", err)
	}

	path := "/sap/bc/adt/packages"
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": "application/vnd.sap.adt.packages.v2+xml"},
	)
	if err != nil {
		return fmt.Errorf("CreatePackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 404 {
		return fmt.Errorf("CreatePackage: the /sap/bc/adt/packages endpoint is not available on this SAP system — " +
			"package creation via ADT REST requires S/4HANA or a recent ABAP Platform version. " +
			"On older ECC systems, create the package manually via transaction SE80 or SE21, then use it in CreateObject")
	}
	return checkResponse(resp)
}

func (c *httpClient) DeleteObject(ctx context.Context, objectURI, lockHandle, transport string) error {
	path := objectURI
	if transport != "" {
		path += "?corrNr=" + transport
	}
	// Use optimistic locking via If-Match header: SAP locks internally and deletes.
	// The pessimistic path (lockHandle query param) fails on some systems because
	// CL_ADT_ENQUEUE=>READ doesn't find the REST-session lock.
	// Fetch the ETag from the object URI itself (not /source/main).
	accept := acceptHeaderForURI(objectURI)
	etagResp, err := c.doRead(ctx, objectURI, map[string]string{"Accept": accept})
	if err != nil {
		return fmt.Errorf("DeleteObject fetch ETag: %w", err)
	}
	etag := etagResp.Header.Get("ETag")
	_ = etagResp.Body.Close()
	if etag == "" {
		return fmt.Errorf("DeleteObject: no ETag returned for %s", objectURI)
	}
	headers := map[string]string{
		"If-Match": etag,
	}
	resp, err := c.doMutate(ctx, http.MethodDelete, path, nil, headers)
	if err != nil {
		return fmt.Errorf("DeleteObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}
