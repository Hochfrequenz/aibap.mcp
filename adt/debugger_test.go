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

func TestDebugSessionSetBreakpoint(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/breakpoints" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger"><breakpoint kind="line" id="BP1" adtcore:uri="/sap/bc/adt/programs/programs/ztest/source/main#start=2" adtcore:type="PROG/P" adtcore:name="ZTEST" xmlns:adtcore="http://www.sap.com/adt/core"/></dbg:breakpoints>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)
	dbg := adt.NewDebugSession(client, "U")

	bp, err := dbg.SetBreakpoint(context.Background(), "/sap/bc/adt/programs/programs/ztest/source/main", 2, "PROG/P", "ZTEST")
	if err != nil {
		t.Fatalf("SetBreakpoint: %v", err)
	}
	if bp.ID != "BP1" {
		t.Errorf("id: got %q", bp.ID)
	}
	if !strings.Contains(gotBody, "scope=\"external\"") {
		t.Errorf("body missing scope=external: %s", gotBody)
	}
}

func TestDebugSessionStartListenerTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/listeners" {
			// Simulate timeout — empty response
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	result, err := dbg.StartListener(context.Background(), 1)
	if err != nil {
		t.Fatalf("StartListener: %v", err)
	}
	if result.Status != "timeout" {
		t.Errorf("status: got %q, want timeout", result.Status)
	}
}

func TestDebugSessionStopListener(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/listeners" {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	err := dbg.StopListener(context.Background())
	if err != nil {
		t.Fatalf("StopListener: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", gotMethod)
	}
	if gotPath != "/sap/bc/adt/debugger/listeners" {
		t.Errorf("path: got %q", gotPath)
	}
}

func TestDebugSessionGetDebuggeeSessions(t *testing.T) {
	var gotPath, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger" && r.URL.Query().Get("method") == "getDebuggeeSessions" {
			gotPath = r.URL.Path
			gotAccept = r.Header.Get("Accept")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<sessions><session id="123"/></sessions>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	data, err := dbg.GetDebuggeeSessions(context.Background())
	if err != nil {
		t.Fatalf("GetDebuggeeSessions: %v", err)
	}
	if gotPath != "/sap/bc/adt/debugger" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotAccept != "application/vnd.sap.as+xml" {
		t.Errorf("accept: got %q", gotAccept)
	}
	if !strings.Contains(string(data), "session") {
		t.Errorf("unexpected response: %s", data)
	}
}

func TestDebugSessionAttach(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger" && r.URL.Query().Get("method") == "attach" {
			gotPath = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	err := dbg.Attach(context.Background(), "debuggee-42")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if !strings.Contains(gotPath, "debuggeeId=debuggee-42") {
		t.Errorf("path missing debuggeeId: %s", gotPath)
	}
}

func TestDebugSessionStep(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/actions" {
			gotPath = r.URL.RequestURI()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<step result="ok"/>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	data, err := dbg.Step(context.Background(), "stepInto")
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !strings.Contains(gotPath, "action=stepInto") {
		t.Errorf("path missing action: %s", gotPath)
	}
	if !strings.Contains(string(data), "step") {
		t.Errorf("unexpected response: %s", data)
	}
}

func TestDebugSessionGetVariable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/variables/LV_TEST/value" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<variable name="LV_TEST" value="hello"/>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	data, err := dbg.GetVariable(context.Background(), "LV_TEST")
	if err != nil {
		t.Fatalf("GetVariable: %v", err)
	}
	if !strings.Contains(string(data), "LV_TEST") {
		t.Errorf("unexpected response: %s", data)
	}
}

func TestDebugSessionGetStack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/sap/bc/adt/debugger/systemareas/stack" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<stack><frame level="1" name="MAIN"/></stack>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := config.SAPConfig{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	dbg := adt.NewDebugSession(adt.NewClient(cfg), "U")

	data, err := dbg.GetStack(context.Background())
	if err != nil {
		t.Fatalf("GetStack: %v", err)
	}
	if !strings.Contains(string(data), "stack") {
		t.Errorf("unexpected response: %s", data)
	}
}
