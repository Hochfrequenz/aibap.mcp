package bwf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// nodeKinds maps a NODETYPE to the normalised kind; FUNC is refined by its task.
var nodeKinds = map[string]string{
	"STRT": "start",
	"END":  "end",
	"FUNC": "step",
	"IF":   "condition",
	"CASE": "multiple_condition",
	"UNTL": "loop_until",
	"WHIL": "loop_while",
	"FORK": "fork",
	"JOIN": "join",
	"OPRT": "container_operation",
	"CTRL": "process_control",
	"WAIT": "wait_event",
	"TRIG": "trigger_event",
	"MSEL": "user_decision",
	"MAIL": "mail",
}

// Container operation commands (CONT_CMD, domain SWD_CONTRL) and their operators.
var arithmeticOps = map[string]string{"11": "+", "13": "-", "14": "*", "15": "/"}

// Process control commands on CTRL nodes.
var controlOps = map[string]string{
	"01": "cancel work item",
	"02": "cancel workflow",
	"04": "set step obsolete",
	"07": "complete workflow",
}

const (
	exprAgent    = "013"
	exprResult   = "031"
	exprOperand1 = "032"
	exprOperand2 = "033"
	exprCase     = "035"
)

var typeSuffixRe = regexp.MustCompile(`\{TYPE=[^}]*\}$`)

// decodeNodes turns the raw node rows into output nodes in raw (un-collapsed) form.
func decodeNodes(m *rawModel, language string) ([]Node, []string) {
	warnings := []string{}
	byNode := func(rows []Row) map[string][]Row {
		out := map[string][]Row{}
		for _, r := range rows {
			id := trimID(r["NODEID"])
			out[id] = append(out[id], r)
		}
		return out
	}
	texts, conds, exprs, binds := byNode(m.Texts), byNode(m.Conds), byNode(m.Exprs), byNode(m.Bindings)

	known := map[string]bool{}
	for _, n := range m.Nodes {
		known[trimID(n.ID)] = true
	}
	orphans := 0
	for id, rows := range texts {
		if !known[id] {
			orphans += len(rows)
		}
	}
	if orphans > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"ignored %d SWDSTEXT text rows for node ids that do not exist in version %s", orphans, m.Version))
	}

	nodes := make([]Node, 0, len(m.Nodes))
	for _, rn := range m.Nodes {
		id := trimID(rn.ID)
		n := Node{
			ID:            id,
			Type:          rn.Type,
			Kind:          kindOf(rn),
			Task:          taskOf(rn.Task),
			Event:         eventOf(rn),
			BlockID:       trimID(rn.BlockID),
			ParentBlockID: trimID(rn.ParentBlockID),
			Bindings:      decodeBindings(binds[id]),
		}
		ex := exprMap(exprs[id])
		n.Agent = ex[exprAgent]
		switch rn.Type {
		case "CASE":
			n.Condition = ex[exprCase]
		case "OPRT":
			op, warn := operationOf(rn.ContCmd, ex)
			n.Operation = op
			if warn != "" {
				warnings = append(warnings, fmt.Sprintf("node %s: %s", id, warn))
			}
		case "CTRL":
			if op, ok := controlOps[rn.ContCmd]; ok {
				n.Operation = op
			} else {
				n.Operation = "cmd " + rn.ContCmd
				warnings = append(warnings, fmt.Sprintf("node %s: unknown process control command %s", id, rn.ContCmd))
			}
		}
		if c := conditionOf(conds[id]); c != "" {
			n.Condition = c
		}
		n.Label = labelOf(rn, texts[id], m.TaskTexts, language, m.MasterLanguage)
		n.ConditionResolved = n.Condition
		n.OperationResolved = n.Operation
		nodes = append(nodes, n)
	}
	return nodes, warnings
}

// rawEdges returns every line as an edge, ids without leading zeros.
func rawEdges(m *rawModel) []Edge {
	edges := make([]Edge, 0, len(m.Lines))
	for _, l := range m.Lines {
		edges = append(edges, Edge{From: trimID(l.From), To: trimID(l.To), Label: l.ReturnCode})
	}
	return edges
}

func kindOf(n rawNode) string {
	k, ok := nodeKinds[n.Type]
	if !ok {
		return "other"
	}
	if k == "step" && strings.HasPrefix(taskOf(n.Task), "WS") {
		return "subworkflow"
	}
	return k
}

func eventOf(n rawNode) string {
	switch {
	case n.EvtOType != "" && n.EvtName != "":
		return n.EvtOType + "." + n.EvtName
	case n.EvtName != "":
		return n.EvtName
	default:
		return n.EvtOType
	}
}

// exprMap keys a node's SWDSEXPR rows by EXPRTYPE (first row wins).
func exprMap(rows []Row) map[string]string {
	out := map[string]string{}
	for _, r := range rows {
		if _, ok := out[r["EXPRTYPE"]]; !ok {
			out[r["EXPRTYPE"]] = r["EXPRESSION"]
		}
	}
	return out
}

// operationOf renders a container operation; an unknown command is returned
// as "cmd <nn>" with its operands, plus a warning.
func operationOf(cmd string, ex map[string]string) (string, string) {
	res, a, b := ex[exprResult], ex[exprOperand1], ex[exprOperand2]
	switch {
	case cmd == "12":
		return res + " := " + a, ""
	case cmd == "18":
		return "append " + a + " to " + res, ""
	case arithmeticOps[cmd] != "":
		return res + " := " + a + " " + arithmeticOps[cmd] + " " + b, ""
	}
	var parts []string
	for _, p := range []string{res, a, b} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	op := "cmd " + cmd
	if len(parts) > 0 {
		op += " " + strings.Join(parts, ", ")
	}
	return op, "unknown container operation command " + cmd
}

// conditionOf renders the SWDSCONDEF lines of one node. Only expression ('-')
// and parenthesis ('(' / ')') lines count; the parse-tree line ('E') and the
// change-info line ('') are dropped wherever they sit. A line's LOGOP joins it
// to the next item; a ')' line carries the operator after its group, and the
// RIGHTEXPR of a parenthesis line is a comment.
func conditionOf(rows []Row) string {
	var lines []Row
	for _, r := range rows {
		switch r["LINE_TYP"] {
		case "-", "(", ")":
			lines = append(lines, r)
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i]["LINE_ID"] < lines[j]["LINE_ID"] })
	var parts []string
	pending := ""
	for _, r := range lines {
		not := ""
		if r["NOTFLAG"] != "" {
			not = "NOT "
		}
		switch r["LINE_TYP"] {
		case ")":
			parts = append(parts, ")")
		case "(":
			if pending != "" {
				parts = append(parts, pending)
			}
			parts = append(parts, not+"(")
			pending = ""
			continue
		default:
			if pending != "" {
				parts = append(parts, pending)
			}
			parts = append(parts, not+comparison(r))
		}
		pending = strings.ToUpper(r["LOGOP"])
	}
	return strings.Join(parts, " ")
}

func comparison(r Row) string {
	left := typeSuffixRe.ReplaceAllString(r["LEFTEXPR"], "")
	right := typeSuffixRe.ReplaceAllString(r["RIGHTEXPR"], "")
	switch r["COMPOP"] {
	case "EX":
		return left + " is filled"
	case "NX":
		return left + " is initial"
	}
	return strings.TrimSpace(left + " " + r["COMPOP"] + " " + right)
}

// labelOf picks the node name: ND text in language, master language, any
// language; then the task short text; then the raw node type.
func labelOf(n rawNode, texts, taskTexts []Row, language, master string) string {
	var nd []Row
	for _, r := range texts {
		if r["TEXTTYP"] == "ND" && r["WFD_TEXT"] != "" {
			nd = append(nd, r)
		}
	}
	sort.SliceStable(nd, func(i, j int) bool { return nd[i]["TEXTNUMBER"] < nd[j]["TEXTNUMBER"] })
	if t := pickLanguage(nd, "LANGUAGE", "WFD_TEXT", language, master); t != "" {
		return t
	}
	if task := taskOf(n.Task); task != "" && len(task) > 2 {
		var tt []Row
		for _, r := range taskTexts {
			if r["OTYPE"]+r["OBJID"] == task && r["STEXT"] != "" {
				tt = append(tt, r)
			}
		}
		if t := pickLanguage(tt, "LANGU", "STEXT", language, master); t != "" {
			return t
		}
	}
	return n.Type
}

func pickLanguage(rows []Row, langCol, textCol, language, master string) string {
	for _, l := range []string{language, master} {
		for _, r := range rows {
			if r[langCol] == l {
				return r[textCol]
			}
		}
	}
	if len(rows) > 0 {
		return rows[0][textCol]
	}
	return ""
}

func decodeBindings(rows []Row) []Binding {
	sorted := append([]Row(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a["BINDTYPE"] != b["BINDTYPE"] {
			return a["BINDTYPE"] < b["BINDTYPE"]
		}
		if a["DATAFLOW"] != b["DATAFLOW"] {
			return a["DATAFLOW"] < b["DATAFLOW"]
		}
		return a["ELEMENT"] < b["ELEMENT"]
	})
	out := make([]Binding, 0, len(sorted))
	for _, r := range sorted {
		dir := r["DATAFLOW"]
		switch dir {
		case "E":
			dir = "export"
		case "I":
			dir = "import"
		}
		out = append(out, Binding{
			Direction:          dir,
			StepElement:        r["ELEMENT"],
			Expression:         r["EXPR"],
			ExpressionResolved: r["EXPR"],
			BindType:           r["BINDTYPE"],
		})
	}
	return out
}
