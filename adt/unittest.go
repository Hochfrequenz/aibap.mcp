package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func (c *httpClient) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	reqBody := adtxml.RunConfiguration{
		NS: "http://www.sap.com/adt/aunit",
		External: adtxml.External{
			Coverage: adtxml.Coverage{Active: "false"},
		},
		Options: adtxml.RunOptions{
			URIType:                   adtxml.Value{Value: "semantic"},
			TestDeterminationStrategy: adtxml.TestDetermination{SameProgram: "true", AssignedTests: "false", PublicMethods: "false"},
			TestRiskLevels:            adtxml.RiskLevels{Harmless: "true", Dangerous: "true", Critical: "true"},
			TestDurations:             adtxml.Durations{Short: "true", Medium: "true", Long: "true"},
		},
		Objects: adtxml.ObjectSets{
			NS: nsADTCore,
			Set: adtxml.ObjectSet{
				Kind: "inclusive",
				References: adtxml.AUnitObjectRefs{
					Refs: []adtxml.ObjectRef{{URI: objectURI}},
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
	var runResult adtxml.RunResult
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
