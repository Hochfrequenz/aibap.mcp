package adt

import (
	"testing"
)

func TestParseEnhancementSpotXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<objectData name="BADI_TEST_SPOT" description="Test Enhancement Spot"
    xmlns:adtcore="http://www.sap.com/adt/core">
  <packageRef name="TEST_PKG"/>
  <contentSpecific>
    <badiTechnology>
      <badiDefinitions>
        <badiDefinition name="BADI_TEST_DEF" shorttext="Test BAdI" singleUse="true" useFallbackClass="false">
          <interface uri="/sap/bc/adt/oo/interfaces/IF_TEST" type="INTF" name="IF_TEST"/>
          <sampleClasses>
            <sampleClass uri="/sap/bc/adt/oo/classes/CL_TEST_SAMPLE" type="CLAS" name="CL_TEST_SAMPLE"/>
          </sampleClasses>
          <filters>
            <filter filterName="COUNTRY" filterType="CHAR"/>
            <filter filterName="COMPANY" filterType="CHAR"/>
          </filters>
        </badiDefinition>
        <badiDefinition name="BADI_TEST_DEF2" shorttext="Second BAdI" singleUse="false" useFallbackClass="true">
          <interface uri="/sap/bc/adt/oo/interfaces/IF_TEST2" type="INTF" name="IF_TEST2"/>
          <sampleClasses/>
        </badiDefinition>
      </badiDefinitions>
    </badiTechnology>
  </contentSpecific>
</objectData>`)

	result, err := parseEnhancementSpotXML(data)
	if err != nil {
		t.Fatalf("parseEnhancementSpotXML: %v", err)
	}
	if result.Name != "BADI_TEST_SPOT" {
		t.Errorf("name: got %q, want BADI_TEST_SPOT", result.Name)
	}
	if result.Description != "Test Enhancement Spot" {
		t.Errorf("description: got %q", result.Description)
	}
	if result.Package != "TEST_PKG" {
		t.Errorf("package: got %q", result.Package)
	}
	if len(result.Definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(result.Definitions))
	}

	d := result.Definitions[0]
	if d.Name != "BADI_TEST_DEF" {
		t.Errorf("def[0] name: got %q", d.Name)
	}
	if d.Description != "Test BAdI" {
		t.Errorf("def[0] description: got %q", d.Description)
	}
	if !d.SingleUse {
		t.Error("def[0] should be single-use")
	}
	if d.UseFallbackClass {
		t.Error("def[0] should not use fallback class")
	}
	if d.Interface.Name != "IF_TEST" {
		t.Errorf("def[0] interface: got %q", d.Interface.Name)
	}
	if d.SampleClass == nil || d.SampleClass.Name != "CL_TEST_SAMPLE" {
		t.Errorf("def[0] sample class: got %v", d.SampleClass)
	}
	if len(d.Filters) != 2 {
		t.Fatalf("def[0] expected 2 filters, got %d", len(d.Filters))
	}
	if d.Filters[0].Name != "COUNTRY" {
		t.Errorf("def[0] filter[0]: got %q", d.Filters[0].Name)
	}

	d2 := result.Definitions[1]
	if d2.SingleUse {
		t.Error("def[1] should not be single-use")
	}
	if !d2.UseFallbackClass {
		t.Error("def[1] should use fallback class")
	}
	if d2.SampleClass != nil {
		t.Errorf("def[1] sample class should be nil, got %v", d2.SampleClass)
	}
	if len(d2.Filters) != 0 {
		t.Errorf("def[1] expected 0 filters, got %d", len(d2.Filters))
	}
}

func TestParseEnhancementSpotXML_Empty(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<objectData name="EMPTY_SPOT" description="Empty">
  <packageRef name="PKG"/>
  <contentSpecific>
    <badiTechnology>
      <badiDefinitions/>
    </badiTechnology>
  </contentSpecific>
</objectData>`)

	result, err := parseEnhancementSpotXML(data)
	if err != nil {
		t.Fatalf("parseEnhancementSpotXML: %v", err)
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected 0 definitions, got %d", len(result.Definitions))
	}
}

func TestParseEnhancementImplXML(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<objectData name="TEST_IMPL" description="Test Implementation"
    xmlns:adtcore="http://www.sap.com/adt/core">
  <packageRef name="TEST_PKG"/>
  <contentSpecific>
    <badiTechnology>
      <badiImplementations>
        <badiImplementation name="IMPL_ONE" shortText="First impl" isActive="true" isDefault="false">
          <enhancementSpot uri="/sap/bc/adt/enhancements/enhsxs/BADI_TEST_SPOT" type="ENHS" name="BADI_TEST_SPOT"/>
          <badiDefinition uri="/sap/bc/adt/enhancements/enhsxs/BADI_TEST_SPOT#BADI_TEST_DEF" type="BADI" name="BADI_TEST_DEF"/>
          <implementingClass uri="/sap/bc/adt/oo/classes/ZCL_TEST_IMPL" type="CLAS" name="ZCL_TEST_IMPL"/>
        </badiImplementation>
        <badiImplementation name="IMPL_TWO" shortText="Second impl" isActive="false" isDefault="true">
          <enhancementSpot uri="/sap/bc/adt/enhancements/enhsxs/BADI_TEST_SPOT" type="ENHS" name="BADI_TEST_SPOT"/>
          <badiDefinition uri="/sap/bc/adt/enhancements/enhsxs/BADI_TEST_SPOT#BADI_TEST_DEF" type="BADI" name="BADI_TEST_DEF"/>
          <implementingClass uri="/sap/bc/adt/oo/classes/ZCL_TEST_IMPL2" type="CLAS" name="ZCL_TEST_IMPL2"/>
        </badiImplementation>
      </badiImplementations>
    </badiTechnology>
  </contentSpecific>
</objectData>`)

	result, err := parseEnhancementImplXML(data)
	if err != nil {
		t.Fatalf("parseEnhancementImplXML: %v", err)
	}
	if result.Name != "TEST_IMPL" {
		t.Errorf("name: got %q", result.Name)
	}
	if result.Description != "Test Implementation" {
		t.Errorf("description: got %q", result.Description)
	}
	if result.Package != "TEST_PKG" {
		t.Errorf("package: got %q", result.Package)
	}
	if len(result.Implementations) != 2 {
		t.Fatalf("expected 2 implementations, got %d", len(result.Implementations))
	}

	impl := result.Implementations[0]
	if impl.Name != "IMPL_ONE" {
		t.Errorf("impl[0] name: got %q", impl.Name)
	}
	if !impl.IsActive {
		t.Error("impl[0] should be active")
	}
	if impl.IsDefault {
		t.Error("impl[0] should not be default")
	}
	if impl.ImplementingClass.Name != "ZCL_TEST_IMPL" {
		t.Errorf("impl[0] class: got %q", impl.ImplementingClass.Name)
	}
	if impl.EnhancementSpot.Name != "BADI_TEST_SPOT" {
		t.Errorf("impl[0] spot: got %q", impl.EnhancementSpot.Name)
	}

	impl2 := result.Implementations[1]
	if impl2.IsActive {
		t.Error("impl[1] should not be active")
	}
	if !impl2.IsDefault {
		t.Error("impl[1] should be default")
	}
}

func TestParseEnhancementImplXML_Empty(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<objectData name="EMPTY_IMPL" description="Empty">
  <packageRef name="PKG"/>
  <contentSpecific>
    <badiTechnology>
      <badiImplementations/>
    </badiTechnology>
  </contentSpecific>
</objectData>`)

	result, err := parseEnhancementImplXML(data)
	if err != nil {
		t.Fatalf("parseEnhancementImplXML: %v", err)
	}
	if len(result.Implementations) != 0 {
		t.Errorf("expected 0 implementations, got %d", len(result.Implementations))
	}
}
