//go:build integration

// Delete after the bump PR merges.
//
// Reproducer harness for the adtler v0.5.4 bump, per CLAUDE.md point 6. The
// issue bodies remain the source of truth for each snippet; this file only
// makes them callable in one go:
//
//	go test -tags integration -run BumpVerify ./tools/...

package tools_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// hostInURL matches the scheme and authority of any URL in an error string.
// Go's transport errors quote the full request URL, which carries the
// internal host name and port, and this output is pasted into a public pull
// request. See adtler#167.
var hostInURL = regexp.MustCompile(`https?://[^/\s"]+`)

// searchOne returns the URI and name of one object of the given ADT type on
// the selected system, or skips when the system holds none. Fixtures are
// discovered rather than hardcoded, and only counts and kinds are logged.
//
// Names in a registered namespace are deliberately NOT filtered out here: the
// callers use the URI the system itself reports, never one built from a name,
// so encoding is not in play — and on the S/4 system every enhancement
// implementation is namespaced, so filtering would skip the whole check.
func searchOne(t *testing.T, sys, adtType string) (uri, name string) {
	t.Helper()
	res := callTool(t, sharedServer, "search_objects", map[string]interface{}{
		"query":       "*",
		"object_type": adtType,
		"max_results": 10,
	})
	if res.IsError {
		// A search that never returned is not a finding about this bump, but
		// it must not pass silently either: the ECC system does not answer
		// the enhancement search within the client's 30-second timeout.
		text := hostInURL.ReplaceAllString(textOf(res), "<host>")
		if strings.Contains(text, "deadline exceeded") || strings.Contains(text, "Timeout") {
			t.Skipf("search for %s did not return within the client timeout, so nothing could be measured: %s", adtType, text)
		}
		t.Fatalf("search_objects(%s) failed: %s", adtType, text)
	}
	var payload struct {
		Count   int `json:"count"`
		Results []struct {
			URI  string `json:"uri"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
		t.Fatalf("unmarshal search_objects result: %v", err)
	}
	t.Logf("%s: system holds %d %s object(s) in the first page", sys, payload.Count, adtType)
	for _, r := range payload.Results {
		if r.URI != "" && r.Name != "" {
			return r.URI, r.Name
		}
	}
	t.Skipf("no %s object on this system", adtType)
	return "", ""
}

// assertObjectInfoReadable calls get_object_info and requires a real answer.
//
// This is the request adtler#65 recorded as a dead end. ADT publishes one
// media type per object kind, so the old client's "application/xml" was
// refused with 406 — which reads like a missing endpoint rather than a wrong
// header. Before the bump this fails with
// "SAP ADT error 406 (ExceptionResourceNotAcceptable)"; after it, the client
// re-asks with */* and the object document comes back.
func assertObjectInfoReadable(t *testing.T, kind, uri, wantName string) {
	t.Helper()
	res := callTool(t, sharedServer, "get_object_info", map[string]interface{}{
		"object_uri": uri,
	})
	if res.IsError {
		// The object name is not printed: ADT error text echoes the requested
		// object back verbatim, and this output is pasted into a public pull
		// request (adtler#167).
		t.Fatalf("get_object_info on a %s returned IsError=true — the 406 is still there", kind)
	}
	var info struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		PackageName string `json:"package_name"`
	}
	if err := json.Unmarshal([]byte(textOf(res)), &info); err != nil {
		t.Fatalf("unmarshal get_object_info result for a %s: %v", kind, err)
	}
	// An empty name would mean the fallback let a non-object document through.
	if !strings.EqualFold(info.Name, wantName) {
		t.Fatalf("get_object_info for a %s returned a different object than requested (or none)", kind)
	}
	t.Logf("%s: object document read, type %s", kind, info.Type)
}

// TestBumpVerify_ServiceBindingObjectInfo is the direct consumer-side check of
// what adtler v0.5.4 changes: a service binding's object document was
// unreadable through this MCP because the request asked for the wrong media
// type.
func TestBumpVerify_ServiceBindingObjectInfo(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			uri, name := searchOne(t, sys, "SRVB/SVB")
			assertObjectInfoReadable(t, "service binding", uri, name)
		})
	}
}

// TestBumpVerify_UnmappedKindObjectInfo checks the part of the fix that
// outlives the three object types it was written for. An enhancement
// implementation is a kind adtler has no catalogue entry for at all, so it
// reaches the same 406 — and reads correctly after the bump only because the
// client stops guessing and lets the server choose. adtler#166 tracks giving
// the kind a real entry; this proves it is readable meanwhile.
func TestBumpVerify_UnmappedKindObjectInfo(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			uri, name := searchOne(t, sys, "ENHO")
			assertObjectInfoReadable(t, "enhancement implementation", uri, name)
		})
	}
}

// TestBumpVerify_RAPSourceStillReads guards against the bump regressing what
// already worked. Reading a behavior definition's and a service definition's
// source was fine before v0.5.4 — get_source never depended on the object-type
// catalogue — and #518 records that reading behavior definitions works end to
// end. Creating one is still blocked on adtler#148.
func TestBumpVerify_RAPSourceStillReads(t *testing.T) {
	for _, kind := range []struct{ label, adtType, keyword string }{
		{"behavior definition", "BDEF/BDO", "define behavior"},
		{"service definition", "SRVD/SRV", "define service"},
	} {
		t.Run(kind.label, func(t *testing.T) {
			for _, sys := range integrationSystems {
				t.Run(sys, func(t *testing.T) {
					requireReachable(t, sys)
					mustSelectSystem(t, sharedServer, sys)

					uri, _ := searchOne(t, sys, kind.adtType)
					res := callTool(t, sharedServer, "get_source", map[string]interface{}{
						"object_uri": uri,
					})
					if res.IsError {
						t.Fatalf("get_source on a %s returned IsError=true", kind.label)
					}
					var payload struct {
						Source string `json:"source"`
					}
					if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
						t.Fatalf("unmarshal get_source result: %v", err)
					}
					if !strings.Contains(strings.ToLower(payload.Source), kind.keyword) {
						t.Fatalf("%s source does not contain %q", kind.label, kind.keyword)
					}
					t.Logf("%s: read %d bytes of source", kind.label, len(payload.Source))
				})
			}
		})
	}
}
