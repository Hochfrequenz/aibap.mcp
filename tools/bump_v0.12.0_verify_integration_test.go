//go:build integration

// Delete after the bump PR merges.
//
// Reproducer harness for the adtler v0.12.0 bump (tracker #612): #610's
// reproducer plus live smoke checks of the four tool surfaces the bump had to
// adapt (create_transport, get_transport_requests, add_to_transport;
// release_transport is covered by unit tests only, because a live release
// cannot be undone). These create and delete transport requests.
//
//	go test -tags integration -count=1 -v -run BumpVerify ./tools/...
package tools_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// eccSystem is the integration alias of the ECC system, which has no
// transport organizer create/add-object actions.
const eccSystem = "hfq"

func bumpDeleteTransport(t *testing.T, tr string) {
	t.Helper()
	if r := callTool(t, sharedServer, "delete_transport", map[string]interface{}{"transport": tr}); r.IsError {
		t.Errorf("CLEANUP FAILED, delete %s by hand: %s", tr, textOf(r))
	}
}

func bumpCreateTransport(t *testing.T, args map[string]interface{}) (string, string, bool) {
	t.Helper()
	r := callTool(t, sharedServer, "create_transport", args)
	if r.IsError {
		return "", textOf(r), false
	}
	var out struct {
		TransportNumber string `json:"transport_number"`
	}
	if err := json.Unmarshal([]byte(textOf(r)), &out); err != nil || out.TransportNumber == "" {
		t.Fatalf("create_transport: no transport number in %s", textOf(r))
	}
	return out.TransportNumber, "", true
}

// #610: create_transport with category T and a target creates a transport of
// copies on S/4 and is refused as not supported on ECC.
func TestBumpVerify_610_CreateTransportOfCopies(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			tr, errText, ok := bumpCreateTransport(t, map[string]interface{}{
				"category": "T", "description": "aibap.mcp#610 reproducer", "target": "DUM",
			})
			if sys == eccSystem {
				if ok {
					bumpDeleteTransport(t, tr)
					t.Fatalf("ECC: expected not-supported, got request %s", tr)
				}
				if !strings.Contains(errText, "cannot create a request of type T over ADT") {
					t.Fatalf("ECC: expected adtler's not-supported error, got: %s", errText)
				}
				t.Logf("ECC: %s", errText)
				return
			}
			if !ok {
				t.Fatalf("S/4: create_transport failed: %s", errText)
			}
			defer bumpDeleteTransport(t, tr)
			q := callTool(t, sharedServer, "run_query", map[string]interface{}{
				"purpose": "transport_tracking",
				"sql":     "SELECT TRKORR, TRFUNCTION, TARSYSTEM FROM E070 WHERE TRKORR = '" + tr + "'",
			})
			if q.IsError {
				t.Fatalf("E070 read: %s", textOf(q))
			}
			var res struct{ Rows [][]string }
			_ = json.Unmarshal([]byte(textOf(q)), &res)
			if len(res.Rows) != 1 || res.Rows[0][1] != "T" || res.Rows[0][2] != "DUM" {
				t.Fatalf("S/4: E070 should show TRFUNCTION=T TARSYSTEM=DUM, got %v", res.Rows)
			}
			t.Logf("S/4: E070 %v", res.Rows[0][1:])
		})
	}
}

// get_transport_requests: options reach adtler, states are strings, paging works.
func TestBumpVerify_GetTransportRequests(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			type page struct {
				Count      int `json:"count"`
				Transports []struct {
					Number, State, RawStatus string
				} `json:"transports"`
				Next    string `json:"next"`
				Warning string `json:"warning"`
			}
			get := func(args map[string]interface{}) page {
				r := callTool(t, sharedServer, "get_transport_requests", args)
				if r.IsError {
					t.Fatalf("get_transport_requests %v: %s", args, textOf(r))
				}
				var p page
				if err := json.Unmarshal([]byte(textOf(r)), &p); err != nil {
					t.Fatalf("decode: %v in %s", err, textOf(r))
				}
				if p.Warning != "" {
					t.Logf("partial page warning: %s", p.Warning)
				}
				return p
			}
			p1 := get(map[string]interface{}{"user": "*", "status": "released", "limit": 3})
			if p1.Count == 0 || p1.Count > 3 {
				t.Fatalf("page 1: want 1..3 requests, got %d", p1.Count)
			}
			raw := textOf(callTool(t, sharedServer, "get_transport_requests", map[string]interface{}{"user": "*", "status": "released", "limit": 3}))
			if !strings.Contains(raw, `"state":"released"`) {
				t.Errorf("expected state as a string, got %s", raw)
			}
			if p1.Next == "" {
				t.Fatal("page 1: expected a next cursor for a system with more than 3 released requests")
			}
			p2 := get(map[string]interface{}{"user": "*", "status": "released", "limit": 3, "after": p1.Next})
			if p2.Count == 0 {
				t.Fatal("page 2 empty")
			}
			for _, a := range p1.Transports {
				for _, b := range p2.Transports {
					if a.Number == b.Number {
						t.Errorf("request %s on both pages", a.Number)
					}
				}
			}
			mine := get(map[string]interface{}{"status": "modifiable"})
			t.Logf("released page1=%d page2=%d; own modifiable=%d", p1.Count, p2.Count, mine.Count)
		})
	}
}

// add_to_transport takes an E071 key: records it on S/4, refused on ECC.
func TestBumpVerify_AddToTransport(t *testing.T) {
	const prog = "Z_ADT_MCP_ROLLBACK_B"
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)
			tr, errText, ok := bumpCreateTransport(t, map[string]interface{}{
				"category": "K", "description": "aibap.mcp adtler v0.12.0 add_to_transport check", "package": "Z_ADT_MCP_TEST",
			})
			if !ok {
				t.Fatalf("create_transport K: %s", errText)
			}
			defer bumpDeleteTransport(t, tr)

			r := callTool(t, sharedServer, "add_to_transport", map[string]interface{}{
				"object_type": "PROG", "object_name": prog, "transport": tr,
			})
			if sys == eccSystem {
				if !r.IsError {
					t.Fatalf("ECC: expected not-supported, got %s", textOf(r))
				}
				t.Logf("ECC: %s", textOf(r))
				return
			}
			if r.IsError {
				t.Fatalf("S/4: add_to_transport: %s", textOf(r))
			}
			o := callTool(t, sharedServer, "get_transport_objects", map[string]interface{}{"transport": tr})
			var objs struct {
				Objects []struct {
					PgmID    string `json:"pgmid"`
					Type     string `json:"type"`
					Name     string `json:"name"`
					WBType   string `json:"wb_type"`
					Position string `json:"position"`
					Task     string `json:"task"`
				} `json:"objects"`
			}
			_ = json.Unmarshal([]byte(textOf(o)), &objs)
			found := false
			for _, ob := range objs.Objects {
				if ob.Name != prog {
					continue
				}
				found = true
				task := ob.Task
				if task == "" {
					task = tr
				}
				if rr := callTool(t, sharedServer, "remove_from_transport", map[string]interface{}{
					"task_number": task, "parent_transport": tr, "pgmid": ob.PgmID, "object_type": ob.Type,
					"object_name": ob.Name, "wb_type": ob.WBType, "position": ob.Position,
				}); rr.IsError {
					t.Errorf("CLEANUP: remove_from_transport: %s", textOf(rr))
				}
			}
			if !found {
				t.Fatalf("S/4: %s not listed after add: %s", prog, textOf(o))
			}
		})
	}
}
