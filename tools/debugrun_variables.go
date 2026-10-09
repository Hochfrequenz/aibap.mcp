package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// Table pages of debug_get_variable: the hard cap keeps the tool a debugging
// aid for a halted program, not a way to retrieve data (scope guardrail).
const (
	defaultTableRows = 20
	maxTableRows     = 100
)

// variableQuery is what debug_get_variable was asked for.
type variableQuery struct {
	name          string
	expand, table bool
	offset, limit int
}

func parseVariableQuery(args map[string]any) (variableQuery, error) {
	name, _ := args["variable_name"].(string)
	q := variableQuery{name: strings.TrimSpace(name)}
	if q.name == "" {
		return q, errors.New(`debug_get_variable: "variable_name" is required`)
	}
	if v, ok := args["expand"].(bool); ok {
		q.expand = v
	}
	off, hasOff := args["offset"]
	lim, hasLim := args["limit"]
	hasOff, hasLim = hasOff && off != nil, hasLim && lim != nil
	if hasOff || hasLim {
		q.table, q.offset, q.limit = true, 1, defaultTableRows
		if hasOff {
			n, ok := intArg(off)
			if !ok || n < 1 {
				return q, errors.New("debug_get_variable: offset must be a whole number of at least 1 (rows are 1-based)")
			}
			q.offset = n
		}
		if hasLim {
			n, ok := intArg(lim)
			if !ok || n < 1 || n > maxTableRows {
				return q, fmt.Errorf("debug_get_variable: limit must be a whole number from 1 to %d (hard cap per call)", maxTableRows)
			}
			q.limit = n
		}
	}
	if q.table && q.expand {
		return q, errors.New("debug_get_variable: use either expand or offset/limit, not both")
	}
	return q, nil
}

// readVariable answers debug_get_variable on the attached debug session.
func readVariable(ctx context.Context, d *adt.DebugSession, q variableQuery) (DebugVariableResult, error) {
	switch {
	case q.table:
		page, err := d.GetTableRows(ctx, q.name, q.offset, q.limit)
		if errors.Is(err, adt.ErrNotATable) {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: %s is not an internal table; drop offset/limit", q.name)
		}
		if err != nil {
			return DebugVariableResult{}, err
		}
		res := DebugVariableResult{VariableName: q.name, MetaType: "table",
			Table: &DebugTablePage{TotalLines: page.TotalLines, Offset: page.Offset, Rows: []DebugTableRow{}}}
		for _, r := range page.Rows {
			row := DebugTableRow{Index: r.Index, Fields: []DebugTableField{}}
			for _, f := range r.Fields {
				row.Fields = append(row.Fields, DebugTableField{Path: f.Path, Value: f.Value})
			}
			res.Table.Rows = append(res.Table.Rows, row)
		}
		return res, nil
	case q.expand:
		vars, err := d.GetVariables(ctx, q.name)
		if err != nil {
			return DebugVariableResult{}, err
		}
		meta := findVariable(vars, q.name)
		if meta == nil {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: unknown variable %q", q.name)
		}
		if meta.MetaType == "table" {
			return DebugVariableResult{}, fmt.Errorf("debug_get_variable: %s is an internal table; read its rows with offset/limit", q.name)
		}
		parent := meta.ID
		if meta.MetaType == "dataref" {
			parent += "->*"
		}
		kids, err := d.GetChildVariables(ctx, parent)
		if err != nil {
			return DebugVariableResult{}, err
		}
		return DebugVariableResult{VariableName: q.name, Value: meta.Value, MetaType: meta.MetaType, Children: childrenOf(kids, parent)}, nil
	}
	data, err := d.GetVariable(ctx, q.name)
	if err != nil {
		return DebugVariableResult{}, err
	}
	return buildDebugVariableResult(q.name, data), nil
}

// findVariable picks the entry answering a one-ID getVariables request: exact
// ID, else the only entry, else a case-insensitive match.
func findVariable(vars []adt.DebugVariable, name string) *adt.DebugVariable {
	for i := range vars {
		if vars[i].ID == name {
			return &vars[i]
		}
	}
	if len(vars) == 1 {
		return &vars[0]
	}
	for i := range vars {
		if strings.EqualFold(vars[i].ID, name) {
			return &vars[i]
		}
	}
	return nil
}

// childrenOf returns the children of parent in link order; without links,
// every returned variable.
func childrenOf(kids *adt.DebugChildVariables, parent string) []DebugVariableChild {
	byID := make(map[string]adt.DebugVariable, len(kids.Variables))
	for _, v := range kids.Variables {
		byID[v.ID] = v
	}
	var picked []adt.DebugVariable
	for _, l := range kids.Links {
		if v, ok := byID[l.ChildID]; ok && l.ParentID == parent {
			picked = append(picked, v)
		}
	}
	if len(picked) == 0 {
		picked = kids.Variables
	}
	out := make([]DebugVariableChild, 0, len(picked))
	for _, v := range picked {
		typ := v.ActualType
		if typ == "" {
			typ = v.DeclaredType
		}
		out = append(out, DebugVariableChild{ID: v.ID, Name: v.Name, MetaType: v.MetaType, Type: typ, Value: v.Value, TableLines: v.TableLines})
	}
	return out
}
