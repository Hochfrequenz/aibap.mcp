package adt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestCheckTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/cts/transportchecks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>ZTEST</OBJECTNAME>
      <OPERATION>I</OPERATION>
      <DEVCLASS>ZPACKAGE</DEVCLASS>
      <RESULT>S</RESULT>
      <RECORDING>X</RECORDING>
      <REQUESTS>
        <CTS_REQUEST>
          <REQ_HEADER>
            <TRKORR>DEVK900001</TRKORR>
            <TRFUNCTION>K</TRFUNCTION>
            <TRSTATUS>D</TRSTATUS>
            <AS4TEXT>My transport</AS4TEXT>
          </REQ_HEADER>
        </CTS_REQUEST>
      </REQUESTS>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	result, err := client.CheckTransport(context.Background(), "R3TR", "PROG", "ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "S" {
		t.Errorf("Result: got %q, want S", result.Result)
	}
	if !result.Recording {
		t.Error("expected Recording=true")
	}
	if result.DevClass != "ZPACKAGE" {
		t.Errorf("DevClass: got %q", result.DevClass)
	}
	if len(result.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(result.Requests))
	}
	if result.Requests[0].Number != "DEVK900001" {
		t.Errorf("transport number: got %q", result.Requests[0].Number)
	}
}

func TestGetTransportRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/cts/transportrequests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.sap.adt.transportorganizertree.v1+xml" {
			t.Errorf("Accept header: got %q, want %q", accept, "application/vnd.sap.adt.transportorganizertree.v1+xml")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm" xmlns:adtcore="http://www.sap.com/adt/core">
  <tm:workbench tm:category="Workbench">
    <tm:modifiable tm:status="Modifiable">
      <tm:request tm:number="DEVK900123" tm:owner="DEVELOPER" tm:desc="Feature transport" tm:status="D"/>
    </tm:modifiable>
    <tm:released tm:status="Released">
      <tm:request tm:number="DEVK900124" tm:owner="DEVELOPER" tm:desc="Released transport" tm:status="L"/>
    </tm:released>
  </tm:workbench>
  <tm:customizing tm:category="Customizing">
    <tm:modifiable tm:status="Modifiable">
      <tm:request tm:number="DEVK900125" tm:owner="DEVELOPER" tm:desc="Customizing transport" tm:status="D"/>
    </tm:modifiable>
  </tm:customizing>
</tm:root>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	transports, err := client.GetTransportRequests(context.Background(), "", "D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transports) != 3 {
		t.Fatalf("expected 3 transports (1 modifiable wb + 1 released wb + 1 customizing), got %d", len(transports))
	}
	if transports[0].Number != "DEVK900123" {
		t.Errorf("workbench modifiable: got %q", transports[0].Number)
	}
	if transports[1].Number != "DEVK900124" {
		t.Errorf("workbench released: got %q", transports[1].Number)
	}
	if transports[2].Number != "DEVK900125" {
		t.Errorf("customizing: got %q", transports[2].Number)
	}
}

func TestAddToTransport(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	err := client.AddToTransport(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "DEVK900123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "/sap/bc/adt/cts/transportrequests/DEVK900123/abaptransportcomponents"
	if gotPath != expected {
		t.Errorf("path: got %q, want %q", gotPath, expected)
	}
}

func TestCreateTransport(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/cts/transports" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <TRKORR>DEVK900999</TRKORR>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	nr, err := client.CreateTransport(context.Background(), "K", "DUM", "My description", "ZTEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nr != "DEVK900999" {
		t.Errorf("transport number: got %q, want %q", nr, "DEVK900999")
	}
	// REQUEST_TEXT sets AS4TEXT (short text) on both ECC and S4 (see #226).
	// DESCRIPTION sets the documentation tab (required for release on some systems).
	if !strings.Contains(gotBody, "<REQUEST_TEXT>My description</REQUEST_TEXT>") {
		t.Errorf("body must contain REQUEST_TEXT, got:\n%s", gotBody)
	}
	if !strings.Contains(gotBody, "<DESCRIPTION>My description</DESCRIPTION>") {
		t.Errorf("body must contain DESCRIPTION, got:\n%s", gotBody)
	}
	if strings.Contains(gotBody, "<AS4TEXT>") {
		t.Error("body must not contain AS4TEXT (use REQUEST_TEXT instead)")
	}
}

func TestCreateTransportWithoutPackage(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <TRKORR>DEVK900888</TRKORR>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	nr, err := client.CreateTransport(context.Background(), "K", "", "No package transport", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nr != "DEVK900888" {
		t.Errorf("transport number: got %q", nr)
	}
	if strings.Contains(gotBody, "<DEVCLASS>") {
		t.Errorf("body must not contain DEVCLASS when package is empty, got:\n%s", gotBody)
	}
	if strings.Contains(gotBody, "<TARGET>") {
		t.Errorf("body must not contain TARGET when target is empty, got:\n%s", gotBody)
	}
}

func TestParseBackgroundRunPollURI(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "sync release response (no background run)",
			data: `<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">
  <tm:releasereports><tm:checkReport chkrun:status="released"/></tm:releasereports>
</tm:root>`,
			want: "",
		},
		{
			name: "background run with self link",
			data: `<?xml version="1.0" encoding="utf-8"?>
<runs:run xmlns:runs="http://www.sap.com/adt/bgrun" xmlns:atom="http://www.w3.org/2005/Atom">
  <runs:status>running</runs:status>
  <atom:link rel="self" href="/sap/bc/adt/system/backgroundruns/12345"/>
</runs:run>`,
			want: "/sap/bc/adt/system/backgroundruns/12345",
		},
		{
			name: "empty data",
			data: "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adt.ParseBackgroundRunPollURI([]byte(tt.data))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReleaseTransportAsync(t *testing.T) {
	pollCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		// POST to newreleasejobs → return background run
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "newreleasejobs") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<runs:run xmlns:runs="http://www.sap.com/adt/bgrun" xmlns:atom="http://www.w3.org/2005/Atom">
  <runs:status>new</runs:status>
  <atom:link rel="self" href="/sap/bc/adt/system/backgroundruns/99"/>
</runs:run>`))
			return
		}
		// GET poll URI → first call: running, second: finished with result link
		if r.URL.Path == "/sap/bc/adt/system/backgroundruns/99" {
			pollCount++
			if pollCount < 2 {
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<runs:run xmlns:runs="http://www.sap.com/adt/bgrun" xmlns:atom="http://www.w3.org/2005/Atom">
  <runs:status>running</runs:status>
</runs:run>`))
			} else {
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<runs:run xmlns:runs="http://www.sap.com/adt/bgrun" xmlns:atom="http://www.w3.org/2005/Atom">
  <runs:status>finished</runs:status>
  <atom:link rel="http://www.sap.com/adt/relations/runs/result" href="/sap/bc/adt/system/backgroundruns/99/result" type="application/http"/>
</runs:run>`))
			}
			return
		}
		// GET result URI → release report
		if r.URL.Path == "/sap/bc/adt/system/backgroundruns/99/result" {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm" xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <tm:releasereports><tm:checkReport chkrun:status="released"/></tm:releasereports>
</tm:root>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClientWithPollInterval(cfg, 10*time.Millisecond)

	err := client.ReleaseTransport(context.Background(), "DEVK900123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pollCount < 2 {
		t.Errorf("expected at least 2 poll requests, got %d", pollCount)
	}
}

func TestCreateTransportTask(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<tm:root tm:number="S4UK902500" xmlns:tm="http://www.sap.com/cts/adt/tm">
  <tm:task tm:owner="U" tm:desc="My task"/>
</tm:root>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	taskNumber, err := client.CreateTransportTask(context.Background(), "S4UK902339", "", "My task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %q, want POST", gotMethod)
	}
	expected := "/sap/bc/adt/cts/transportrequests/S4UK902339/tasks"
	if gotPath != expected {
		t.Errorf("path: got %q, want %q", gotPath, expected)
	}
	if taskNumber != "S4UK902500" {
		t.Errorf("task number: got %q, want S4UK902500", taskNumber)
	}
}

func TestDeleteAndReleaseTransport(t *testing.T) {
	tests := []struct {
		name       string
		call       func(adt.Client) error
		wantMethod string
		wantPath   string
	}{
		{
			name:       "delete",
			call:       func(c adt.Client) error { return c.DeleteTransport(context.Background(), "DEVK900123") },
			wantMethod: http.MethodDelete,
			wantPath:   "/sap/bc/adt/cts/transportrequests/DEVK900123",
		},
		{
			name:       "release",
			call:       func(c adt.Client) error { return c.ReleaseTransport(context.Background(), "DEVK900123") },
			wantMethod: http.MethodPost,
			wantPath:   "/sap/bc/adt/cts/transportrequests/DEVK900123/newreleasejobs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == csrfEndpoint {
					w.Header().Set("X-CSRF-Token", "token")
					w.WriteHeader(http.StatusOK)
					return
				}
				gotPath = r.URL.Path
				gotMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
			client := adt.NewClient(cfg)

			if err := tt.call(client); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method: got %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path: got %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}
