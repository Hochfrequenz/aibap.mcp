package bwf

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Hochfrequenz/adtler/adt"
)

const maxDepth = 5

// ErrInvalidTask marks a called workflow whose id is not a valid WS task id.
var ErrInvalidTask = errors.New("invalid workflow task id")

// ErrUnsupported marks a definition that exists but cannot be read yet.
var ErrUnsupported = errors.New("workflow definition not supported")

// Get reads the workflow definition for opts.Task and, up to opts.Depth
// levels, the sub-workflows it calls.
func Get(ctx context.Context, q adt.QueryClient, opts Options) (*Result, error) {
	task, err := NormalizeTask(opts.Task)
	if err != nil {
		return nil, err
	}
	opts.Task = task
	if opts.Language == "" {
		opts.Language = "E"
	}
	if opts.Today.IsZero() {
		opts.Today = time.Now()
	}
	opts.Depth = min(max(opts.Depth, 0), maxDepth)

	rootDef, err := buildDefinition(ctx, q, opts, task)
	if err != nil {
		return nil, err
	}
	res := &Result{Definition: *rootDef}
	root := &res.Definition

	type queued struct {
		def   *Definition
		depth int
	}
	var order []*Subworkflow
	index := map[string]*Subworkflow{}
	queue := []queued{{root, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= opts.Depth {
			continue
		}
		for _, c := range cur.def.Calls {
			if s, ok := index[c.WfdID]; ok {
				s.CalledBy = appendUnique(s.CalledBy, cur.def.Task)
				continue
			}
			if c.WfdID == task { // call back to the root: already represented
				continue
			}
			s := &Subworkflow{WfdID: c.WfdID, Depth: cur.depth + 1, CalledBy: []string{cur.def.Task}}
			index[c.WfdID] = s
			order = append(order, s)
			def, err := buildDefinition(ctx, q, opts, c.WfdID)
			switch {
			case errors.Is(err, ErrNotFound), errors.Is(err, ErrInvalidTask), errors.Is(err, ErrUnsupported):
				root.Warnings = append(root.Warnings, fmt.Sprintf(
					"called workflow %s (node %s of %s) could not be resolved: %v", c.WfdID, c.Node, cur.def.Task, err))
			case err != nil:
				return nil, fmt.Errorf("sub-workflow %s: %w", c.WfdID, err)
			default:
				s.Found, s.Definition = true, def
				queue = append(queue, queued{def, cur.depth + 1})
			}
		}
	}
	for _, s := range order {
		res.Subworkflows = append(res.Subworkflows, *s)
	}
	res.normalise()
	return res, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// buildDefinition loads and decodes one workflow definition.
func buildDefinition(ctx context.Context, q adt.QueryClient, opts Options, task string) (*Definition, error) {
	if !taskRe.MatchString(task) {
		return nil, fmt.Errorf("%w %q", ErrInvalidTask, task)
	}
	requested := ""
	if task == opts.Task {
		requested = opts.Version
	}
	var info versionInfo
	var err error
	if task == opts.Task && opts.Source == "runtime" {
		info, err = runtimeVersion(ctx, q, task, requested)
	} else {
		info, err = resolveVersion(ctx, q, task, requested, opts.Today)
	}
	if err != nil {
		return nil, err
	}
	var m *rawModel
	switch {
	case info.Source == "runtime":
		m, err = loadSWD(ctx, q, info.WfdID, info.Version, "E", opts.Language, opts.Container)
	case info.Tables == "SWD":
		m, err = loadSWD(ctx, q, info.WfdID, info.Version, "S", opts.Language, opts.Container)
	default:
		m, err = loadSWDS(ctx, q, info.WfdID, info.Version, opts.Language, opts.Container)
	}
	if err != nil {
		return nil, err
	}
	nodes, warnings := decodeNodes(m, opts.Language)
	edges := rawEdges(m)
	switch {
	case m.Model == "SWD":
		nodes, edges = finishRuntime(m, nodes, edges, opts.Raw)
	case !opts.Raw:
		nodes, edges = collapse(m, nodes, edges)
	}
	nodes, unresolved, err := resolveConstants(ctx, q, m, nodes)
	if err != nil {
		return nil, err
	}
	d := &Definition{
		WfdID:               info.WfdID,
		Task:                task,
		Version:             info.Version,
		VersionSource:       info.Source,
		SourceTables:        info.Tables,
		RuntimeVersion:      info.RuntimeVersion,
		HasConfiguration:    info.HasConfiguration,
		Language:            opts.Language,
		ActivatedOn:         formatDate(m.ActivatedOn),
		Nodes:               nodes,
		Edges:               edges,
		UnresolvedConstants: unresolved,
		Warnings:            append(append([]string{}, info.Warnings...), warnings...),
	}
	if opts.Container {
		els, cw, err := decodeContainer(m)
		if err != nil {
			return nil, err
		}
		d.Container = els
		d.Warnings = append(d.Warnings, cw...)
	}
	if d.Title, err = workflowTitle(ctx, q, task, opts.Language, m.MasterLanguage); err != nil {
		return nil, err
	}
	for _, n := range d.Nodes {
		if n.Kind == "subworkflow" {
			d.Calls = append(d.Calls, Call{Node: n.ID, WfdID: n.Task})
		}
	}
	if opts.Mermaid {
		d.Mermaid = renderMermaid(d)
	}
	return d, nil
}

// runtimeVersion selects the runtime copy of task in the logon client
// (SWD_HEADER, EXETYP 'E'). Only the root of a source=runtime read uses it;
// called workflows follow the active version rule.
func runtimeVersion(ctx context.Context, q adt.QueryClient, task, requested string) (versionInfo, error) {
	var info versionInfo
	wfdID, version, hasConfig, err := resolveRuntime(ctx, q, task)
	if err != nil {
		return info, err
	}
	if wfdID == "" {
		return info, fmt.Errorf("%w: no runtime copy of %s in the logon client", ErrNotFound, task)
	}
	if requested != "" && requested != version {
		return info, fmt.Errorf("the runtime copy of %s in the logon client is version %s, not %s", task, version, requested)
	}
	return versionInfo{WfdID: wfdID, Version: version, Source: "runtime", Tables: "SWD",
		RuntimeWfdID: wfdID, RuntimeVersion: version, HasConfiguration: hasConfig}, nil
}

// workflowTitle reads the workflow's short text from HRS1000, else HRP1000.
func workflowTitle(ctx context.Context, q adt.QueryClient, task, language, master string) (string, error) {
	objid := task[2:]
	for _, table := range []string{"hrs1000", "hrp1000"} {
		rows, err := fetch(ctx, q, fmt.Sprintf(
			"SELECT otype, objid, langu, stext FROM %s WHERE otype = 'WS' AND objid = '%s'", table, objid))
		if err != nil {
			return "", err
		}
		rows = keepEq(keepEq(rows, "OTYPE", "WS"), "OBJID", objid)
		if t := pickLanguage(rows, "LANGU", "STEXT", language, master); t != "" {
			return t, nil
		}
	}
	return "", nil
}

// formatDate turns SAP's YYYYMMDD into YYYY-MM-DD; "" for initial/invalid dates.
func formatDate(d string) string {
	if d == "" || d == "00000000" {
		return ""
	}
	t, err := time.Parse("20060102", d)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// normalise makes every slice non-nil so the result serialises as [] never null.
func (r *Result) normalise() {
	r.Definition.normalise()
	if r.Subworkflows == nil {
		r.Subworkflows = []Subworkflow{}
	}
	for i := range r.Subworkflows {
		s := &r.Subworkflows[i]
		if s.CalledBy == nil {
			s.CalledBy = []string{}
		}
		if s.Definition != nil {
			s.Definition.normalise()
		}
	}
}

func (d *Definition) normalise() {
	if d.Nodes == nil {
		d.Nodes = []Node{}
	}
	for i := range d.Nodes {
		if d.Nodes[i].Bindings == nil {
			d.Nodes[i].Bindings = []Binding{}
		}
	}
	if d.Edges == nil {
		d.Edges = []Edge{}
	}
	if d.Container == nil {
		d.Container = []ContainerElement{}
	}
	if d.Calls == nil {
		d.Calls = []Call{}
	}
	if d.UnresolvedConstants == nil {
		d.UnresolvedConstants = []string{}
	}
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
}
