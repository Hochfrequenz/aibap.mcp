package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

// #625: a write tool called with an object's main-source URI must take and
// track its lock under the object URI, where unlock_object and
// activate_objects, called with the object URI, look for it.

const (
	classURI625       = "/sap/bc/adt/oo/classes/zcl_625"
	classSourceURI625 = classURI625 + "/source/main"
)

// lockRecorder is a mockClient whose lock calls are recorded.
type lockRecorder struct {
	locked   []string
	unlocked []string
}

func (r *lockRecorder) client() *mockClient {
	return &mockClient{
		getSourceFn: func(context.Context, string) (*adt.SourceResult, error) {
			return &adt.SourceResult{Source: "CLASS zcl_625 DEFINITION.\nENDCLASS.", ETag: `"e1"`}, nil
		},
		lockObjectFn: func(_ context.Context, uri string) (string, error) {
			r.locked = append(r.locked, uri)
			return "handle-625", nil
		},
		unlockObjectFn: func(_ context.Context, uri, _ string) error {
			r.unlocked = append(r.unlocked, uri)
			return nil
		},
	}
}

func TestPatchSource_SourceMainURI_UnlockWithObjectURI(t *testing.T) {
	rec := &lockRecorder{}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(rec.client(), lockMap)

	result := callTool(t, s, "patch_source", map[string]interface{}{
		"object_uri": classSourceURI625,
		"operations": []interface{}{
			map[string]interface{}{"type": "search_replace", "search": "DEFINITION", "replace": "DEFINITION PUBLIC"},
		},
	})
	if result.IsError {
		t.Fatalf("patch_source: %s", firstText(result))
	}
	if len(rec.locked) != 1 || rec.locked[0] != classURI625 {
		t.Fatalf("locked %v, want the object URI %s", rec.locked, classURI625)
	}

	result = callTool(t, s, "unlock_object", map[string]interface{}{"object_uri": classURI625})
	if result.IsError {
		t.Fatalf("unlock_object with the object URI: %s", firstText(result))
	}
	if len(rec.unlocked) != 1 || rec.unlocked[0] != classURI625 {
		t.Errorf("unlocked %v, want the object URI %s", rec.unlocked, classURI625)
	}
}

func TestSetSourceFromFile_SourceMainURI_ActivateReleasesTheLock(t *testing.T) {
	rec := &lockRecorder{}
	lockMap := adt.NewLockMap()
	s := newTestServerWithLockMap(rec.client(), lockMap)

	file := filepath.Join(t.TempDir(), "zcl_625.abap")
	if err := os.WriteFile(file, []byte("CLASS zcl_625 DEFINITION PUBLIC.\nENDCLASS."), 0o600); err != nil {
		t.Fatal(err)
	}
	result := callTool(t, s, "set_source_from_file", map[string]interface{}{
		"object_uri": classSourceURI625,
		"file_path":  file,
	})
	if result.IsError {
		t.Fatalf("set_source_from_file: %s", firstText(result))
	}

	result = callTool(t, s, "activate_objects", map[string]interface{}{"object_uris": []interface{}{classURI625}})
	if result.IsError {
		t.Fatalf("activate_objects: %s", firstText(result))
	}
	if len(rec.unlocked) != 1 || rec.unlocked[0] != classURI625 {
		t.Errorf("unlocked before activation %v, want the object URI %s", rec.unlocked, classURI625)
	}
	if _, held := lockMap.Get("dev:" + classURI625); held {
		t.Error("lock map still tracks the lock after activation")
	}
}

func TestLockObject_SourceMainURI_UnlockWithObjectURI(t *testing.T) {
	rec := &lockRecorder{}
	s := newTestServerWithLockMap(rec.client(), adt.NewLockMap())

	// Upper case and a fragment, as ADT itself hands source URIs out.
	result := callTool(t, s, "lock_object", map[string]interface{}{"object_uri": classURI625 + "/SOURCE/MAIN#start=1,0"})
	if result.IsError {
		t.Fatalf("lock_object: %s", firstText(result))
	}
	if len(rec.locked) != 1 || rec.locked[0] != classURI625 {
		t.Fatalf("locked %v, want the object URI %s", rec.locked, classURI625)
	}
	result = callTool(t, s, "unlock_object", map[string]interface{}{"object_uri": classURI625})
	if result.IsError {
		t.Fatalf("unlock_object with the object URI: %s", firstText(result))
	}
}
