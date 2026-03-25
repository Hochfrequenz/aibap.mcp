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
// SAP returns <chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
// with <msg> children when there are errors/warnings.
// Verified: 2026-03-24 against hfq.sap.msp.local:8100
type ActivationMessages struct {
	XMLName  xml.Name            `xml:"messages"`
	Messages []ActivationMessage `xml:"msg"`
}

// ActivationMessage is a single message in an activation response.
// Verified: 2026-03-24 against hfq.sap.msp.local:8100
type ActivationMessage struct {
	ObjDescr       string `xml:"objDescr,attr"`
	Type           string `xml:"type,attr"`
	Line           string `xml:"line,attr"`
	Href           string `xml:"href,attr"`
	ForceSupported string `xml:"forceSupported,attr"`
	ShortText      struct {
		Text string `xml:"txt"`
	} `xml:"shortText"`
}
