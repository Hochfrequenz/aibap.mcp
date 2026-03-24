package adtmodel

import "encoding/xml"

// CheckRunReports is the XML response from POST /sap/bc/adt/checkruns
// with Accept: application/vnd.sap.adt.checkmessages+xml.
type CheckRunReports struct {
	XMLName xml.Name         `xml:"checkRunReports"`
	Reports []CheckRunReport `xml:"checkReport"`
}

// CheckRunReport is a single report within a check run response.
type CheckRunReport struct {
	Reporter   string         `xml:"reporter,attr"`
	TriggerURI string         `xml:"triggeringUri,attr"`
	Status     string         `xml:"status,attr"`
	StatusText string         `xml:"statusText,attr"`
	Messages   []CheckMessage `xml:"checkMessageList>checkMessage"`
}

// CheckMessage is a single message in a syntax check report.
// Line and column are extracted from the URI fragment (#start=line,col).
type CheckMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText string `xml:"shortText,attr"`
}
