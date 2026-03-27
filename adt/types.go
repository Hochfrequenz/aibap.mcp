package adt

import "fmt"

// Common constants used across ADT operations.
const (
	contentTypeXML = "application/xml"
	nsADTCore      = "http://www.sap.com/adt/core"
)

// SourceResult holds ABAP source code and its ETag for optimistic locking.
type SourceResult struct {
	Source string
	ETag   string
}

// ObjectInfo describes an ABAP repository object.
type ObjectInfo struct {
	URI         string
	Type        string
	Name        string
	Description string
	PackageName string
}

// ActivationMessage is a per-object message from an activation response.
type ActivationMessage struct {
	ObjectURI string
	Type      string // "E" error, "W" warning, "I" info
	Text      string
}

// ActivationResult is returned by ActivateObject.
type ActivationResult struct {
	Success  bool
	Messages []ActivationMessage
}

// SyntaxMessage is a single message from a syntax check.
type SyntaxMessage struct {
	Type   string // "E", "W", "I"
	Text   string
	Line   int
	Column int
}

// TestCase represents a single ABAP unit test method result.
type TestCase struct {
	Name          string
	ExecutionTime float64
	Passed        bool
	Messages      []string
}

// TestResult is returned by RunUnitTests.
type TestResult struct {
	Passed    int
	Failed    int
	Errors    int
	TestCases []TestCase
}

// TransportRequest describes a CTS transport request.
type TransportRequest struct {
	Number      string
	Owner       string
	Description string
	Status      string // "D" = modifiable, "L" = released
}

// TransportCheckResult is returned by CheckTransport.
type TransportCheckResult struct {
	PgmID      string             // R3TR, LIMU, etc.
	Object     string             // PROG, CLAS, INTF, etc.
	ObjectName string             // object name
	DevClass   string             // package
	Result     string             // S=success, E=error
	Recording  bool               // true if object can be recorded
	Requests   []TransportRequest // available transport requests
	Messages   []string           // informational/error messages
}

// CompletionItem represents a single code completion proposal.
type CompletionItem struct {
	Text        string
	Description string
}

// ATCCustomizingResult holds ATC configuration from the SAP system.
type ATCCustomizingResult struct {
	SystemCheckVariant string
	Properties         map[string]string
}

// ATCFinding represents a single ATC check finding.
type ATCFinding struct {
	ObjectURI    string
	Priority     string // 1=error, 2=warning, 3=info
	CheckID      string
	CheckTitle   string
	MessageTitle string
	Location     string // e.g. line number reference
}

// ATCResult is returned by RunATCCheck.
type ATCResult struct {
	WorklistID string
	Findings   []ATCFinding
}

// QueryResult holds the result of a SQL query via ADT data preview.
type QueryResult struct {
	Columns     []QueryColumn
	Rows        [][]string // row-major: Rows[rowIdx][colIdx]
	TotalRows   int
	ExecutionMs float64
}

// QueryColumn describes a single column in a query result.
type QueryColumn struct {
	Name        string
	Type        string // ABAP type: C, N, D, T, P, I, etc.
	Description string
	IsKey       bool
}

// ADTError is returned when SAP ADT responds with an error status.
type ADTError struct {
	StatusCode int
	Message    string
}

func (e *ADTError) Error() string {
	return fmt.Sprintf("SAP ADT error %d: %s", e.StatusCode, e.Message)
}
