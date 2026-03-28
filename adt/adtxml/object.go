package adtxml

import "encoding/xml"

// CreateProgram is the XML body for creating an ABAP program.
type CreateProgram struct {
	XMLName     xml.Name   `xml:"program:abapProgram"`
	NSProgram   string     `xml:"xmlns:program,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateClass is the XML body for creating an ABAP class.
type CreateClass struct {
	XMLName     xml.Name   `xml:"class:abapClass"`
	NSClass     string     `xml:"xmlns:class,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateInterface is the XML body for creating an ABAP interface.
type CreateInterface struct {
	XMLName     xml.Name   `xml:"intf:abapInterface"`
	NSIntf      string     `xml:"xmlns:intf,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateFunctionGroup is the XML body for creating a function group.
type CreateFunctionGroup struct {
	XMLName     xml.Name   `xml:"group:abapFunctionGroup"`
	NSGroup     string     `xml:"xmlns:group,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateDataElement is the XML body for creating a data element (DTEL). S4 only.
type CreateDataElement struct {
	XMLName     xml.Name   `xml:"dtel:wbobj"`
	NSDtel      string     `xml:"xmlns:dtel,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateDomain is the XML body for creating a domain (DOMA). S4 only.
type CreateDomain struct {
	XMLName     xml.Name   `xml:"domain:domain"`
	NSDomain    string     `xml:"xmlns:domain,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// CreateFunctionModule is the XML body for creating a function module inside a function group.
type CreateFunctionModule struct {
	XMLName     xml.Name   `xml:"fmodule:abapFunctionModule"`
	NSModule    string     `xml:"xmlns:fmodule,attr"`
	NSCore      string     `xml:"xmlns:adtcore,attr"`
	Type        string     `xml:"adtcore:type,attr"`
	Description string     `xml:"adtcore:description,attr"`
	Name        string     `xml:"adtcore:name,attr"`
	PackageRef  PackageRef `xml:"adtcore:packageRef"`
}

// PackageRef is the adtcore:packageRef element used in object creation.
type PackageRef struct {
	XMLName xml.Name `xml:"adtcore:packageRef"`
	Name    string   `xml:"adtcore:name,attr"`
}

// CreatePackage is the XML body for creating an ABAP package (DEVC).
// Content-Type must be application/vnd.sap.adt.packages.v2+xml.
type CreatePackage struct {
	XMLName              xml.Name        `xml:"pak:package"`
	NSPak                string          `xml:"xmlns:pak,attr"`
	NSCore               string          `xml:"xmlns:adtcore,attr"`
	Name                 string          `xml:"adtcore:name,attr"`
	Type                 string          `xml:"adtcore:type,attr"`
	Description          string          `xml:"adtcore:description,attr"`
	Responsible          string          `xml:"adtcore:responsible,attr"`
	Attributes           PakAttributes   `xml:"pak:attributes"`
	SuperPackage         PakSuperPackage `xml:"pak:superPackage"`
	ApplicationComponent PakAppComponent `xml:"pak:applicationComponent"`
	Transport            PakTransport    `xml:"pak:transport"`
	UseAccesses          PakEmpty        `xml:"pak:useAccesses"`
	PackageInterfaces    PakEmpty        `xml:"pak:packageInterfaces"`
	SubPackages          PakEmpty        `xml:"pak:subPackages"`
}

// PakAttributes holds package attributes like packageType.
type PakAttributes struct {
	PackageType string `xml:"pak:packageType,attr"`
}

// PakSuperPackage references the parent package (empty for top-level).
type PakSuperPackage struct {
	Name string `xml:"adtcore:name,attr,omitempty"`
}

// PakAppComponent references the application component (optional).
type PakAppComponent struct {
	Name string `xml:"pak:name,attr,omitempty"`
}

// PakTransport holds software component and transport layer.
type PakTransport struct {
	SoftwareComponent PakNamedItem `xml:"pak:softwareComponent"`
	TransportLayer    PakNamedItem `xml:"pak:transportLayer"`
}

// PakNamedItem is a package sub-element with a name attribute.
type PakNamedItem struct {
	Name string `xml:"pak:name,attr"`
}

// PakEmpty is an empty XML element required by SAP in the package creation body.
type PakEmpty struct{}
