package adt

import (
	"encoding/xml"
	"strings"
	"testing"
)

// --- Unmarshal tests using real SAP response samples ---

func TestUnmarshalASXData_LockResponse(t *testing.T) {
	// Real lock response from SAP ADT.
	raw := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <LOCK_HANDLE>15F0D1EAA10BDCBE10C24098848DC83FF52C7A5F</LOCK_HANDLE>
      <CORRNR/>
      <CORRUSER/>
      <CORRTEXT/>
      <IS_LOCAL/>
      <IS_LINK_UP/>
      <MODIFICATION_SUPPORT>ModificationsLoggedOnly</MODIFICATION_SUPPORT>
    </DATA>
  </asx:values>
</asx:abap>`

	type lockData struct {
		LockHandle string `xml:"LOCK_HANDLE"`
		CorrNr     string `xml:"CORRNR"`
	}

	got, err := UnmarshalASXData[lockData]([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LockHandle != "15F0D1EAA10BDCBE10C24098848DC83FF52C7A5F" {
		t.Errorf("LockHandle: got %q", got.LockHandle)
	}
	if got.CorrNr != "" {
		t.Errorf("CorrNr: got %q, want empty", got.CorrNr)
	}
}

func TestUnmarshalASXData_BrowsePackageResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <TREE_CONTENT>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>DEVC/K</OBJECT_TYPE>
          <OBJECT_NAME>STUN_COMMON</OBJECT_NAME>
          <TECH_NAME>STUN_COMMON</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/packages/stun_common</OBJECT_URI>
          <EXPANDABLE>X</EXPANDABLE>
          <NODE_ID>000002</NODE_ID>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
        <SEU_ADT_REPOSITORY_OBJ_NODE>
          <OBJECT_TYPE>PROG/P</OBJECT_TYPE>
          <OBJECT_NAME>ZREPORT</OBJECT_NAME>
          <TECH_NAME>ZREPORT</TECH_NAME>
          <OBJECT_URI>/sap/bc/adt/programs/programs/zreport</OBJECT_URI>
          <EXPANDABLE/>
          <NODE_ID>000003</NODE_ID>
        </SEU_ADT_REPOSITORY_OBJ_NODE>
      </TREE_CONTENT>
    </DATA>
  </asx:values>
</asx:abap>`

	type repoNode struct {
		ObjectType string `xml:"OBJECT_TYPE"`
		ObjectName string `xml:"OBJECT_NAME"`
		TechName   string `xml:"TECH_NAME"`
		ObjectURI  string `xml:"OBJECT_URI"`
		Expandable string `xml:"EXPANDABLE"`
		NodeID     string `xml:"NODE_ID"`
	}
	type treeContent struct {
		Nodes []repoNode `xml:"TREE_CONTENT>SEU_ADT_REPOSITORY_OBJ_NODE"`
	}

	got, err := UnmarshalASXData[treeContent]([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got.Nodes))
	}
	if got.Nodes[0].ObjectType != "DEVC/K" {
		t.Errorf("node[0].ObjectType: got %q", got.Nodes[0].ObjectType)
	}
	if got.Nodes[0].ObjectName != "STUN_COMMON" {
		t.Errorf("node[0].ObjectName: got %q", got.Nodes[0].ObjectName)
	}
	if got.Nodes[1].ObjectURI != "/sap/bc/adt/programs/programs/zreport" {
		t.Errorf("node[1].ObjectURI: got %q", got.Nodes[1].ObjectURI)
	}
}

func TestUnmarshalASXData_TransportCheckResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap version="1.0" xmlns:asx="http://www.sap.com/abapxml">
  <asx:values>
    <DATA>
      <PGMID>R3TR</PGMID>
      <OBJECT>PROG</OBJECT>
      <OBJECTNAME>RSPARAM</OBJECTNAME>
      <OPERATION>I</OPERATION>
      <DEVCLASS>STUN</DEVCLASS>
      <RESULT>S</RESULT>
      <RECORDING>X</RECORDING>
      <REQUESTS>
        <CTS_REQUEST>
          <REQ_HEADER>
            <TRKORR>S4UK902321</TRKORR>
            <TRFUNCTION>K</TRFUNCTION>
            <TRSTATUS>D</TRSTATUS>
            <AS4TEXT>zdm_sql</AS4TEXT>
          </REQ_HEADER>
        </CTS_REQUEST>
      </REQUESTS>
    </DATA>
  </asx:values>
</asx:abap>`

	type reqHeader struct {
		TrKorr     string `xml:"TRKORR"`
		TrFunction string `xml:"TRFUNCTION"`
		TrStatus   string `xml:"TRSTATUS"`
		Text       string `xml:"AS4TEXT"`
	}
	type ctsRequest struct {
		Header reqHeader `xml:"REQ_HEADER"`
	}
	type transportCheck struct {
		PgmID      string       `xml:"PGMID"`
		Object     string       `xml:"OBJECT"`
		ObjectName string       `xml:"OBJECTNAME"`
		Result     string       `xml:"RESULT"`
		Requests   []ctsRequest `xml:"REQUESTS>CTS_REQUEST"`
	}

	got, err := UnmarshalASXData[transportCheck]([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PgmID != "R3TR" {
		t.Errorf("PgmID: got %q", got.PgmID)
	}
	if got.ObjectName != "RSPARAM" {
		t.Errorf("ObjectName: got %q", got.ObjectName)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got.Requests))
	}
	if got.Requests[0].Header.TrKorr != "S4UK902321" {
		t.Errorf("TrKorr: got %q", got.Requests[0].Header.TrKorr)
	}
	if got.Requests[0].Header.Text != "zdm_sql" {
		t.Errorf("Text: got %q", got.Requests[0].Header.Text)
	}
}

// --- Marshal tests ---

func TestMarshalASXData_TransportCreateRequest(t *testing.T) {
	type createTransport struct {
		Category    string `xml:"CATEGORY"`
		Target      string `xml:"TARGET"`
		Description string `xml:"DESCRIPTION"`
		DevClass    string `xml:"DEVCLASS"`
	}

	input := createTransport{
		Category:    "K",
		Target:      "DUM",
		Description: "My transport description",
		DevClass:    "$TMP",
	}

	data, err := MarshalASXData(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	xml := string(data)
	if !strings.Contains(xml, `xmlns:asx="http://www.sap.com/abapxml"`) {
		t.Error("missing asx namespace")
	}
	if !strings.Contains(xml, "<CATEGORY>K</CATEGORY>") {
		t.Error("missing CATEGORY element")
	}
	if !strings.Contains(xml, "<DESCRIPTION>My transport description</DESCRIPTION>") {
		t.Error("missing DESCRIPTION element")
	}
	if !strings.Contains(xml, "<DEVCLASS>$TMP</DEVCLASS>") {
		t.Error("missing DEVCLASS element")
	}
}

// --- Round-trip test ---

func TestASXData_RoundTrip(t *testing.T) {
	type simple struct {
		Name  string `xml:"NAME"`
		Value string `xml:"VALUE"`
	}

	original := simple{Name: "TEST_KEY", Value: "test_value"}

	data, err := MarshalASXData(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := UnmarshalASXData[simple](data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != original.Name {
		t.Errorf("Name: got %q, want %q", got.Name, original.Name)
	}
	if got.Value != original.Value {
		t.Errorf("Value: got %q, want %q", got.Value, original.Value)
	}
}

// --- Edge cases ---

func TestUnmarshalASXData_EmptyData(t *testing.T) {
	raw := `<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA/></asx:values></asx:abap>`

	type empty struct {
		XMLName xml.Name `xml:"DATA"`
		Field   string   `xml:"FIELD"`
	}

	got, err := UnmarshalASXData[empty]([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Field != "" {
		t.Errorf("Field: got %q, want empty", got.Field)
	}
}

func TestUnmarshalASXData_InvalidXML(t *testing.T) {
	_, err := UnmarshalASXData[struct{}]([]byte("not xml at all"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}
