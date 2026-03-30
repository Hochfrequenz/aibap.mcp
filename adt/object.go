package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

// Object type constants for DDIC types used in multiple switch statements.
const (
	objTypeDTEL = "DTEL"
	objTypeDOMA = "DOMA"
	objTypeTABL = "TABL"
	objTypeDDLS = "DDLS"
)

var objectTypeMap = map[string]struct {
	endpoint string
	adtType  string
}{
	"PROG":      {"/sap/bc/adt/programs/programs", "PROG/P"},
	"CLAS":      {"/sap/bc/adt/oo/classes", "CLAS/OC"},
	"INTF":      {"/sap/bc/adt/oo/interfaces", "INTF/OI"},
	"FUGR":      {"/sap/bc/adt/functions/groups", "FUGR/F"},
	objTypeDTEL: {"/sap/bc/adt/ddic/dataelements", "DTEL/DE"},
	objTypeDOMA: {"/sap/bc/adt/ddic/domains", "DOMA/DD"},
	objTypeTABL: {"/sap/bc/adt/ddic/tables", "TABL/DT"},
	objTypeDDLS: {"/sap/bc/adt/ddic/ddl/sources", "DDLS/STOB"},
	"MSAG":      {"/sap/bc/adt/messageclass", "MSAG/N"},
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
	case objTypeDTEL:
		body, err = xml.Marshal(adtxml.CreateDataElement{
			NSDtel: "http://www.sap.com/wbobj/dictionary/dtel", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case objTypeDOMA:
		body, err = xml.Marshal(adtxml.CreateDomain{
			NSDomain: "http://www.sap.com/dictionary/domain", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case objTypeTABL:
		body, err = xml.Marshal(adtxml.CreateTable{
			NSBlue: "http://www.sap.com/wbobj/blue", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case objTypeDDLS:
		body, err = xml.Marshal(adtxml.CreateDDLSource{
			NSDdl: "http://www.sap.com/adt/ddic/ddlsources", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "MSAG":
		body, err = xml.Marshal(adtxml.CreateMessageClass{
			NSMC: "http://www.sap.com/adt/MessageClass", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	}
	if err != nil {
		return fmt.Errorf("CreateObject marshal: %w", err)
	}

	// DDIC objects need specific content types on S4
	ct := contentTypeXML
	switch strings.ToUpper(objectType) {
	case objTypeDTEL:
		ct = "application/vnd.sap.adt.dataelements.v2+xml"
	case objTypeDOMA:
		ct = "application/vnd.sap.adt.domains.v2+xml"
	case objTypeTABL:
		ct = "application/vnd.sap.adt.tables.v2+xml"
	case objTypeDDLS:
		ct = "application/vnd.sap.adt.ddlSource+xml"
	}

	path := info.endpoint
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": ct, "Accept": ct},
	)
	if err != nil {
		return fmt.Errorf("CreateObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 404 {
		ot := strings.ToUpper(objectType)
		if ot == objTypeDTEL || ot == objTypeDOMA || ot == objTypeTABL || ot == objTypeDDLS {
			return fmt.Errorf("CreateObject: the /sap/bc/adt/ddic/ endpoint for %s is not available on this SAP system — "+
				"DDIC object creation via ADT REST requires S/4HANA or a recent ABAP Platform version. "+
				"On older ECC systems, create DDIC objects via transaction SE11", ot)
		}
	}
	return checkResponse(resp)
}

func (c *httpClient) CreateFunctionModule(ctx context.Context, groupName, moduleName, description, packageName, transport string) error {
	body, err := xml.Marshal(adtxml.CreateFunctionModule{
		NSModule:    "http://www.sap.com/adt/functions/fmodules",
		NSCore:      nsADTCore,
		Type:        "FUGR/FF",
		Name:        strings.ToUpper(moduleName),
		Description: description,
		PackageRef:  adtxml.PackageRef{Name: packageName},
	})
	if err != nil {
		return fmt.Errorf("CreateFunctionModule marshal: %w", err)
	}

	path := "/sap/bc/adt/functions/groups/" + strings.ToLower(groupName) + "/fmodules"
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": "application/vnd.sap.adt.functions.fmodules.v2+xml"},
	)
	if err != nil {
		return fmt.Errorf("CreateFunctionModule: %w", err)
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
