package adt

import "testing"

const testPkgV2 = "application/vnd.sap.adt.packages.v2+xml"
const testPkgV1 = "application/vnd.sap.adt.packages.v1+xml"

func TestParseDiscovery(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<app:service xmlns:app="http://www.w3.org/2007/app" xmlns:atom="http://www.w3.org/2005/Atom">
  <app:workspace>
    <atom:title>Package</atom:title>
    <app:collection href="/sap/bc/adt/packages">
      <atom:title>Package</atom:title>
      <app:accept>application/vnd.sap.adt.packages.v2+xml</app:accept>
      <app:accept>application/vnd.sap.adt.packages.v1+xml</app:accept>
    </app:collection>
  </app:workspace>
  <app:workspace>
    <atom:title>Check</atom:title>
    <app:collection href="/sap/bc/adt/checkruns">
      <atom:title>Check</atom:title>
    </app:collection>
  </app:workspace>
</app:service>`

	result := parseDiscovery([]byte(xml))

	if len(result) != 1 {
		t.Fatalf("expected 1 endpoint with accepts, got %d", len(result))
	}

	accepts := result["/sap/bc/adt/packages"]
	if len(accepts) != 2 {
		t.Fatalf("expected 2 accepts for /sap/bc/adt/packages, got %d", len(accepts))
	}
	if accepts[0] != testPkgV2 {
		t.Errorf("first accept: got %q", accepts[0])
	}
	if accepts[1] != testPkgV1 {
		t.Errorf("second accept: got %q", accepts[1])
	}
}

func TestNegotiateContentType(t *testing.T) {
	c := &httpClient{
		discovery: map[string][]string{
			"/sap/bc/adt/packages": {
				testPkgV2,
				testPkgV1,
			},
		},
	}

	// Prefer v2, system has v2
	got := c.NegotiateContentType("/sap/bc/adt/packages",
		[]string{testPkgV2, testPkgV1},
		"application/xml")
	if got != testPkgV2 {
		t.Errorf("expected v2, got %q", got)
	}

	// Endpoint not in discovery → fallback
	got = c.NegotiateContentType("/sap/bc/adt/unknown",
		[]string{"application/vnd.sap.adt.foo.v2+xml"},
		"application/xml")
	if got != "application/xml" {
		t.Errorf("expected fallback, got %q", got)
	}

	// Prefer v3 but system only has v2/v1 → should pick v2 as second choice
	got = c.NegotiateContentType("/sap/bc/adt/packages",
		[]string{"application/vnd.sap.adt.packages.v3+xml", testPkgV2},
		"application/xml")
	if got != testPkgV2 {
		t.Errorf("expected v2 fallback, got %q", got)
	}
}

func TestAcceptHeaderForURI_UsesDiscovery(t *testing.T) {
	// System only supports v1 for packages (older ECC)
	c := &httpClient{
		discovery: map[string][]string{
			"/sap/bc/adt/packages": {testPkgV1},
		},
	}

	// Hardcoded default is v2, but discovery only has v1 → should use v1
	got := c.acceptHeaderForURI("/sap/bc/adt/packages/Z_MY_PKG")
	if got != testPkgV1+", application/xml" {
		t.Errorf("expected v1 from discovery, got %q", got)
	}
}

func TestAcceptHeaderForURI_FallsBackToHardcoded(t *testing.T) {
	// No discovery data → use hardcoded defaults
	c := &httpClient{}

	got := c.acceptHeaderForURI("/sap/bc/adt/packages/Z_MY_PKG")
	want := "application/vnd.sap.adt.packages.v2+xml, application/xml"
	if got != want {
		t.Errorf("expected hardcoded fallback %q, got %q", want, got)
	}
}

func TestAcceptHeaderForURI_UnknownURI(t *testing.T) {
	c := &httpClient{}
	got := c.acceptHeaderForURI("/sap/bc/adt/something/unknown")
	if got != "application/xml" {
		t.Errorf("expected generic xml, got %q", got)
	}
}
