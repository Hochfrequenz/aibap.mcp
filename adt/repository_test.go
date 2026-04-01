package adt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

func TestBrowsePackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == csrfEndpoint {
			w.Header().Set("X-CSRF-Token", "token")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/sap/bc/adt/repository/nodestructure" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.sap.as+xml" {
			t.Errorf("Accept header: got %q", accept)
		}
		q := r.URL.Query()
		if q.Get("parent_name") != "STUN" {
			t.Errorf("parent_name: got %q", q.Get("parent_name"))
		}
		w.Header().Set("Content-Type", "application/vnd.sap.as+xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <TREE_CONTENT>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>PROG/P</OBJECT_TYPE>
          <OBJECT_NAME>RSPARAM</OBJECT_NAME>
          <TECH_NAME>RSPARAM</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/programs/programs/RSPARAM</OBJECT_URI>
          <DESCRIPTION>Display SAP Profile Parameters</DESCRIPTION>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>DEVC/K</OBJECT_TYPE>
          <OBJECT_NAME>STUN_COMMON</OBJECT_NAME>
          <TECH_NAME>STUN_COMMON</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/packages/stun_common</OBJECT_URI>
          <DESCRIPTION>Common Monitoring</DESCRIPTION>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
      </TREE_CONTENT>
    </DATA>
  </asx:values>
</asx:abap>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	results, err := client.BrowsePackage(context.Background(), "STUN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "RSPARAM" {
		t.Errorf("name[0]: got %q", results[0].Name)
	}
	if results[0].URI != "/sap/bc/adt/programs/programs/RSPARAM" {
		t.Errorf("uri[0]: got %q", results[0].URI)
	}
	if results[0].Type != "PROG/P" {
		t.Errorf("type[0]: got %q", results[0].Type)
	}
	if results[0].Description != "Display SAP Profile Parameters" {
		t.Errorf("description[0]: got %q", results[0].Description)
	}
}

func TestGetObjectInfoProgram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/programs/programs/RSPARAM" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/vnd.sap.adt.programs.programs") {
			t.Errorf("Accept header missing program type: %q", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.programs.programs.v2+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<program:abapProgram
  adtcore:name="RSPARAM" adtcore:type="PROG/P"
  adtcore:description="Display SAP Profile Parameters"
  xmlns:program="http://www.sap.com/adt/programs/programs"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/stun" adtcore:name="STUN"/>
</program:abapProgram>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/programs/programs/RSPARAM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "RSPARAM" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "PROG/P" {
		t.Errorf("type: got %q", info.Type)
	}
	if info.Description != "Display SAP Profile Parameters" {
		t.Errorf("description: got %q", info.Description)
	}
	if info.PackageName != "STUN" {
		t.Errorf("packageName: got %q", info.PackageName)
	}
}

func TestGetObjectInfoClass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/oo/classes/ZCL_EXAMPLE" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "application/vnd.sap.adt.oo.classes") {
			t.Errorf("Accept header missing class type: %q", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.oo.classes.v4+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<class:abapClass adtcore:name="ZCL_EXAMPLE" adtcore:type="CLAS/OC"
  adtcore:description="Example Class"
  xmlns:class="http://www.sap.com/adt/oo/classes"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/ztest" adtcore:name="ZTEST"/>
</class:abapClass>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/oo/classes/ZCL_EXAMPLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZCL_EXAMPLE" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "CLAS/OC" {
		t.Errorf("type: got %q", info.Type)
	}
	if info.PackageName != "ZTEST" {
		t.Errorf("packageName: got %q", info.PackageName)
	}
}

func TestGetObjectInfoInterface(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sap/bc/adt/oo/interfaces/ZIF_EXAMPLE" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.sap.adt.oo.interfaces.v5+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<intf:abapInterface adtcore:name="ZIF_EXAMPLE" adtcore:type="INTF/OI"
  adtcore:description="Example Interface"
  xmlns:intf="http://www.sap.com/adt/oo/interfaces"
  xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/ztest" adtcore:name="ZTEST"/>
</intf:abapInterface>`))
	}))
	defer srv.Close()

	cfg := config.SAPSystem{Host: srv.URL, User: "U", Password: "P", Client: "100"}
	client := adt.NewClient(cfg)

	info, err := client.GetObjectInfo(context.Background(), "/sap/bc/adt/oo/interfaces/ZIF_EXAMPLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "ZIF_EXAMPLE" {
		t.Errorf("name: got %q", info.Name)
	}
	if info.Type != "INTF/OI" {
		t.Errorf("type: got %q", info.Type)
	}
}
