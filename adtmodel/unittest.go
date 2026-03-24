package adtmodel

import "encoding/xml"

// RunConfiguration is the XML request body for POST /sap/bc/adt/abapunit/testruns.
type RunConfiguration struct {
	XMLName  xml.Name   `xml:"aunit:runConfiguration"`
	NS       string     `xml:"xmlns:aunit,attr"`
	External External   `xml:"external"`
	Options  RunOptions `xml:"options"`
	Objects  ObjectSets `xml:"adtcore:objectSets"`
}

// External contains external options for a unit test run.
type External struct {
	Coverage Coverage `xml:"coverage"`
}

// Coverage controls code coverage during unit test runs.
type Coverage struct {
	Active string `xml:"active,attr"`
}

// RunOptions contains execution options for a unit test run.
type RunOptions struct {
	URIType                   Value             `xml:"uriType"`
	TestDeterminationStrategy TestDetermination `xml:"testDeterminationStrategy"`
	TestRiskLevels            RiskLevels        `xml:"testRiskLevels"`
	TestDurations             Durations         `xml:"testDurations"`
}

// Value is a generic value element with a value attribute.
type Value struct {
	Value string `xml:"value,attr"`
}

// TestDetermination controls which tests are selected.
type TestDetermination struct {
	SameProgram   string `xml:"sameProgram,attr"`
	AssignedTests string `xml:"assignedTests,attr"`
	PublicMethods string `xml:"publicMethods,attr"`
}

// RiskLevels controls which risk levels to include.
type RiskLevels struct {
	Harmless  string `xml:"harmless,attr"`
	Dangerous string `xml:"dangerous,attr"`
	Critical  string `xml:"critical,attr"`
}

// Durations controls which test durations to include.
type Durations struct {
	Short  string `xml:"short,attr"`
	Medium string `xml:"medium,attr"`
	Long   string `xml:"long,attr"`
}

// ObjectSets wraps the object set for unit test target selection.
type ObjectSets struct {
	XMLName xml.Name  `xml:"adtcore:objectSets"`
	NS      string    `xml:"xmlns:adtcore,attr"`
	Set     ObjectSet `xml:"objectSet"`
}

// ObjectSet is a set of object references with a kind attribute.
type ObjectSet struct {
	Kind       string          `xml:"kind,attr"`
	References AUnitObjectRefs `xml:"adtcore:objectReferences"`
}

// AUnitObjectRefs wraps object references for unit test runs.
type AUnitObjectRefs struct {
	Refs []ObjectRef `xml:"adtcore:objectReference"`
}

// ObjectRef is a single object reference URI.
type ObjectRef struct {
	URI string `xml:"adtcore:uri,attr"`
}

// RunResult is the XML response from a unit test run.
type RunResult struct {
	XMLName  xml.Name  `xml:"runResult"`
	Programs []Program `xml:"program"`
}

// Program is a single program in a unit test result.
type Program struct {
	Classes []TestClass `xml:"testClasses>testClass"`
}

// TestClass is a single test class result.
type TestClass struct {
	Name         string       `xml:"name,attr"`
	FailureCount int          `xml:"failureCount,attr"`
	ErrorCount   int          `xml:"errorCount,attr"`
	Methods      []TestMethod `xml:"testMethods>testMethod"`
}

// TestMethod is a single test method result.
type TestMethod struct {
	Name          string  `xml:"name,attr"`
	ExecutionTime float64 `xml:"executionTime,attr"`
	Alerts        []Alert `xml:"alerts>alert"`
}

// Alert is a test failure/error alert.
type Alert struct {
	Kind     string `xml:"kind,attr"`
	Severity string `xml:"severity,attr"`
	Title    string `xml:"title"`
}
