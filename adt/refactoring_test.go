package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

// evaluateResponse is a sample SAP ADT refactoring evaluate response.
const evaluateResponse = `<?xml version="1.0" encoding="utf-8"?>
<rename:renameRefactoring xmlns:rename="http://www.sap.com/adt/ris/refactoring/rename"
  xmlns:generic="http://www.sap.com/adt/ris/refactoring/generic">
  <rename:oldName>OLD_VAR</rename:oldName>
  <rename:newName>OLD_VAR</rename:newName>
  <generic:transport/>
</rename:renameRefactoring>`

// previewResponse is a sample SAP ADT refactoring preview response with affected objects.
const previewResponse = `<?xml version="1.0" encoding="utf-8"?>
<rename:renameRefactoring xmlns:rename="http://www.sap.com/adt/ris/refactoring/rename"
  xmlns:generic="http://www.sap.com/adt/ris/refactoring/generic">
  <rename:oldName>OLD_VAR</rename:oldName>
  <rename:newName>NEW_VAR</rename:newName>
  <generic:transport/>
  <generic:affectedObjects>
    <generic:affectedObject uri="/sap/bc/adt/programs/programs/ZTEST_PROG" type="PROG/P" name="ZTEST_PROG">
      <generic:textReplaceDeltas>
        <generic:textReplaceDelta>
          <generic:rangeFragment>#start=2,6;end=2,13</generic:rangeFragment>
          <generic:contentOld>OLD_VAR</generic:contentOld>
          <generic:contentNew>NEW_VAR</generic:contentNew>
        </generic:textReplaceDelta>
        <generic:textReplaceDelta>
          <generic:rangeFragment>#start=5,10;end=5,17</generic:rangeFragment>
          <generic:contentOld>OLD_VAR</generic:contentOld>
          <generic:contentNew>NEW_VAR</generic:contentNew>
        </generic:textReplaceDelta>
      </generic:textReplaceDeltas>
    </generic:affectedObject>
    <generic:affectedObject uri="/sap/bc/adt/programs/includes/ZTEST_INCLUDE" type="PROG/I" name="ZTEST_INCLUDE">
      <generic:textReplaceDeltas>
        <generic:textReplaceDelta>
          <generic:rangeFragment>#start=10,4;end=10,11</generic:rangeFragment>
          <generic:contentOld>OLD_VAR</generic:contentOld>
          <generic:contentNew>NEW_VAR</generic:contentNew>
        </generic:textReplaceDelta>
      </generic:textReplaceDeltas>
    </generic:affectedObject>
  </generic:affectedObjects>
</rename:renameRefactoring>`

// executeResponse is a sample SAP ADT refactoring execute response.
const executeResponse = `<?xml version="1.0" encoding="utf-8"?>
<rename:renameRefactoring xmlns:rename="http://www.sap.com/adt/ris/refactoring/rename">
  <rename:oldName>OLD_VAR</rename:oldName>
  <rename:newName>NEW_VAR</rename:newName>
</rename:renameRefactoring>`

func TestRenameSuccess(t *testing.T) {
	var steps []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/refactorings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/xml" {
			t.Errorf("Content-Type: got %q, want application/xml", ct)
		}
		if accept := r.Header.Get("Accept"); accept != "application/xml" {
			t.Errorf("Accept: got %q, want application/xml", accept)
		}

		step := r.URL.Query().Get("step")
		steps = append(steps, step)
		w.Header().Set("Content-Type", "application/xml")

		switch step {
		case "evaluate":
			// Verify evaluate params
			if rel := r.URL.Query().Get("rel"); rel != "http://www.sap.com/adt/relations/refactoring/rename" {
				t.Errorf("evaluate rel: got %q", rel)
			}
			if uri := r.URL.Query().Get("uri"); uri != "/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6" {
				t.Errorf("evaluate uri: got %q", uri)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(evaluateResponse))
		case "preview":
			// Verify preview sends modified XML with new name
			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)
			if !strings.Contains(bodyStr, "<rename:newName>NEW_VAR</rename:newName>") {
				t.Errorf("preview body missing new name, got:\n%s", bodyStr)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(previewResponse))
		case "execute":
			// Verify execute sends the preview XML
			body, _ := io.ReadAll(r.Body)
			if len(body) == 0 {
				t.Error("execute body is empty")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(executeResponse))
		default:
			t.Errorf("unexpected step: %q", step)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6", "NEW_VAR", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all three steps were called in order
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d: %v", len(steps), steps)
	}
	if steps[0] != "evaluate" || steps[1] != "preview" || steps[2] != "execute" {
		t.Errorf("steps: got %v, want [evaluate preview execute]", steps)
	}

	// Verify result
	if result.OldName != "OLD_VAR" {
		t.Errorf("OldName: got %q, want OLD_VAR", result.OldName)
	}
	if result.NewName != "NEW_VAR" {
		t.Errorf("NewName: got %q, want NEW_VAR", result.NewName)
	}
	if len(result.AffectedObjects) != 2 {
		t.Fatalf("expected 2 affected objects, got %d", len(result.AffectedObjects))
	}

	// First affected object
	obj0 := result.AffectedObjects[0]
	if obj0.URI != "/sap/bc/adt/programs/programs/ZTEST_PROG" {
		t.Errorf("obj[0].URI: got %q", obj0.URI)
	}
	if obj0.Type != "PROG/P" {
		t.Errorf("obj[0].Type: got %q", obj0.Type)
	}
	if obj0.Name != "ZTEST_PROG" {
		t.Errorf("obj[0].Name: got %q", obj0.Name)
	}
	if len(obj0.Locations) != 2 {
		t.Fatalf("obj[0] expected 2 locations, got %d", len(obj0.Locations))
	}
	if obj0.Locations[0].Range != "#start=2,6;end=2,13" {
		t.Errorf("obj[0].Locations[0].Range: got %q", obj0.Locations[0].Range)
	}
	if obj0.Locations[0].ContentOld != "OLD_VAR" {
		t.Errorf("obj[0].Locations[0].ContentOld: got %q", obj0.Locations[0].ContentOld)
	}
	if obj0.Locations[0].ContentNew != "NEW_VAR" {
		t.Errorf("obj[0].Locations[0].ContentNew: got %q", obj0.Locations[0].ContentNew)
	}

	// Second affected object
	obj1 := result.AffectedObjects[1]
	if obj1.URI != "/sap/bc/adt/programs/includes/ZTEST_INCLUDE" {
		t.Errorf("obj[1].URI: got %q", obj1.URI)
	}
	if obj1.Type != "PROG/I" {
		t.Errorf("obj[1].Type: got %q", obj1.Type)
	}
	if len(obj1.Locations) != 1 {
		t.Fatalf("obj[1] expected 1 location, got %d", len(obj1.Locations))
	}
}

func TestRenameWithTransport(t *testing.T) {
	var executeBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/refactorings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		step := r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)

		switch step {
		case "evaluate":
			_, _ = w.Write([]byte(evaluateResponse))
		case "preview":
			_, _ = w.Write([]byte(previewResponse))
		case "execute":
			body, _ := io.ReadAll(r.Body)
			executeBody = string(body)
			_, _ = w.Write([]byte(executeResponse))
		}
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6", "NEW_VAR", "DEVK900123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the transport was injected into the execute body
	if !strings.Contains(executeBody, "<generic:transport>DEVK900123</generic:transport>") {
		t.Errorf("execute body should contain transport DEVK900123, got:\n%s", executeBody)
	}
	// The empty placeholder should no longer be present
	if strings.Contains(executeBody, "<generic:transport/>") {
		t.Errorf("execute body should not contain empty transport placeholder, got:\n%s", executeBody)
	}
}

func TestRenameEvaluateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		// Return error on evaluate step
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<exc:exception xmlns:exc="http://www.sap.com/adt/exceptions">
  <exc:message>Symbol not found at position</exc:message>
</exc:exception>`))
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=99,1", "NEW_VAR", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Rename evaluate") {
		t.Errorf("error should mention evaluate step, got: %v", err)
	}
}

func TestRenamePreviewError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}

		step := r.URL.Query().Get("step")
		switch step {
		case "evaluate":
			callCount++
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(evaluateResponse))
		case "preview":
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<exc:exception xmlns:exc="http://www.sap.com/adt/exceptions">
  <exc:message>Preview failed</exc:message>
</exc:exception>`))
		default:
			callCount++
		}
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6", "NEW_VAR", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Rename preview") {
		t.Errorf("error should mention preview step, got: %v", err)
	}
	// Execute step should not have been called
	if callCount != 2 {
		t.Errorf("expected 2 calls (evaluate + preview), got %d", callCount)
	}
}

func TestRenameExecuteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}

		step := r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/xml")

		switch step {
		case "evaluate":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(evaluateResponse))
		case "preview":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(previewResponse))
		case "execute":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<exc:exception xmlns:exc="http://www.sap.com/adt/exceptions">
  <exc:message>Object locked by another user</exc:message>
</exc:exception>`))
		}
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6", "NEW_VAR", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Rename execute") {
		t.Errorf("error should mention execute step, got: %v", err)
	}
}

func TestRenameMissingOldName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}

		step := r.URL.Query().Get("step")
		if step == "evaluate" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			// Response without oldName tag
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<rename:renameRefactoring xmlns:rename="http://www.sap.com/adt/ris/refactoring/rename">
  <rename:newName></rename:newName>
</rename:renameRefactoring>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := sapmcpconfig.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.Rename(context.Background(),
		"/sap/bc/adt/programs/programs/ZTEST_PROG#start=2,6", "NEW_VAR", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "could not find old name") {
		t.Errorf("error should mention missing old name, got: %v", err)
	}
}
