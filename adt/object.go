package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

type xmlCreateProgram struct {
	XMLName     xml.Name       `xml:"program:abapProgram"`
	NSProgram   string         `xml:"xmlns:program,attr"`
	NSCore      string         `xml:"xmlns:adtcore,attr"`
	Type        string         `xml:"adtcore:type,attr"`
	Description string         `xml:"adtcore:description,attr"`
	Name        string         `xml:"adtcore:name,attr"`
	PackageRef  xmlPackageRef  `xml:"adtcore:packageRef"`
}

type xmlCreateClass struct {
	XMLName     xml.Name       `xml:"class:abapClass"`
	NSClass     string         `xml:"xmlns:class,attr"`
	NSCore      string         `xml:"xmlns:adtcore,attr"`
	Type        string         `xml:"adtcore:type,attr"`
	Description string         `xml:"adtcore:description,attr"`
	Name        string         `xml:"adtcore:name,attr"`
	PackageRef  xmlPackageRef  `xml:"adtcore:packageRef"`
}

type xmlCreateInterface struct {
	XMLName     xml.Name       `xml:"intf:abapInterface"`
	NSIntf      string         `xml:"xmlns:intf,attr"`
	NSCore      string         `xml:"xmlns:adtcore,attr"`
	Type        string         `xml:"adtcore:type,attr"`
	Description string         `xml:"adtcore:description,attr"`
	Name        string         `xml:"adtcore:name,attr"`
	PackageRef  xmlPackageRef  `xml:"adtcore:packageRef"`
}

type xmlPackageRef struct {
	XMLName xml.Name `xml:"adtcore:packageRef"`
	Name    string   `xml:"adtcore:name,attr"`
}

var objectTypeMap = map[string]struct {
	endpoint string
	adtType  string
}{
	"PROG": {"/sap/bc/adt/programs/programs", "PROG/P"},
	"CLAS": {"/sap/bc/adt/oo/classes", "CLAS/OC"},
	"INTF": {"/sap/bc/adt/oo/interfaces", "INTF/OI"},
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
	pkgRef := xmlPackageRef{Name: packageName}
	switch strings.ToUpper(objectType) {
	case "PROG":
		body, err = xml.Marshal(xmlCreateProgram{
			NSProgram: "http://www.sap.com/adt/programs/programs", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "CLAS":
		body, err = xml.Marshal(xmlCreateClass{
			NSClass: "http://www.sap.com/adt/oo/classes", NSCore: nsADTCore,
			Type: info.adtType, Description: description, Name: name, PackageRef: pkgRef,
		})
	case "INTF":
		body, err = xml.Marshal(xmlCreateInterface{
			NSIntf: "http://www.sap.com/adt/oo/interfaces", NSCore: nsADTCore,
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

func (c *httpClient) DeleteObject(ctx context.Context, objectURI, transport string) error {
	path := objectURI
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return fmt.Errorf("DeleteObject: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}
