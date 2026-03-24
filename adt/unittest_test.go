package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestRunUnitTests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/abapunit/testruns" {
			body, _ := io.ReadAll(r.Body)
			reqBody := string(body)
			if !strings.Contains(reqBody, "aunit:runConfiguration") {
				t.Error("request body missing aunit:runConfiguration root element")
			}
			if !strings.Contains(reqBody, "objectSet") {
				t.Error("request body missing objectSet element")
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit" xmlns:adtcore="http://www.sap.com/adt/core">
  <program adtcore:uri="/sap/bc/adt/classes/classes/ZCL_TEST" adtcore:name="ZCL_TEST">
    <testClasses><testClass adtcore:name="ZCL_TEST" aunit:testCount="2" aunit:errorCount="0" aunit:failureCount="1">
      <testMethods>
        <testMethod adtcore:name="TEST_PASS" executionTime="0.001"><alerts/></testMethod>
        <testMethod adtcore:name="TEST_FAIL" executionTime="0.002">
          <alerts><alert kind="failedAssertion" severity="critical"><title>Assertion failed</title></alert></alerts>
        </testMethod>
      </testMethods>
    </testClass></testClasses>
  </program>
</aunit:runResult>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.RunUnitTests(context.Background(), "/sap/bc/adt/classes/classes/ZCL_TEST", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed != 1 {
		t.Errorf("passed: got %d, want 1", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("failed: got %d, want 1", result.Failed)
	}
	if len(result.TestCases) != 2 {
		t.Fatalf("expected 2 test cases, got %d", len(result.TestCases))
	}
}
