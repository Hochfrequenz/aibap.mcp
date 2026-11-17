//go:build integration

package tools_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegration_SetSourceFromFile_ClassInclude reproduces #621 live:
// set_source_from_file must write a class's test-class include from a file,
// byte for byte. Before the fix the ETag fetch on the include URI answered
// 404. The include holds a JSON escape inside an ABAP comment, which the
// inline set_include_source argument used to decode.
//
// It works on a throwaway $TMP class with a new name per run and deletes it.
func TestIntegration_SetSourceFromFile_ClassInclude(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			name := fmt.Sprintf("ZCL_AIBAP_621_%06X", time.Now().UnixNano()&0xFFFFFF)
			classURI := "/sap/bc/adt/oo/classes/" + strings.ToLower(name)

			res := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "CLAS", "name": name, "package": "$TMP", "description": "aibap.mcp #621 guard",
			})
			if res.IsError {
				t.Fatalf("create_object: %s", textOf(res))
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": classURI})
				if r := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": classURI}); r.IsError {
					t.Errorf("cleanup: delete_object left the $TMP class behind: %s", textOf(r))
				}
			})

			if r := callTool(t, sharedServer, "lock_object", map[string]interface{}{"object_uri": classURI}); r.IsError {
				t.Fatalf("lock_object: %s", textOf(r))
			}
			if r := callTool(t, sharedServer, "create_test_include", map[string]interface{}{"object_uri": classURI}); r.IsError {
				t.Fatalf("create_test_include: %s", textOf(r))
			}

			source := "\"! \\u00fc must stay six characters\n" +
				"CLASS ltcl_621 DEFINITION FINAL FOR TESTING DURATION SHORT RISK LEVEL HARMLESS.\n" +
				"ENDCLASS.\n\n" +
				"CLASS ltcl_621 IMPLEMENTATION.\n" +
				"ENDCLASS.\n"
			file := filepath.Join(t.TempDir(), "testclasses.abap")
			if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			r := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": classURI + "/includes/testclasses",
				"file_path":  file,
			})
			if r.IsError {
				t.Fatalf("set_source_from_file with the include URI: %s", textOf(r))
			}

			r = callTool(t, sharedServer, "get_include_source", map[string]interface{}{"object_uri": classURI, "include": "testclasses"})
			if r.IsError {
				t.Fatalf("get_include_source: %s", textOf(r))
			}
			var got struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal([]byte(textOf(r)), &got); err != nil {
				t.Fatalf("parse get_include_source: %v", err)
			}
			if firstLine, _, _ := strings.Cut(source, "\n"); !strings.Contains(got.Source, firstLine) {
				t.Errorf("include read back without the literal escape:\n%s", got.Source)
			}
		})
	}
}
