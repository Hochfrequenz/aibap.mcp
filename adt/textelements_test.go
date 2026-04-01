package adt

import "testing"

func TestParseTextSymbols(t *testing.T) {
	source := "@MaxLength:50\n001=ABAP Objects Performance Examples\n\n@MaxLength:20\n002=syntax error occured\n"
	symbols := parseTextSymbols(source)
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0].Key != "001" {
		t.Errorf("key: got %q", symbols[0].Key)
	}
	if symbols[0].Text != "ABAP Objects Performance Examples" {
		t.Errorf("text: got %q", symbols[0].Text)
	}
	if symbols[0].MaxLength != 50 {
		t.Errorf("max_length: got %d", symbols[0].MaxLength)
	}
	if symbols[1].MaxLength != 20 {
		t.Errorf("max_length[1]: got %d", symbols[1].MaxLength)
	}
}

func TestParseTextSymbols_Empty(t *testing.T) {
	symbols := parseTextSymbols("")
	if len(symbols) != 0 {
		t.Errorf("expected 0, got %d", len(symbols))
	}
}

func TestParseSelectionTexts(t *testing.T) {
	source := "OSCLOCK =Betriebssystemuhr\n\nPAR_CNT =Anzahl Durchläufe\n"
	texts := parseSelectionTexts(source)
	if len(texts) != 2 {
		t.Fatalf("expected 2 texts, got %d", len(texts))
	}
	if texts[0].Name != "OSCLOCK" {
		t.Errorf("name: got %q", texts[0].Name)
	}
	if texts[0].Text != "Betriebssystemuhr" {
		t.Errorf("text: got %q", texts[0].Text)
	}
	if texts[1].Name != "PAR_CNT" {
		t.Errorf("name[1]: got %q", texts[1].Name)
	}
}

func TestResolveTextElementPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"/sap/bc/adt/programs/programs/ZTEST", "/sap/bc/adt/textelements/programs/ZTEST"},
		{"/sap/bc/adt/oo/classes/ZCL_TEST", "/sap/bc/adt/textelements/classes/ZCL_TEST"},
		{"/sap/bc/adt/functions/groups/ZFGRP", "/sap/bc/adt/textelements/functiongroups/ZFGRP"},
	}
	for _, tt := range tests {
		got, err := resolveTextElementPath(tt.uri)
		if err != nil {
			t.Errorf("resolveTextElementPath(%q): %v", tt.uri, err)
			continue
		}
		if got != tt.want {
			t.Errorf("resolveTextElementPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}

	// Unsupported type
	_, err := resolveTextElementPath("/sap/bc/adt/ddic/tables/ZTABLE")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}
