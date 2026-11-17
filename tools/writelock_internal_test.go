package tools

import "testing"

func TestLockTargetURI(t *testing.T) {
	const obj = "/sap/bc/adt/oo/classes/zcl_625"
	for in, want := range map[string]string{
		obj:                                   obj,
		obj + "/":                             obj,
		obj + "/source/main":                  obj,
		obj + "/SOURCE/MAIN":                  obj,
		obj + "/source/main/":                 obj,
		obj + "/source/main#start=3,1":        obj,
		obj + "/source/main?version=inactive": obj,
		obj + "/includes/testclasses":         obj + "/includes/testclasses",
		"/sap/bc/adt/programs/programs/zp":    "/sap/bc/adt/programs/programs/zp",
	} {
		if got := lockTargetURI(in); got != want {
			t.Errorf("lockTargetURI(%q) = %q, want %q", in, got, want)
		}
	}
}
