package adtmodel

import "encoding/xml"

// CheckMessages is the XML response from POST /sap/bc/adt/checkruns.
type CheckMessages struct {
	XMLName  xml.Name       `xml:"messages"`
	Messages []CheckMessage `xml:"message"`
}

// CheckMessage is a single message in a syntax check response.
type CheckMessage struct {
	Type      string `xml:"type,attr"`
	TypeText  string `xml:"typeText,attr"`
	ShortText struct {
		Text string `xml:"shortText"`
	} `xml:"shortTextElements"`
	Line   int `xml:"line"`
	Column int `xml:"column"`
}
