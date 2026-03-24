package adtmodel

import "encoding/xml"

// ActivationRequest is the XML body for POST /sap/bc/adt/activation.
type ActivationRequest struct {
	XMLName xml.Name `xml:"adtcore:objectReferences"`
	NS      string   `xml:"xmlns:adtcore,attr"`
	Objects []ActivationObject
}

// ActivationObject is a single object reference in an activation request.
type ActivationObject struct {
	XMLName xml.Name `xml:"adtcore:objectReference"`
	URI     string   `xml:"adtcore:uri,attr"`
}

// ActivationMessages is the response from POST /sap/bc/adt/activation.
type ActivationMessages struct {
	XMLName  xml.Name               `xml:"messages"`
	Messages []ActivationMessage `xml:"message"`
}

// ActivationMessage is a single message in an activation response.
type ActivationMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText struct {
		Text string `xml:"shortText"`
	} `xml:"shortTextElements"`
}
