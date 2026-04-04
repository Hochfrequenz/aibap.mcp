package adtxml

import "encoding/xml"

// TransportRoot is the XML response from GET /sap/bc/adt/cts/transportrequests.
// Real SAP structure: root > workbench > modifiable|released > request
//
//	root > customizing > modifiable|released > request
type TransportRoot struct {
	XMLName     xml.Name       `xml:"root"`
	Workbench   TransportGroup `xml:"workbench"`
	Customizing TransportGroup `xml:"customizing"`
}

// TransportGroup holds modifiable and released transport requests.
type TransportGroup struct {
	Modifiable TransportBucket `xml:"modifiable"`
	Released   TransportBucket `xml:"released"`
}

// TransportBucket holds a list of transport requests.
type TransportBucket struct {
	Requests []TransportRequest `xml:"request"`
}

// TransportRequest is a single transport request in the transport list XML response.
type TransportRequest struct {
	Number      string `xml:"number,attr"`
	Owner       string `xml:"owner,attr"`
	Description string `xml:"desc,attr"`
	Status      string `xml:"status,attr"`
}

// TransportComponent is the XML body for adding an object to a transport.
type TransportComponent struct {
	XMLName   xml.Name `xml:"adtcore:objectReference"`
	NSCore    string   `xml:"xmlns:adtcore,attr"`
	ObjectURI string   `xml:"adtcore:uri,attr"`
}
