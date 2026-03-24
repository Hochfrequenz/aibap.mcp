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
