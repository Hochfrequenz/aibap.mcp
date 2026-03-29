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

// discoveryWithInlineSyntaxCheck returns ADT discovery XML that advertises
// the inline syntax check content type.
const discoveryWithInlineSyntaxCheck = `<?xml version="1.0" encoding="utf-8"?>
<app:service xmlns:app="http://www.w3.org/2007/app" xmlns:atom="http://www.w3.org/2005/Atom">
  <app:workspace>
    <app:collection href="/sap/bc/adt/programs/programs">
      <app:accept>application/vnd.sap.adt.functions.abapsource.syntaxcheck.v1+xml</app:accept>
    </app:collection>
  </app:workspace>
</app:service>`

func TestInlineSyntaxCheckWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.Header().Set("Content-Type", "application/atomsvc+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(discoveryWithInlineSyntaxCheck))
			return
		}
		if r.URL.Path != "/sap/bc/adt/programs/programs/ZTEST/source/main" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("_action") != "CHECK" {
			t.Errorf("missing _action=CHECK query param: %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "asx:abap") {
			t.Errorf("body missing ASX envelope: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "WRITE lv_foo.") {
			t.Errorf("body missing source code: %s", bodyStr)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.functions.abapsource.syntaxcheck.v1+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
<asx:values>
<DATA>
<ERRORS_WITH_URI>
<item><LINE>3</LINE><COL>7</COL><MESSAGE>Field "LV_FOO" is unknown.</MESSAGE><URI>/sap/bc/adt/programs/programs/ZTEST/source/main</URI></item>
</ERRORS_WITH_URI>
<WARNINGS_WITH_URI>
<item><LINE>1</LINE><COL>1</COL><MESSAGE>Unused variable LV_BAR.</MESSAGE><URI>/sap/bc/adt/programs/programs/ZTEST/source/main</URI></item>
</WARNINGS_WITH_URI>
</DATA>
</asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	source := "REPORT ztest.\nDATA lv_bar TYPE string.\nWRITE lv_foo."
	msgs, err := client.InlineSyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Type != "E" {
		t.Errorf("msg[0] type: got %q, want E", msgs[0].Type)
	}
	if msgs[0].Line != 3 {
		t.Errorf("msg[0] line: got %d, want 3", msgs[0].Line)
	}
	if msgs[1].Type != "W" {
		t.Errorf("msg[1] type: got %q, want W", msgs[1].Type)
	}
	if msgs[1].Line != 1 {
		t.Errorf("msg[1] line: got %d, want 1", msgs[1].Line)
	}
}

func TestInlineSyntaxCheckNotSupported(t *testing.T) {
	// Server without inline syntax check in discovery → should return ErrInlineSyntaxCheckNotSupported.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			// No discovery XML → no inline syntax check support
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	_, err := client.InlineSyntaxCheck(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "REPORT ztest.")
	if err == nil {
		t.Fatal("expected error for unsupported system")
	}
	if err.Error() != "inline syntax check not supported by this system" {
		t.Errorf("unexpected error: %v", err)
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
