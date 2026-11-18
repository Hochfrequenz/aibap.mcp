package bwf

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
)

func TestNormalizeTask(t *testing.T) {
	for _, in := range []string{"WS48900002", "ws48900002", " 48900002 "} {
		got, err := NormalizeTask(in)
		if err != nil || got != "WS48900002" {
			t.Errorf("NormalizeTask(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"WS4890000", "TS30000018", "WS4890000X", ""} {
		if _, err := NormalizeTask(in); err == nil {
			t.Errorf("NormalizeTask(%q) expected error", in)
		}
	}
}

type stubQuery struct {
	res     *adt.QueryResult
	maxRows int
}

func (s *stubQuery) RunQuery(_ context.Context, _ string, maxRows int) (*adt.QueryResult, error) {
	s.maxRows = maxRows
	return s.res, nil
}

func TestFetch_MapsRowsByColumn(t *testing.T) {
	q := &stubQuery{res: &adt.QueryResult{
		Columns:   []adt.QueryColumn{{Name: "NODEID"}, {Name: "NODETYPE"}},
		Rows:      [][]string{{"0000000001", "STRT"}},
		TotalRows: 1,
	}}
	got, err := fetch(context.Background(), q, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{{"NODEID": "0000000001", "NODETYPE": "STRT"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestFetch_TruncationIsError(t *testing.T) {
	q := &stubQuery{res: &adt.QueryResult{
		Columns:   []adt.QueryColumn{{Name: "A"}},
		Rows:      [][]string{{"1"}, {"2"}},
		TotalRows: 6000,
	}}
	if _, err := fetch(context.Background(), q, "x"); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("expected truncated error, got %v", err)
	}
	rows := make([][]string, 5000)
	for i := range rows {
		rows[i] = []string{"v"}
	}
	q = &stubQuery{res: &adt.QueryResult{Columns: []adt.QueryColumn{{Name: "A"}}, Rows: rows, TotalRows: 5000}}
	if _, err := fetch(context.Background(), q, "x"); err == nil {
		t.Error("expected error at exactly maxRows rows")
	}
}

func TestFetch_PassesMaxRows(t *testing.T) {
	q := &stubQuery{res: &adt.QueryResult{}}
	if _, err := fetch(context.Background(), q, "x"); err != nil {
		t.Fatal(err)
	}
	if q.maxRows != 5000 {
		t.Errorf("maxRows = %d", q.maxRows)
	}
}

func TestValidName(t *testing.T) {
	for _, s := range []string{"/IDXGC/CL_PD_WF_DOCUMENT", "GC_ABAP_TRUE"} {
		if !validName(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"X' OR '1", strings.Repeat("A", 31), ""} {
		if validName(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}
