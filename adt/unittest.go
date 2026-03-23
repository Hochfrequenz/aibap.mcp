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

type xmlUnitTestRunRequest struct {
	XMLName xml.Name           `xml:"aunit:run"`
	NS      string             `xml:"xmlns:aunit,attr"`
	NSCore  string             `xml:"xmlns:adtcore,attr"`
	Timeout int                `xml:"adtcore:timeout,attr"`
	Objects []xmlUnitTestObject
}

type xmlUnitTestObject struct {
	XMLName xml.Name `xml:"adtcore:objectReference"`
	URI     string   `xml:"adtcore:uri,attr"`
}

type xmlRunResult struct {
	XMLName  xml.Name     `xml:"runResult"`
	Programs []xmlProgram `xml:"program"`
}

type xmlProgram struct {
	Classes []xmlTestClass `xml:"testClass"`
}

type xmlTestClass struct {
	Name         string          `xml:"name,attr"`
	FailureCount int             `xml:"failureCount,attr"`
	ErrorCount   int             `xml:"errorCount,attr"`
	Methods      []xmlTestMethod `xml:"testMethod"`
}

type xmlTestMethod struct {
	Name          string     `xml:"name,attr"`
	ExecutionTime float64    `xml:"executionTime,attr"`
	Alerts        []xmlAlert `xml:"alerts>alert"`
}

type xmlAlert struct {
	Type  string `xml:"type,attr"`
	Title string `xml:"title"`
}

func (c *httpClient) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	body, err := xml.Marshal(xmlUnitTestRunRequest{
		NS:      "http://www.sap.com/adt/aunit",
		NSCore:  nsADTCore,
		Timeout: timeoutSeconds * 1000,
		Objects: []xmlUnitTestObject{{URI: objectURI}},
	})
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
	defer resp.Body.Close()
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
