//go:build integration

// Delete after the bump PR merges.
//
// Reproducers of the issues the adtler v0.11.0 bump closes (CLAUDE.md,
// "Cross-Repo Issue Tracking (adtler)", point 6). Logs counts and languages
// only, never object names.

package tools_test

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/Hochfrequenz/aibap.mcp/config"
)

// bumpQuery runs run_query and returns its rows.
func bumpQuery(t *testing.T, sql string) [][]string {
	t.Helper()
	res := callTool(t, sharedServer, "run_query", map[string]interface{}{"sql": sql, "max_rows": 10, "purpose": "development_metadata"})
	if res.IsError {
		t.Fatalf("run_query failed: %s", textOf(res))
	}
	var qr adt.QueryResult
	structuredInto(t, res, &qr)
	return qr.Rows
}

func bumpCount(t *testing.T, sql string) string {
	t.Helper()
	rows := bumpQuery(t, sql)
	if len(rows) != 1 || len(rows[0]) != 1 {
		t.Fatalf("COUNT(*) returned %d rows", len(rows))
	}
	return strings.TrimSpace(rows[0][0])
}

// #560: verify_source must not leave its temporary program behind.
func TestBumpVerify_560_VerifySourceLeavesNoProgram(t *testing.T) {
	const countSQL = "SELECT COUNT(*) FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'PROG' AND OBJ_NAME LIKE 'Z_ADTLER_VERIFY_%'"
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			before := bumpCount(t, countSQL)
			res := callTool(t, sharedServer, "verify_source", map[string]interface{}{"source": "REPORT ztest.\nWRITE / 'hello'."})
			if res.IsError {
				t.Fatalf("verify_source failed: %s", textOf(res))
			}
			after := bumpCount(t, countSQL)
			t.Logf("Z_ADTLER_VERIFY_* programs: %s before, %s after", before, after)
			if before != after {
				t.Errorf("verify_source left a program behind: %s before, %s after", before, after)
			}
		})
	}
}

// #595: object_exists must report a nonexistent VIT object as missing, and
// still report an existing one.
func TestBumpVerify_595_ObjectExistsVIT(t *testing.T) {
	const vitPrefix = "/sap/bc/adt/vit/wb/object_type/wdccyg/object_name/"
	exists := func(t *testing.T, uri string) bool {
		t.Helper()
		res := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": uri})
		if res.IsError {
			t.Fatalf("object_exists failed")
		}
		var out struct {
			Exists bool `json:"exists"`
		}
		structuredInto(t, res, &out)
		return out.Exists
	}
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			if exists(t, vitPrefix+"ZZZ_BUMPVERIFY_595_MISSING") {
				t.Errorf("object_exists reports a nonexistent WDCC object as existing")
			}
			rows := bumpQuery(t, "SELECT OBJ_NAME FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'WDCC' AND DELFLAG = ' '")
			if len(rows) == 0 {
				t.Log("no WDCC object on this system, existing-object control skipped")
				return
			}
			// SAP decodes a raw '+' as a blank, so it has to go out as %2B.
			name := strings.ReplaceAll(url.PathEscape(rows[0][0]), "+", "%2B")
			if !exists(t, vitPrefix+name) {
				t.Errorf("object_exists reports an existing WDCC object as missing")
			}
		})
	}
}

// #592: a new object gets the configured logon language as its original language.
func TestBumpVerify_592_ConfiguredLanguageBecomesOriginalLanguage(t *testing.T) {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		t.Fatalf("config.Load failed")
	}
	sapLang := map[string]string{"DE": "D", "EN": "E"}
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			want, ok := sapLang[strings.ToUpper(cfg.Systems[sys].Language)]
			if !ok {
				t.Skip("configured language is neither DE nor EN")
			}
			mustSelectSystem(t, sharedServer, sys)
			n, err := rand.Int(rand.Reader, big.NewInt(1000000))
			if err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("ZCL_AIBAP_BV592_%06d", n.Int64())
			res := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": "CLAS", "name": name, "package": "$TMP", "description": "bump verify 592",
			})
			if res.IsError {
				t.Fatalf("create_object failed: %s", strings.ReplaceAll(textOf(res), name, "<name>"))
			}
			t.Cleanup(func() {
				del := callTool(t, sharedServer, "delete_object", map[string]interface{}{"object_uri": "/sap/bc/adt/oo/classes/" + strings.ToLower(name)})
				if del.IsError {
					t.Errorf("cleanup: delete_object failed, a class is left in $TMP")
				}
			})
			master := bumpQuery(t, "SELECT MASTERLANG FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'CLAS' AND OBJ_NAME = '"+name+"'")
			texts := bumpQuery(t, "SELECT LANGU FROM SEOCLASSTX WHERE CLSNAME = '"+name+"'")
			var langs []string
			for _, r := range texts {
				langs = append(langs, strings.TrimSpace(r[0]))
			}
			got := ""
			if len(master) == 1 {
				got = strings.TrimSpace(master[0][0])
			}
			t.Logf("configured %s, MASTERLANG %q, description languages %v", want, got, langs)
			if got != want {
				t.Errorf("MASTERLANG = %q, want %q", got, want)
			}
			if len(langs) != 1 || langs[0] != want {
				t.Errorf("description languages = %v, want [%s]", langs, want)
			}
		})
	}
}
