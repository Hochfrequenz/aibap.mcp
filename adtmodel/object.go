package adtmodel

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

// PackageRef is the adtcore:packageRef element used in object creation.
type PackageRef struct {
	XMLName xml.Name `xml:"adtcore:packageRef"`
	Name    string   `xml:"adtcore:name,attr"`
}
