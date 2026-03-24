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

func TestSyntaxCheckWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/checkruns" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/vnd.sap.adt.checkobjects+xml" {
			t.Errorf("Content-Type: got %q", ct)
		}
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.sap.adt.checkmessages+xml" {
			t.Errorf("Accept: got %q", accept)
		}
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "checkObjectList") {
			t.Errorf("body missing checkObjectList: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "/sap/bc/adt/programs/programs/ZTEST") {
			t.Errorf("body missing object URI: %s", bodyStr)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun"
    chkrun:triggeringUri="/sap/bc/adt/programs/programs/ZTEST"
    chkrun:status="processed" chkrun:statusText="Syntax check performed">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/programs/ZTEST/source/main#start=42,5"
        chkrun:type="E" chkrun:shortText="Field &quot;FOO&quot; is unknown."/>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != "E" {
		t.Errorf("type: got %q", msgs[0].Type)
	}
	if msgs[0].Text != `Field "FOO" is unknown.` {
		t.Errorf("text: got %q", msgs[0].Text)
	}
	if msgs[0].Line != 42 {
		t.Errorf("line: got %d", msgs[0].Line)
	}
	if msgs[0].Column != 5 {
		t.Errorf("column: got %d", msgs[0].Column)
	}
}

func TestSyntaxCheckClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun"
    chkrun:triggeringUri="/sap/bc/adt/programs/programs/ZTEST"
    chkrun:status="processed" chkrun:statusText="Object ZTEST has been checked"/>
</chkrun:checkRunReports>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	msgs, err := client.SyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for clean check, got %d", len(msgs))
	}
}
