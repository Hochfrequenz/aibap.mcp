package tools

import "testing"

// TestClassIncludeOf runs URIs through lockTargetURI first, as
// set_source_from_file does.
func TestClassIncludeOf(t *testing.T) {
	const class = "/sap/bc/adt/oo/classes/zcl_621"
	for _, tc := range []struct {
		uri, wantClass, wantInclude string
		ok                          bool
	}{
		{class + "/includes/testclasses", class, "testclasses", true},
		{class + "/INCLUDES/Definitions", class, "definitions", true},
		{class + "/includes/macros/", class, "macros", true},
		{class + "/includes/implementations#start=1,0", class, "implementations", true},
		{class, "", "", false},
		{class + "/source/main", "", "", false},
		{class + "/includes/", "", "", false},
		{"/sap/bc/adt/oo/classes//includes/testclasses", "", "", false},
		{class + "/includes/testclasses/extra", "", "", false},
		{"/sap/bc/adt/programs/programs/zp/includes/x", "", "", false},
	} {
		gotClass, gotInclude, ok := classIncludeOf(lockTargetURI(tc.uri))
		if ok != tc.ok || gotClass != tc.wantClass || gotInclude != tc.wantInclude {
			t.Errorf("classIncludeOf(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.uri, gotClass, gotInclude, ok, tc.wantClass, tc.wantInclude, tc.ok)
		}
	}
}
