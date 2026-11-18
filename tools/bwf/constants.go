package bwf

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// maxConstantDepth bounds the interface/superclass hierarchy walk.
const maxConstantDepth = 5

var constRefRe = regexp.MustCompile(`&([A-Z0-9_]+)\.([A-Z0-9_]+)&`)

// constKey identifies a constant reference: container element and constant name.
type constKey struct{ element, name string }

// constSearch tracks the hierarchy walk for one reference.
type constSearch struct {
	frontier []string
	visited  map[string]bool
	value    string
	done     bool
}

// resolveConstants replaces &ELEMENT.CONSTANT& references in node conditions,
// operations and binding expressions by the constant's ABAP literal. Only
// elements typed as ABAP classes/interfaces are candidates. References that
// cannot be resolved stay verbatim and are returned (deduplicated, sorted).
func resolveConstants(ctx context.Context, q adt.QueryClient, m *rawModel, nodes []Node) ([]Node, []string, error) {
	classOf := map[string]string{}
	for _, r := range m.CntIdx {
		if r["OTYPE"] == "WS" && r["CATID"] == "CL" {
			if _, ok := classOf[r["ELEMENT"]]; !ok {
				classOf[r["ELEMENT"]] = r["TYPEID"]
			}
		}
	}

	searches := map[constKey]*constSearch{}
	scan := func(s string) {
		for _, mm := range constRefRe.FindAllStringSubmatch(s, -1) {
			cls, ok := classOf[mm[1]]
			if !ok {
				continue
			}
			k := constKey{mm[1], mm[2]}
			if _, seen := searches[k]; seen {
				continue
			}
			if !validName(cls) || !validName(k.name) {
				searches[k] = &constSearch{done: true}
				continue
			}
			searches[k] = &constSearch{frontier: []string{cls}, visited: map[string]bool{cls: true}}
		}
	}
	for _, n := range nodes {
		scan(n.Condition)
		scan(n.Operation)
		for _, b := range n.Bindings {
			scan(b.Expression)
		}
	}

	if err := walkHierarchy(ctx, q, searches); err != nil {
		return nil, nil, err
	}

	unresolvedSet := map[string]bool{}
	replace := func(s string) string {
		return constRefRe.ReplaceAllStringFunc(s, func(ref string) string {
			mm := constRefRe.FindStringSubmatch(ref)
			sr, ok := searches[constKey{mm[1], mm[2]}]
			if !ok {
				return ref
			}
			if sr.value == "" {
				unresolvedSet[ref] = true
				return ref
			}
			return sr.value
		})
	}

	out := make([]Node, len(nodes))
	for i, n := range nodes {
		n.ConditionResolved = replace(n.Condition)
		n.OperationResolved = replace(n.Operation)
		if n.Bindings != nil {
			bs := make([]Binding, len(n.Bindings))
			for j, b := range n.Bindings {
				b.ExpressionResolved = replace(b.Expression)
				bs[j] = b
			}
			n.Bindings = bs
		}
		out[i] = n
	}
	unresolved := make([]string, 0, len(unresolvedSet))
	for k := range unresolvedSet {
		unresolved = append(unresolved, k)
	}
	sort.Strings(unresolved)
	return out, unresolved, nil
}

// walkHierarchy resolves all searches level by level: one SEOCOMPODF query per
// round, then one SEOMETAREL query to move each open search up one level.
func walkHierarchy(ctx context.Context, q adt.QueryClient, searches map[constKey]*constSearch) error {
	for level := 0; level < maxConstantDepth; level++ {
		classes, consts := map[string]bool{}, map[string]bool{}
		for k, s := range searches {
			if s.done {
				continue
			}
			for _, c := range s.frontier {
				classes[c] = true
			}
			consts[k.name] = true
		}
		if len(classes) == 0 {
			return nil
		}
		rows, err := fetch(ctx, q, "SELECT clsname, cmpname, version, attdecltyp, attvalue FROM seocompodf\n"+
			"WHERE clsname IN ("+sqlList(classes)+")\n"+
			"AND cmpname IN ("+sqlList(consts)+")\n"+
			"AND version = '1' AND attdecltyp = '2'")
		if err != nil {
			return fmt.Errorf("resolve constants: %w", err)
		}
		values := map[[2]string]string{}
		for _, r := range rows {
			if r["VERSION"] == "1" && r["ATTDECLTYP"] == "2" && r["ATTVALUE"] != "" {
				values[[2]string{r["CLSNAME"], r["CMPNAME"]}] = r["ATTVALUE"]
			}
		}
		open := map[string]bool{}
		for k, s := range searches {
			if s.done {
				continue
			}
			for _, c := range s.frontier {
				if v, ok := values[[2]string{c, k.name}]; ok {
					s.value, s.done = v, true
					break
				}
			}
			if !s.done {
				for _, c := range s.frontier {
					open[c] = true
				}
			}
		}
		if len(open) == 0 || level == maxConstantDepth-1 {
			return nil
		}
		parents, err := loadParents(ctx, q, open)
		if err != nil {
			return err
		}
		for _, s := range searches {
			if s.done {
				continue
			}
			var next []string
			for _, c := range s.frontier {
				for _, p := range parents[c] {
					if !s.visited[p] {
						s.visited[p] = true
						next = append(next, p)
					}
				}
			}
			s.frontier = next
			if len(next) == 0 {
				s.done = true
			}
		}
	}
	return nil
}

// loadParents returns the implemented interfaces and superclasses per class.
func loadParents(ctx context.Context, q adt.QueryClient, classes map[string]bool) (map[string][]string, error) {
	rel, err := fetch(ctx, q, "SELECT clsname, refclsname, version, reltype FROM seometarel\n"+
		"WHERE clsname IN ("+sqlList(classes)+")\n"+
		"AND version = '1' AND reltype IN ('1', '2')")
	if err != nil {
		return nil, fmt.Errorf("resolve constants: %w", err)
	}
	parents := map[string][]string{}
	for _, r := range rel {
		if r["VERSION"] == "1" && (r["RELTYPE"] == "1" || r["RELTYPE"] == "2") && validName(r["REFCLSNAME"]) {
			parents[r["CLSNAME"]] = append(parents[r["CLSNAME"]], r["REFCLSNAME"])
		}
	}
	return parents, nil
}

// sqlList renders a sorted set of already validated names as quoted literals,
// one per line so no SQL line exceeds the data preview limit.
func sqlList(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, "'"+n+"'")
	}
	sort.Strings(names)
	return "\n" + strings.Join(names, ",\n") + "\n"
}
