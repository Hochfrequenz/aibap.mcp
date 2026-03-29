package adt

import (
	"testing"
)

func TestParseVersionFeed(t *testing.T) {
	feed := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>00001</id>
    <updated>2025-01-15T10:30:00Z</updated>
    <title>Version 1</title>
    <author><name>DEVELOPER</name></author>
    <content src="/sap/bc/adt/programs/programs/ZTEST/source/main/versions/20250115103000/00001/content"/>
    <link rel="http://www.sap.com/adt/relations/trans_req" href="Transport for change" name="S4DK900042"/>
  </entry>
  <entry>
    <id>00002</id>
    <updated>2025-01-16T14:00:00Z</updated>
    <title>Version 2</title>
    <author><name>ADMIN</name></author>
    <content src="/sap/bc/adt/programs/programs/ZTEST/source/main/versions/20250116140000/00002/content"/>
  </entry>
</feed>`)

	versions, err := parseVersionFeed(feed)
	if err != nil {
		t.Fatalf("parseVersionFeed: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	v1 := versions[0]
	if v1.VersionNumber != "00001" {
		t.Errorf("version number: got %q, want 00001", v1.VersionNumber)
	}
	if v1.Author != "DEVELOPER" {
		t.Errorf("author: got %q, want DEVELOPER", v1.Author)
	}
	if v1.Transport != "S4DK900042" {
		t.Errorf("transport: got %q, want S4DK900042", v1.Transport)
	}
	if v1.ContentURI != "/sap/bc/adt/programs/programs/ZTEST/source/main/versions/20250115103000/00001/content" {
		t.Errorf("content URI: got %q", v1.ContentURI)
	}
	if v1.Date != "2025-01-15T10:30:00Z" {
		t.Errorf("date: got %q", v1.Date)
	}

	v2 := versions[1]
	if v2.Transport != "" {
		t.Errorf("v2 transport: expected empty, got %q", v2.Transport)
	}
	if v2.TransportDesc != "Version 2" {
		t.Errorf("v2 transport desc fallback: got %q, want 'Version 2'", v2.TransportDesc)
	}
}

func TestParseVersionFeed_Empty(t *testing.T) {
	feed := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`)

	versions, err := parseVersionFeed(feed)
	if err != nil {
		t.Fatalf("parseVersionFeed: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions, got %d", len(versions))
	}
}
