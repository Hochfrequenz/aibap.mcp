package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// #621: set_source_from_file writes a class include URI through
// SetIncludeSource, with the file's bytes unchanged, under a lock on the class.
func TestSetSourceFromFile_ClassInclude(t *testing.T) {
	const classURI = "/sap/bc/adt/oo/classes/zcl_621"
	// A JSON escape inside an ABAP literal: inline JSON arguments decoded it,
	// a file must keep it byte for byte.
	const source = "CLASS ltcl DEFINITION FOR TESTING.\n  \" `M\\u00fcller`\nENDCLASS.\nCLASS ltcl IMPLEMENTATION.\nENDCLASS.\n"

	var locked []string
	var gotURI, gotInclude, gotSource, gotHandle string
	mock := &mockClient{
		getSourceFn: func(context.Context, string) (*adt.SourceResult, error) {
			t.Error("set_source_from_file fetched a main-source ETag for a class include")
			return &adt.SourceResult{}, nil
		},
		lockObjectFn: func(_ context.Context, uri string) (string, error) {
			locked = append(locked, uri)
			return "handle-621", nil
		},
		setSourceFn: func(context.Context, string, string, string, string, string) (string, error) {
			t.Error("set_source_from_file wrote a class include through SetSource")
			return "", nil
		},
		setIncludeSourceFn: func(_ context.Context, uri, include, src, lockHandle, _, _ string) (string, error) {
			gotURI, gotInclude, gotSource, gotHandle = uri, include, src, lockHandle
			return `"inc-etag"`, nil
		},
	}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(mock, lockMap)

	file := filepath.Join(t.TempDir(), "zcl_621.testclasses.abap")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": classURI + "/includes/testclasses",
		"file_path":  file,
	})
	if result.IsError {
		t.Fatalf("set_source_from_file: %s", firstText(result))
	}
	if gotURI != classURI || gotInclude != "testclasses" {
		t.Errorf("SetIncludeSource(%q, %q), want (%q, %q)", gotURI, gotInclude, classURI, "testclasses")
	}
	if gotSource != source {
		t.Errorf("source changed on the way:\n got %q\nwant %q", gotSource, source)
	}
	if len(locked) != 1 || locked[0] != classURI || gotHandle != "handle-621" {
		t.Errorf("locked %v with handle %q, want one lock on the class %s", locked, gotHandle, classURI)
	}
	if _, held := lockMap.Get("dev:" + classURI); !held {
		t.Error("the class lock is not tracked under the class URI")
	}
}
