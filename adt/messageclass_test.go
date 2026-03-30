package adt

import "testing"

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
