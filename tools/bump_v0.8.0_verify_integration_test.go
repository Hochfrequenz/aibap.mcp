//go:build integration

// Throwaway reproducer harness for the adtler v0.7.0 -> v0.8.0 bump.
// Delete after the bump PR merges.
//
// Run:
//
//	go test -tags integration -v -count=1 -run BumpVerify ./tools/...
//
// Each test mirrors the reproducer in the linked issue body. Both are
// read-only and observe the fix on every run.
package tools_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// #568: a source URI (…/source/main) must resolve to the same source as the
// bare object URI instead of 404ing on …/source/main/source/main.
func TestBumpVerify_568_SourceURIAccepted(t *testing.T) {
	const bare = "/sap/bc/adt/programs/programs/z_adt_mcp_test_report"

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			requireFixture(t, sharedServer, sys, bare)

			sources := make([]string, 0, 2)
			for _, uri := range []string{bare, bare + "/source/main"} {
				res := callTool(t, sharedServer, "get_source", map[string]interface{}{
					"object_uri": uri,
				})
				if res.IsError {
					t.Fatalf("get_source(%s) returned IsError=true: %s", uri, textOf(res))
				}
				var payload struct {
					Source string `json:"source"`
				}
				if err := json.Unmarshal([]byte(textOf(res)), &payload); err != nil {
					t.Fatalf("unmarshal get_source(%s): %v\nraw: %s", uri, err, textOf(res))
				}
				if payload.Source == "" {
					t.Fatalf("get_source(%s) returned empty source", uri)
				}
				sources = append(sources, payload.Source)
			}
			if sources[0] != sources[1] {
				t.Errorf("bare and source URI returned different source (%d vs %d bytes)", len(sources[0]), len(sources[1]))
			}
		})
	}
}

// #555: a SQL line longer than 255 characters must not be truncated. The
// padding places "OR TABNAME = 'T001'" after character 255; truncated, the
// query silently returns only T000.
func TestBumpVerify_555_LongSQLLineNotTruncated(t *testing.T) {
	sql := "SELECT TABNAME FROM DD02L WHERE AS4LOCAL = 'A' AND TABNAME = 'T000'" +
		strings.Repeat(" ", 203) + "OR TABNAME = 'T001'"
	if len(sql) != 289 {
		t.Fatalf("reproducer SQL has %d characters, want 289 as in the issue", len(sql))
	}

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "run_query", map[string]interface{}{
				"purpose": "ddic_inspection",
				"sql":     sql,
			})
			if res.IsError {
				t.Fatalf("run_query returned IsError=true: %s", textOf(res))
			}
			text := textOf(res)
			for _, want := range []string{"T000", "T001"} {
				if !strings.Contains(text, `"`+want+`"`) {
					t.Errorf("result lacks row %s (OR condition dropped?): %s", want, text)
				}
			}
		})
	}
}
