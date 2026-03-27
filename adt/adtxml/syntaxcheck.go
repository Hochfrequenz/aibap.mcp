package adtxml

import "encoding/xml"

// CheckRunReports is the XML response from POST /sap/bc/adt/checkruns.
// Endpoint: POST /sap/bc/adt/checkruns
// Verified: 2026-03-24 against srvhfuhana.sap.msp.local:44300
type CheckRunReports struct {
	XMLName xml.Name         `xml:"checkRunReports"`
	Reports []CheckRunReport `xml:"checkReport"`
}

// CheckRunReport is a single report in a check run response.
type CheckRunReport struct {
	Reporter   string         `xml:"reporter,attr"`
	TriggerURI string         `xml:"triggeringUri,attr"`
	Status     string         `xml:"status,attr"`
	StatusText string         `xml:"statusText,attr"`
	Messages   []CheckMessage `xml:"checkMessageList>checkMessage"`
}

// CheckMessage is a single message in a check run report.
type CheckMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText string `xml:"shortText,attr"`
}
