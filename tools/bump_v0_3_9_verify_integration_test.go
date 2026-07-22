//go:build integration && transport

// Bump-verify reproducer for adtler v0.3.9 (aibap.mcp#443): creating a class /
// interface and writing its source through the MCP tools must succeed (was 403
// "currently editing"). Delete after the bump PR merges.
//
//	MCP_INTEGRATION_SYSTEMS="HF S/4 Mandant 100" \
//	  go test -tags 'integration transport' -run TestBumpVerify ./tools/...
package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBumpVerify_443_OOCreateWrite(t *testing.T) {
	mustSelectSystem(t, sharedServer, "HF S/4 Mandant 100")
	trR := callTool(t, sharedServer, "create_transport", map[string]interface{}{
		"category": "K", "description": "aibap #443 v0.3.9 bump verify", "package": "Z_ADT_MCP_TEST"})
	if trR.IsError {
		t.Fatalf("create_transport: %s", textOf(trR))
	}
	var tr struct {
		TransportNumber string `json:"transport_number"`
	}
	_ = json.Unmarshal([]byte(textOf(trR)), &tr)

	cases := []struct{ typ, name, uri, src string }{
		{"CLAS", "ZCL_ADT_MCP_V39", "/sap/bc/adt/oo/classes/zcl_adt_mcp_v39",
			"CLASS zcl_adt_mcp_v39 DEFINITION PUBLIC FINAL CREATE PUBLIC.\n  PUBLIC SECTION.\nENDCLASS.\nCLASS zcl_adt_mcp_v39 IMPLEMENTATION.\nENDCLASS.\n"},
		{"INTF", "ZIF_ADT_MCP_V39", "/sap/bc/adt/oo/interfaces/zif_adt_mcp_v39",
			"INTERFACE zif_adt_mcp_v39 PUBLIC.\n  METHODS noop.\nENDINTERFACE.\n"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.typ, func(t *testing.T) {
			cr := callTool(t, sharedServer, "create_object", map[string]interface{}{
				"object_type": c.typ, "name": c.name, "package": "Z_ADT_MCP_TEST",
				"description": "bump verify", "transport": tr.TransportNumber})
			if cr.IsError {
				if er := callTool(t, sharedServer, "object_exists", map[string]interface{}{"object_uri": c.uri}); er.IsError {
					t.Fatalf("create_object(%s): %s", c.typ, textOf(cr))
				}
				t.Logf("%s exists, reusing", c.name)
			}
			t.Cleanup(func() {
				_ = callTool(t, sharedServer, "unlock_object", map[string]interface{}{"object_uri": c.uri})
			})
			f := filepath.Join(t.TempDir(), "src.abap")
			_ = os.WriteFile(f, []byte(c.src), 0o644)
			w := callTool(t, sharedServer, "set_source_from_file", map[string]interface{}{
				"object_uri": c.uri, "file_path": f, "transport": tr.TransportNumber})
			if w.IsError {
				t.Fatalf("REGRESSION #443: %s create->write via MCP failed: %s", c.typ, textOf(w))
			}
			t.Logf("#443 OK: %s create->write via MCP tools succeeded", c.typ)
		})
	}
}
