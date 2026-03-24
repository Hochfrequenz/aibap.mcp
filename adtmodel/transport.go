package adtmodel

import "encoding/xml"

// TransportRoot is the XML response from GET /sap/bc/adt/cts/transportrequests.
type TransportRoot struct {
	XMLName           xml.Name           `xml:"root"`
	WorkbenchRequests []TransportRequest `xml:"workbenchRequests>workbenchRequest"`
}

// TransportRequest is a single transport request in the transport list XML response.
type TransportRequest struct {
	Number      string `xml:"number,attr"`
	Owner       string `xml:"owner,attr"`
	Description string `xml:"shortDescription,attr"`
	Status      string `xml:"status,attr"`
}

// TransportComponent is the XML body for adding an object to a transport.
type TransportComponent struct {
	XMLName   xml.Name `xml:"adtcore:objectReference"`
	NSCore    string   `xml:"xmlns:adtcore,attr"`
	ObjectURI string   `xml:"adtcore:uri,attr"`
}
