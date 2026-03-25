//go:build integration

package adt_test

import (
	"context"
	"testing"
)

func TestBrowsePackage_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	results, err := client.BrowsePackage(ctx, "STUN")
	if err != nil {
		t.Fatalf("BrowsePackage failed: %v", err)
	}

	// #10: BrowsePackage must return objects (POST + asx:abap parsing works).
	if len(results) == 0 {
		t.Fatal("expected at least one object in package STUN, got 0")
	}
	t.Logf("got %d objects in package STUN", len(results))

	// Verify every returned object has essential fields populated.
	for i, obj := range results {
		if obj.Name == "" {
			t.Errorf("object [%d]: Name is empty", i)
		}
		if obj.Type == "" {
			t.Errorf("object [%d]: Type is empty", i)
		}
		if obj.URI == "" {
			t.Errorf("object [%d]: URI is empty", i)
		}
		if i < 5 {
			t.Logf("  [%d] %s %s %q", i, obj.Type, obj.Name, obj.Description)
		}
	}
}

func TestGetObjectInfo_Integration(t *testing.T) {
	client := newIntegrationClient(t)
	ctx := context.Background()

	// #13: Test multiple object types to verify type-specific Accept headers.
	tests := []struct {
		name        string
		uri         string
		wantType    string
		wantPackage string // empty = don't check
	}{
		{
			name:     "program",
			uri:      "/sap/bc/adt/programs/programs/RSPARAM",
			wantType: "PROG/P",
		},
		{
			name:     "test report fixture",
			uri:      testReportURI,
			wantType: "PROG/P",
		},
		{
			name:        "test class fixture",
			uri:         "/sap/bc/adt/oo/classes/ZCL_ADT_MCP_TEST_NOUNITS",
			wantType:    "CLAS/OC",
			wantPackage: "$TMP",
		},
		{
			name:        "test interface fixture",
			uri:         "/sap/bc/adt/oo/interfaces/ZIF_ADT_MCP_TEST",
			wantType:    "INTF/OI",
			wantPackage: "$TMP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := client.GetObjectInfo(ctx, tt.uri)
			if err != nil {
				t.Fatalf("GetObjectInfo(%s) failed: %v", tt.uri, err)
			}

			if info.Name == "" {
				t.Error("expected non-empty Name")
			}
			if info.Type == "" {
				t.Error("expected non-empty Type")
			}
			if tt.wantType != "" && info.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", info.Type, tt.wantType)
			}
			if tt.wantPackage != "" && info.PackageName != tt.wantPackage {
				t.Errorf("PackageName = %q, want %q", info.PackageName, tt.wantPackage)
			}
			t.Logf("name=%s type=%s description=%q package=%s", info.Name, info.Type, info.Description, info.PackageName)
		})
	}
}
