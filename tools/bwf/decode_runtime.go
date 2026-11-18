package bwf

import "strings"

// swdReturnCodes are the SWD_LINES.RETURNCODE values whose meaning is
// confirmed. 8995/8994 were checked on WS48900002 against the definition's
// outcome nodes of the same node ids (runtime copy 0000 vs. SWDS 0001): the IF
// node 36 leads with 8995 to node 42, which the IFRS with RETURN '0001'
// (true) leads to, and with 8994 to node 50 (IFRS '0000', false); the loop
// condition node 10 leads with 8995 to the loop exit (UNRS '0001') and with
// 8994 back to the loop entry (UNRS '0000'). _SAP_WFSYST_RC_OTHERS is the COTH
// outcome. Other codes (e.g. case values) are returned as they are.
var swdReturnCodes = map[string]string{
	"8995":                  "true",
	"8994":                  "false",
	"_SAP_WFSYST_RC_OTHERS": "others",
}

// lineLabel is the edge label of one SWD_LINES row: a confirmed return code;
// "" for the unconditional code 0000 outside result lines (LINE_TYPE 'R');
// "event <OTYPE>.<EVENT>" for terminating-event lines (LINE_TYPE 'Y', fixed
// layout CATID(2) OBJTYPE(32) EVENT(32) ELEMENT, checked against the TEVT node
// of the definition); otherwise the raw code.
func lineLabel(l rawLine) string {
	rc := l.ReturnCode
	if label, ok := swdReturnCodes[rc]; ok {
		return label
	}
	switch {
	case rc == "0000" && l.LineType != "R":
		return ""
	case l.LineType == "Y" && len(rc) > 34:
		otype := strings.TrimSpace(rc[2:34])
		event := strings.TrimSpace(rc[34:min(len(rc), 66)])
		if otype != "" && event != "" {
			return "event " + otype + "." + event
		}
	}
	return rc
}

// finishRuntime post-processes the SWD_NODES model, which is not collapsed:
// a join is labelled "n of m"; unless raw, block markers are removed (their
// predecessors connected to their successors) and edges to node ids without
// a node row (e.g. the admin node 999502) are dropped.
func finishRuntime(m *rawModel, nodes []Node, edges []Edge, raw bool) ([]Node, []Edge) {
	rawByID := make(map[string]rawNode, len(m.Nodes))
	for _, rn := range m.Nodes {
		rawByID[trimID(rn.ID)] = rn
	}
	out := make([]Node, 0, len(nodes))
	drop := map[string]bool{}
	for _, n := range nodes {
		if !raw && blockMarkerTypes[n.Type] {
			drop[n.ID] = true
			continue
		}
		if n.Kind == "join" {
			rn := rawByID[n.ID]
			n.Label = tokens(rn.MinTokens) + " of " + tokens(rn.ExpTokens)
		}
		out = append(out, n)
	}
	if raw {
		return out, edges
	}
	kept := make(map[string]bool, len(out))
	for _, n := range out {
		kept[n.ID] = true
	}
	succ := map[string][]Edge{}
	for _, e := range edges {
		succ[e.From] = append(succ[e.From], e)
	}
	outEdges := []Edge{}
	seen := map[Edge]bool{}
	type hop struct{ to, label string }
	for _, e := range edges {
		if !kept[e.From] {
			continue
		}
		// Follow the edge through dropped block markers to the next kept node;
		// the first non-empty label on the path labels the edge.
		stack := []hop{{e.To, e.Label}}
		visited := map[string]bool{}
		for len(stack) > 0 {
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !drop[h.to] {
				ne := Edge{From: e.From, To: h.to, Label: h.label}
				if kept[h.to] && !seen[ne] {
					seen[ne] = true
					outEdges = append(outEdges, ne)
				}
				continue
			}
			if visited[h.to] {
				continue
			}
			visited[h.to] = true
			for _, s := range succ[h.to] {
				label := h.label
				if label == "" {
					label = s.Label
				}
				stack = append(stack, hop{s.To, label})
			}
		}
	}
	return out, outEdges
}
