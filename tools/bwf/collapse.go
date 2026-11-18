package bwf

import (
	"strings"
)

// While-loop shape, verified on WS00400012 version 0004 (testdata/ws00400012_while.json):
//
//	WHJO → WHIL → XOR → WHRS ("Continue") → first body node
//	                  → WOTH ("Exit", RETURNCODE _SAP_WFSYST_RC_OTHERS) → node after the loop
//	last body node → EWHI → WHJO
//
// WHRS leads into the body and carries no RETURN value, so WHRS = "true" and
// WOTH = "false". EWHI is the loop's back-edge merge, not its exit.

// helperTypes are block-structure nodes that never survive collapsing.
var helperTypes = map[string]bool{
	"XOR": true, "ECAS": true, "ENDI": true, "UNJO": true, "WHJO": true, "EWHI": true,
	"EFUN": true, "EVTG": true, "OFF": true,
	// outcome nodes
	"IFRS": true, "UNRS": true, "WHRS": true, "WOTH": true, "CRES": true, "COTH": true,
	"DRES": true, "EXCP": true, "TEVT": true, "CEVT": true, "EEVT": true, "EVOI": true,
	"WEVT": true, "IEVT": true,
}

// collapse removes helper, outcome and admin nodes and reconnects the
// remaining nodes with edges labelled by the outcome they pass through. The
// inputs are not modified.
func collapse(m *rawModel, nodes []Node, edges []Edge) ([]Node, []Edge) {
	raw := make(map[string]rawNode, len(m.Nodes))
	for _, rn := range m.Nodes {
		raw[trimID(rn.ID)] = rn
	}
	byID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	succ := map[string][]string{}
	afterXOR := map[string]bool{}
	for _, e := range edges {
		succ[e.From] = append(succ[e.From], e.To)
		if byID[e.From].Type == "XOR" {
			afterXOR[e.To] = true
		}
	}
	removed := func(id string) bool {
		t := byID[id].Type
		if t == "END" {
			return false
		}
		return helperTypes[t] || afterXOR[id] || (len(id) == 6 && strings.HasPrefix(id, "999"))
	}

	outNodes := make([]Node, 0, len(nodes))
	outEdges := []Edge{}
	seen := map[Edge]bool{}
	for _, n := range nodes {
		if removed(n.ID) {
			continue
		}
		if n.Type == "JOIN" {
			rn := raw[n.ID]
			n.Label = tokens(rn.MinTokens) + " of " + tokens(rn.ExpTokens)
		}
		outNodes = append(outNodes, n)

		// Walk every path through removed nodes to the next kept node; the
		// first outcome label met on the path labels the edge.
		type hop struct{ id, label string }
		visited := map[hop]bool{}
		stack := []hop{}
		next := succ[n.ID]
		for i := len(next) - 1; i >= 0; i-- {
			stack = append(stack, hop{next[i], ""})
		}
		for len(stack) > 0 {
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, ok := byID[h.id]; ok && !removed(h.id) {
				e := Edge{From: n.ID, To: h.id, Label: h.label}
				if !seen[e] {
					seen[e] = true
					outEdges = append(outEdges, e)
				}
				continue
			}
			if visited[h] {
				continue
			}
			visited[h] = true
			label := h.label
			if label == "" {
				label = outcomeLabel(raw[h.id], byID[h.id])
			}
			s := succ[h.id]
			for i := len(s) - 1; i >= 0; i-- {
				stack = append(stack, hop{s[i], label})
			}
		}
	}
	return outNodes, outEdges
}

// outcomeLabel is the edge label an outcome node contributes; "" for none.
func outcomeLabel(rn rawNode, n Node) string {
	switch rn.Type {
	case "IFRS", "UNRS":
		switch rn.Return {
		case "0001":
			return "true"
		case "0000":
			return "false"
		}
	case "WHRS":
		return "true"
	case "WOTH":
		return "false"
	case "CRES":
		return rn.ReturnCode
	case "COTH":
		return "others"
	case "DRES":
		return textLabel(rn, n)
	case "EXCP":
		if rn.ExcClass != "" {
			return "exception " + rn.ExcClass
		}
	case "TEVT":
		if rn.EvtName != "" {
			return "event " + rn.EvtName
		}
		return textLabel(rn, n)
	}
	return ""
}

// textLabel is the node's own name, or "" when it only fell back to its type.
func textLabel(rn rawNode, n Node) string {
	if n.Label == rn.Type {
		return ""
	}
	return n.Label
}

// tokens drops leading zeros from a token count ("002" → "2", "000" → "0").
func tokens(s string) string {
	if t := strings.TrimLeft(strings.TrimSpace(s), "0"); t != "" {
		return t
	}
	return "0"
}
