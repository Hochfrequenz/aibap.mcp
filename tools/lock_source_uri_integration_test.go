//go:build integration

package tools_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestIntegration_SourceMainURI_LockFollowsTheObject reproduces #625 live: a
// write tool called with an object's main-source URI must leave its lock where
// unlock_object and activate_objects, called with the object URI, find it.
// Before the fix the lock was tracked under ".../source/main", unlock_object
// with the object URI answered "no lock tracked", and activation failed with
// 403 "currently editing".
//
// It works on a throwaway $TMP program with a new name per run and deletes
// it again.
func TestIntegration_SourceMainURI_LockFollowsTheObject(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			name := fmt.Sprintf("ZAIBAP_625_%06X", time.Now().UnixNano()&0xFFFFFF)
			objectURI := "/sap/bc/adt/programs/programs/" + strings.ToLower(name)
			sourceURI := objectURI + "/source/main"

			res := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "PROG", "name": name, "package": "$TMP", "description": "aibap.mcp #625 guard",
			})
			if res.IsError {
				t.Fatalf("create_object: %s", textOf(res))
			}
			t.Cleanup(func() {
				// Without the fix the lock is tracked under the source URI and
				// would block the delete; a no-op otherwise.
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": sourceURI})
				if r := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": objectURI}); r.IsError {
					t.Errorf("cleanup: delete_object left the $TMP program behind: %s", textOf(r))
				}
			})

			patch := func(comment string) {
				t.Helper()
				r := callTool(t, sharedServer, "patch_source", map[string]interface{}{
					"object_uri": sourceURI,
					"operations": []interface{}{map[string]interface{}{
						// after_line 0: before the first line, valid for any source.
						"type": "insert", "after_line": 0, "content": "* " + comment,
					}},
				})
				if r.IsError {
					t.Fatalf("patch_source with the source URI: %s", textOf(r))
				}
			}

			// Step 1 and 2 of the issue: write through the source URI, unlock
			// through the object URI.
			patch("first write")
			if r := callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": objectURI}); r.IsError {
				t.Fatalf("unlock_object with the object URI after patch_source: %s", textOf(r))
			}

			// Step 3: write again, then activate through the object URI. The
			// activation releases the tool's own lock first.
			patch("second write")
			r := callTool(t, sharedServer, "activate_objects", map[string]interface{}{"object_uris": []interface{}{objectURI}})
			if r.IsError {
				t.Fatalf("activate_objects with the object URI after patch_source: %s", textOf(r))
			}
			var act struct{ Success bool }
			if err := json.Unmarshal([]byte(textOf(r)), &act); err != nil || !act.Success {
				t.Fatalf("activation did not succeed: %s", textOf(r))
			}
		})
	}
}
