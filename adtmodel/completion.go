package adtmodel

import "encoding/xml"

// Completions is the XML response from the code completion endpoint.
type Completions struct {
	XMLName xml.Name     `xml:"completions"`
	Items   []Completion `xml:"completion"`
}

// Completion is a single code completion proposal in XML.
type Completion struct {
	Text        string `xml:"text,attr"`
	Description string `xml:"description,attr"`
}
