package adt

import (
	"testing"
)

func TestParseDefinitionEndLine(t *testing.T) {
	xml := `<abapsource:objectStructureElement xml:base="/sap/bc/adt/oo/classes/zcl_test/objectstructure"
		adtcore:name="ZCL_TEST" adtcore:type="CLAS/OC"
		xmlns:adtcore="http://www.sap.com/adt/core"
		xmlns:abapsource="http://www.sap.com/adt/abapsource"
		xmlns:atom="http://www.w3.org/2005/Atom">
		<atom:link rel="http://www.sap.com/adt/relations/source/definitionIdentifier" href="./source/main#start=1,6;end=1,12"/>
		<atom:link rel="http://www.sap.com/adt/relations/source/implementationIdentifier" href="./source/main#start=30,6;end=30,12"/>
		<atom:link rel="http://www.sap.com/adt/relations/source/definitionBlock" href="./source/main#start=1,0;end=26,8"/>
		<atom:link rel="http://www.sap.com/adt/relations/source/implementationBlock" href="./source/main#start=30,0;end=183,8"/>
	</abapsource:objectStructureElement>`

	line, err := parseDefinitionEndLine([]byte(xml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != 26 {
		t.Errorf("end line: got %d, want 26", line)
	}
}

func TestParseDefinitionEndLine_NotFound(t *testing.T) {
	xml := `<abapsource:objectStructureElement
		xmlns:abapsource="http://www.sap.com/adt/abapsource"
		xmlns:atom="http://www.w3.org/2005/Atom">
		<atom:link rel="http://www.sap.com/adt/relations/other" href="./foo"/>
	</abapsource:objectStructureElement>`

	_, err := parseDefinitionEndLine([]byte(xml))
	if err == nil {
		t.Fatal("expected error for missing definitionBlock")
	}
}
