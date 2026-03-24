package adtmodel

import "encoding/xml"

// ATCCustomizing represents the ATC configuration response.
// Endpoint: GET /sap/bc/adt/atc/customizing
// Accept: application/vnd.sap.atc.customizing-v1+xml
// Verified against S/4 HANA (srvhfuhana.sap.msp.local:44300) on 2026-03-24.
type ATCCustomizing struct {
	XMLName    xml.Name            `xml:"customizing"`
	Properties []ATCProperty       `xml:"properties>property"`
	Exemption  ATCExemption        `xml:"exemption"`
}

// ATCProperty is a name-value pair from ATC customizing.
type ATCProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// ATCExemption contains exemption reasons and validities.
type ATCExemption struct {
	Reasons    []ATCReason    `xml:"reasons>reason"`
	Validities []ATCValidity  `xml:"validities>validity"`
}

// ATCReason is an exemption reason.
type ATCReason struct {
	ID                   string `xml:"id,attr"`
	Title                string `xml:"title,attr"`
	JustificationMandatory string `xml:"justificationMandatory,attr"`
}

// ATCValidity is an exemption validity option.
type ATCValidity struct {
	ID    string `xml:"id,attr"`
	Value string `xml:"value,attr"`
}

// ATCWorklistRun is the response from POST /sap/bc/adt/atc/runs.
// Accept: application/vnd.sap.atc.run.result.v1+xml
// NOTE: Not yet verified — endpoint returns 500 on our S/4 system (2026-03-24).
type ATCWorklistRun struct {
	XMLName    xml.Name `xml:"worklistRun"`
	WorklistID string   `xml:"worklistId,attr"`
	Timestamp  string   `xml:"worklistTimestamp,attr"`
}

// ATCWorklist is the response from GET /sap/bc/adt/atc/worklists/{id}.
// Accept: application/atc.worklist.v1+xml
// NOTE: Not yet verified — depends on working ATC runs endpoint.
type ATCWorklist struct {
	XMLName       xml.Name     `xml:"worklist"`
	ID            string       `xml:"id,attr"`
	Timestamp     string       `xml:"timestamp,attr"`
	ObjectSets    []ATCObject  `xml:"objects>object"`
}

// ATCObject is an object with ATC findings.
type ATCObject struct {
	URI      string       `xml:"uri,attr"`
	Type     string       `xml:"type,attr"`
	Name     string       `xml:"name,attr"`
	Findings []ATCFinding `xml:"findings>finding"`
}

// ATCFinding is a single ATC check finding.
type ATCFinding struct {
	URI         string `xml:"uri,attr"`
	Location    string `xml:"location,attr"`
	Priority    string `xml:"priority,attr"`
	CheckID     string `xml:"checkId,attr"`
	CheckTitle  string `xml:"checkTitle,attr"`
	MessageTitle string `xml:"messageTitle,attr"`
}
