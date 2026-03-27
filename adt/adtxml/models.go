package adtxml

// Models for SAP ADT XML response formats.
//
// IMPORTANT: Only add types here that have been verified against a real SAP system.
// Do not add hypothesized structures. Each type documents which endpoint it maps to
// and when it was verified.

// LockData is the DATA content of a lock response.
// Endpoint: POST {objectURI}?_action=LOCK&accessMode=MODIFY
// Verified: 2026-03-24 against srvhfuhana.sap.msp.local:44300
type LockData struct {
	LockHandle          string `xml:"LOCK_HANDLE"`
	CorrNr              string `xml:"CORRNR"`
	CorrUser            string `xml:"CORRUSER"`
	CorrText            string `xml:"CORRTEXT"`
	IsLocal             string `xml:"IS_LOCAL"`
	IsLinkUp            string `xml:"IS_LINK_UP"`
	ModificationSupport string `xml:"MODIFICATION_SUPPORT"`
}

// PackageNode is a single node in a BrowsePackage response.
// Endpoint: POST /sap/bc/adt/repository/nodestructure
// Verified: 2026-03-23 against srvhfuhana.sap.msp.local:44300
type PackageNode struct {
	ObjectType  string `xml:"OBJECT_TYPE"`
	ObjectName  string `xml:"OBJECT_NAME"`
	TechName    string `xml:"TECH_NAME"`
	ObjectURI   string `xml:"OBJECT_URI"`
	Description string `xml:"DESCRIPTION"`
	Expandable  string `xml:"EXPANDABLE"`
	Visibility  string `xml:"VISIBILITY"`
	NodeID      string `xml:"NODE_ID"`
}

// PackageTreeContent is the DATA content of a BrowsePackage response.
type PackageTreeContent struct {
	Nodes []PackageNode `xml:"TREE_CONTENT>SEU_ADT_REPOSITORY_OBJ_NODE"`
}

// TransportCheckData is the DATA content of a transport check response.
// Endpoint: POST /sap/bc/adt/cts/transportchecks
// Verified: 2026-03-23 against srvhfuhana.sap.msp.local:44300
type TransportCheckData struct {
	PgmID      string              `xml:"PGMID"`
	Object     string              `xml:"OBJECT"`
	ObjectName string              `xml:"OBJECTNAME"`
	Operation  string              `xml:"OPERATION"`
	DevClass   string              `xml:"DEVCLASS"`
	Result     string              `xml:"RESULT"`
	Recording  string              `xml:"RECORDING"`
	Requests   []TransportCheckReq `xml:"REQUESTS>CTS_REQUEST"`
}

// TransportCheckReq is a transport request within a transport check response.
type TransportCheckReq struct {
	Header TransportCheckHeader `xml:"REQ_HEADER"`
}

// TransportCheckHeader contains the details of a transport request.
type TransportCheckHeader struct {
	TrKorr     string `xml:"TRKORR"`
	TrFunction string `xml:"TRFUNCTION"`
	TrStatus   string `xml:"TRSTATUS"`
	Text       string `xml:"AS4TEXT"`
}

// TransportCheckRequest is the DATA content for a transport check request.
// Endpoint: POST /sap/bc/adt/cts/transportchecks
type TransportCheckRequest struct {
	PgmID      string `xml:"PGMID"`
	Object     string `xml:"OBJECT"`
	ObjectName string `xml:"OBJECTNAME"`
	Operation  string `xml:"OPERATION"`
}

// CreateTransportData is the DATA content for creating a transport request.
// Endpoint: POST /sap/bc/adt/cts/transports
// Verified: 2026-03-23 against srvhfuhana.sap.msp.local:44300
type CreateTransportData struct {
	Category    string `xml:"CATEGORY"`
	Target      string `xml:"TARGET"`
	Description string `xml:"DESCRIPTION"`
	DevClass    string `xml:"DEVCLASS"`
}
