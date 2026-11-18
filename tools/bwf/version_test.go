package bwf

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

var testToday = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

func hdrs(task string, versions ...string) []Row {
	var out []Row
	for _, v := range versions {
		out = append(out, Row{"WFD_ID": task, "TASK": task, "VERSION": v, "LANGUAGE": "E"})
	}
	return out
}

func hrs(objid string, vs ...[2]string) []Row {
	var out []Row
	for _, v := range vs {
		out = append(out, Row{"OTYPE": "WS", "OBJID": objid, "WFD_ID": "WS" + objid, "VERSION": v[0], "ACTIVE": v[1]})
	}
	return out
}

func TestResolveVersion_HRS1205Active(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"HRS1205":    hrs("00400012", [2]string{"0003", "X"}),
		"SWDSHEADER": hdrs("WS00400012", "0000", "0001", "0002", "0003", "9999"),
	}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0003" || got.Source != "HRS1205" || got.Tables != "SWDS" || len(got.Warnings) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestResolveVersion_HRS1205MultipleRowsOneActive(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"HRS1205":    hrs("00400012", [2]string{"0000", ""}, [2]string{"0001", ""}, [2]string{"0003", "X"}),
		"SWDSHEADER": hdrs("WS00400012", "0000", "0001", "0003"),
	}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil || got.Version != "0003" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolveVersion_HRS1205TwoActive(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"HRS1205":    hrs("00400012", [2]string{"0001", "X"}, [2]string{"0002", "X"}),
		"SWDSHEADER": hdrs("WS00400012", "0001", "0002"),
	}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil || got.Version != "0002" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "0001") || !strings.Contains(got.Warnings[0], "0002") {
		t.Errorf("warnings = %v", got.Warnings)
	}
}

func TestResolveVersion_Active9999(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"HRS1205":    hrs("00400012", [2]string{"9999", "X"}),
		"SWDSHEADER": hdrs("WS00400012", "0000", "9999"),
	}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil || got.Version != "9999" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolveVersion_HRP1205(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{
		"HRP1205": {
			{"OTYPE": "WS", "OBJID": "00400012", "WFD_ID": "WS00400012", "VERSION": "0002", "ACTIVE": "X", "BEGDA": "20200101", "ENDDA": "99991231"},
			{"OTYPE": "WS", "OBJID": "00400012", "WFD_ID": "WS00400012", "VERSION": "0001", "ACTIVE": "X", "BEGDA": "20100101", "ENDDA": "20191231"},
		},
		"SWDSHEADER": hdrs("WS00400012", "0001", "0002"),
	}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil || got.Version != "0002" || got.Source != "HRP1205" || len(got.Warnings) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolveVersion_LatestNumbered(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{"SWDSHEADER": hdrs("WS00400012", "0000", "0004", "9999")}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil || got.Version != "0004" || got.Source != "latest_numbered" || len(got.Warnings) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestResolveVersion_Requested(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{"SWDSHEADER": hdrs("WS00400012", "0000", "0001")}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "0001", testToday)
	if err != nil || got.Version != "0001" || got.Source != "requested" {
		t.Errorf("got %+v, %v", got, err)
	}
	_, err = resolveVersion(context.Background(), f, "WS00400012", "0007", testToday)
	if err == nil || !strings.Contains(err.Error(), "0007") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveVersion_LegacyClient(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{"SWD_HEADER": {
		{"WFD_ID": "0040000064", "TASK": "WS00400012", "EXETYP": "S", "VERSION": "0001"},
		{"WFD_ID": "0040000064", "TASK": "WS00400012", "EXETYP": "S", "VERSION": "0000"},
	}}}
	got, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if err != nil {
		t.Fatal(err)
	}
	if got.WfdID != "0040000064" || got.Tables != "SWD" || got.Source != "legacy_client" || got.Version != "0001" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveVersion_NotFound(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{}}
	_, err := resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "SWDSHEADER, SWD_HEADER") {
		t.Errorf("message = %v", err)
	}
}

func TestResolveRuntime(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{"SWD_HEADER": {
		{"WFD_ID": "WS48900002", "TASK": "WS48900002", "VERSION": "0000", "EXETYP": "E"},
	}}}
	id, v, cfg, err := resolveRuntime(context.Background(), f, "WS48900002")
	if err != nil || id != "WS48900002" || v != "0000" || cfg {
		t.Errorf("got %q %q %v %v", id, v, cfg, err)
	}
	f.tables["SWD_HEADER"] = append(f.tables["SWD_HEADER"], Row{"WFD_ID": "WS48900002", "TASK": "WS48900002", "VERSION": "0000", "EXETYP": "C"})
	_, _, cfg, err = resolveRuntime(context.Background(), f, "WS48900002")
	if err != nil || !cfg {
		t.Errorf("hasConfig = %v, %v", cfg, err)
	}
}

func TestResolveVersion_WfdIDDiffersFromTask(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{"SWDSHEADER": {
		{"WFD_ID": "WS00000001", "TASK": "WS99999999", "VERSION": "0002", "LANGUAGE": "E"},
	}}}
	got, err := resolveVersion(context.Background(), f, "WS99999999", "", testToday)
	if err != nil || got.WfdID != "WS00000001" || got.Version != "0002" {
		t.Errorf("got %+v, %v", got, err)
	}
}

var bareTaskRe = regexp.MustCompile(`(?i)(^|[^~\w])task\s*=`)

func TestResolveQueriesQualifyTask(t *testing.T) {
	f := &fakeQuery{tables: map[string][]Row{}}
	_, _ = resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	f.tables["SWDSHEADER"] = hdrs("WS00400012", "0001")
	_, _ = resolveVersion(context.Background(), f, "WS00400012", "", testToday)
	_, _, _, _ = resolveRuntime(context.Background(), f, "WS00400012")
	if len(f.calls) == 0 {
		t.Fatal("no queries recorded")
	}
	sawHeader := false
	for _, sql := range f.calls {
		if strings.Contains(strings.ToLower(sql), "swd_header") {
			sawHeader = true
		}
		if bareTaskRe.MatchString(sql) {
			t.Errorf("unqualified task column in: %s", sql)
		}
	}
	if !sawHeader {
		t.Error("no swd_header query recorded")
	}
}
