package adt

import (
	"strings"
	"testing"
)

func TestParseMessageClassXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass adtcore:name="ZTEST" adtcore:description="Test messages"
    xmlns:mc="http://www.sap.com/adt/MessageClass"
    xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:packageRef adtcore:name="ZTEST_PKG"/>
  <mc:messages mc:msgno="001" mc:msgtext="Hello &amp;1" mc:selfexplainatory="true" mc:documented="false" adtcore:name=""/>
  <mc:messages mc:msgno="002" mc:msgtext="Error: &amp;1 in &amp;2" mc:selfexplainatory="false" mc:documented="true" adtcore:name=""/>
  <mc:messages mc:msgno="003" mc:selfexplainatory="false" mc:documented="false" adtcore:name=""/>
</mc:messageClass>`)

	result, err := parseMessageClassXML(data)
	if err != nil {
		t.Fatalf("parseMessageClassXML: %v", err)
	}
	if result.Name != "ZTEST" {
		t.Errorf("name: got %q, want ZTEST", result.Name)
	}
	if result.Description != "Test messages" {
		t.Errorf("description: got %q", result.Description)
	}
	if result.Package != "ZTEST_PKG" {
		t.Errorf("package: got %q", result.Package)
	}
	// Message 003 has no text but has a number, so it's included
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].Number != "001" {
		t.Errorf("msg[0] number: got %q", result.Messages[0].Number)
	}
	if result.Messages[0].Text != "Hello &1" {
		t.Errorf("msg[0] text: got %q", result.Messages[0].Text)
	}
	if !result.Messages[0].SelfExpl {
		t.Error("msg[0] self_explanatory should be true")
	}
	if result.Messages[1].SelfExpl {
		t.Error("msg[1] self_explanatory should be false")
	}
}

func TestParseMessageClassXML_Empty(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass adtcore:name="ZEMPTY" adtcore:description="Empty"
    xmlns:mc="http://www.sap.com/adt/MessageClass"
    xmlns:adtcore="http://www.sap.com/adt/core">
</mc:messageClass>`)

	result, err := parseMessageClassXML(data)
	if err != nil {
		t.Fatalf("parseMessageClassXML: %v", err)
	}
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result.Messages))
	}
}

func TestParseNamedItemList(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<nameditem:namedItemList xmlns:nameditem="http://www.sap.com/adt/nameditem">
  <nameditem:totalItemCount>2</nameditem:totalItemCount>
  <nameditem:namedItem>
    <nameditem:name>00/001</nameditem:name>
    <nameditem:description>&amp;1&amp;2</nameditem:description>
    <nameditem:data>/sap/bc/adt/messageclass/00</nameditem:data>
  </nameditem:namedItem>
  <nameditem:namedItem>
    <nameditem:name>00/002</nameditem:name>
    <nameditem:description>Enter a valid value</nameditem:description>
    <nameditem:data>/sap/bc/adt/messageclass/00</nameditem:data>
  </nameditem:namedItem>
</nameditem:namedItemList>`)

	results, err := parseNamedItemList(data)
	if err != nil {
		t.Fatalf("parseNamedItemList: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "00/001" {
		t.Errorf("name: got %q", results[0].Name)
	}
	if results[1].Description != "Enter a valid value" {
		t.Errorf("description: got %q", results[1].Description)
	}
	if results[0].URI != "/sap/bc/adt/messageclass/00" {
		t.Errorf("URI: got %q", results[0].URI)
	}
}

func TestBuildMessageClassPutXML(t *testing.T) {
	messages := []Message{
		{Number: "001", Text: "Hello &1", SelfExpl: true},
		{Number: "002", Text: "Error <&1>", SelfExpl: false},
	}
	xml := buildMessageClassPutXML("ZTEST", messages)

	if !strings.Contains(xml, `adtcore:name="ZTEST"`) {
		t.Error("missing message class name")
	}
	if !strings.Contains(xml, `mc:msgno="001"`) {
		t.Error("missing message 001")
	}
	if !strings.Contains(xml, `mc:msgtext="Hello &amp;1"`) {
		t.Error("missing/unescaped message text")
	}
	if !strings.Contains(xml, `mc:msgtext="Error &lt;&amp;1&gt;"`) {
		t.Error("missing/unescaped message text with angle brackets")
	}
	if !strings.Contains(xml, `mc:selfexplainatory="true"`) {
		t.Error("msg 001 should be self-explanatory")
	}
	if !strings.Contains(xml, `mc:selfexplainatory="false"`) {
		t.Error("msg 002 should not be self-explanatory")
	}
}

func TestBoolToString(t *testing.T) {
	if boolToString(true) != "true" {
		t.Error("true case")
	}
	if boolToString(false) != "false" {
		t.Error("false case")
	}
}
