package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Request XML structs for aunit:runConfiguration format.
type xmlRunConfiguration struct {
	XMLName  xml.Name      `xml:"aunit:runConfiguration"`
	NS       string        `xml:"xmlns:aunit,attr"`
	External xmlExternal   `xml:"external"`
	Options  xmlRunOptions `xml:"options"`
	Objects  xmlObjectSets `xml:"adtcore:objectSets"`
}

type xmlExternal struct {
	Coverage xmlCoverage `xml:"coverage"`
}

type xmlCoverage struct {
	Active string `xml:"active,attr"`
}

type xmlRunOptions struct {
	URIType                   xmlValue             `xml:"uriType"`
	TestDeterminationStrategy xmlTestDetermination `xml:"testDeterminationStrategy"`
	TestRiskLevels            xmlRiskLevels        `xml:"testRiskLevels"`
	TestDurations             xmlDurations         `xml:"testDurations"`
}

type xmlValue struct {
	Value string `xml:"value,attr"`
}

type xmlTestDetermination struct {
	SameProgram   string `xml:"sameProgram,attr"`
	AssignedTests string `xml:"assignedTests,attr"`
	PublicMethods string `xml:"publicMethods,attr"`
}

type xmlRiskLevels struct {
	Harmless  string `xml:"harmless,attr"`
	Dangerous string `xml:"dangerous,attr"`
	Critical  string `xml:"critical,attr"`
}

type xmlDurations struct {
	Short  string `xml:"short,attr"`
	Medium string `xml:"medium,attr"`
	Long   string `xml:"long,attr"`
}

type xmlObjectSets struct {
	XMLName xml.Name     `xml:"adtcore:objectSets"`
	NS      string       `xml:"xmlns:adtcore,attr"`
	Set     xmlObjectSet `xml:"objectSet"`
}

type xmlObjectSet struct {
	Kind       string             `xml:"kind,attr"`
	References xmlAUnitObjectRefs `xml:"adtcore:objectReferences"`
}

type xmlAUnitObjectRefs struct {
	Refs []xmlObjectRef `xml:"adtcore:objectReference"`
}

type xmlObjectRef struct {
	URI string `xml:"adtcore:uri,attr"`
}

// Response XML structs.
type xmlRunResult struct {
	XMLName  xml.Name     `xml:"runResult"`
	Programs []xmlProgram `xml:"program"`
}

type xmlProgram struct {
	Classes []xmlTestClass `xml:"testClasses>testClass"`
}

type xmlTestClass struct {
	Name         string          `xml:"name,attr"`
	FailureCount int             `xml:"failureCount,attr"`
	ErrorCount   int             `xml:"errorCount,attr"`
	Methods      []xmlTestMethod `xml:"testMethods>testMethod"`
}

type xmlTestMethod struct {
	Name          string     `xml:"name,attr"`
	ExecutionTime float64    `xml:"executionTime,attr"`
	Alerts        []xmlAlert `xml:"alerts>alert"`
}

type xmlAlert struct {
	Kind     string `xml:"kind,attr"`
	Severity string `xml:"severity,attr"`
	Title    string `xml:"title"`
}

func (c *httpClient) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	reqBody := xmlRunConfiguration{
		NS: "http://www.sap.com/adt/aunit",
		External: xmlExternal{
			Coverage: xmlCoverage{Active: "false"},
		},
		Options: xmlRunOptions{
			URIType:                   xmlValue{Value: "semantic"},
			TestDeterminationStrategy: xmlTestDetermination{SameProgram: "true", AssignedTests: "false", PublicMethods: "false"},
			TestRiskLevels:            xmlRiskLevels{Harmless: "true", Dangerous: "true", Critical: "true"},
			TestDurations:             xmlDurations{Short: "true", Medium: "true", Long: "true"},
		},
		Objects: xmlObjectSets{
			NS: nsADTCore,
			Set: xmlObjectSet{
				Kind: "inclusive",
				References: xmlAUnitObjectRefs{
					Refs: []xmlObjectRef{{URI: objectURI}},
				},
			},
		},
	}

	body, err := xml.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal unit test request: %w", err)
	}

	resp, err := c.doMutate(reqCtx, http.MethodPost,
		"/sap/bc/adt/abapunit/testruns",
		strings.NewReader(xml.Header+string(body)),
		map[string]string{
			"Content-Type": contentTypeXML,
			"Accept":       contentTypeXML,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("RunUnitTests: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var runResult xmlRunResult
	xml.Unmarshal(data, &runResult) //nolint:errcheck

	result := &TestResult{}
	for _, prog := range runResult.Programs {
		for _, class := range prog.Classes {
			for _, method := range class.Methods {
				tc := TestCase{
					Name:          method.Name,
					ExecutionTime: method.ExecutionTime,
					Passed:        len(method.Alerts) == 0,
				}
				for _, alert := range method.Alerts {
					tc.Messages = append(tc.Messages, alert.Title)
				}
				result.TestCases = append(result.TestCases, tc)
				if tc.Passed {
					result.Passed++
				} else {
					result.Failed++
				}
			}
			result.Errors += class.ErrorCount
		}
	}
	return result, nil
}
