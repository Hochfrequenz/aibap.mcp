package bwf

import (
	"context"
	"fmt"
	"sort"

	"github.com/Hochfrequenz/adtler/adt"
)

// loadSWD reads one client-dependent definition version from the SWD_* tables
// of the logon client (the CLIENT column is never named: data preview reads
// the logon client).
//
// exetyp "E" is the runtime copy: nodes from SWD_NODES (1-character node
// types, Model "SWD"), lines with RETURNCODE from SWD_LINES, step tasks from
// SWD_STEPS. exetyp "S" is a legacy definition: verified on both systems,
// SWD_NODES holds no EXETYP 'S' rows; legacy definitions are stored in
// SWD_MNODES/SWD_MLINES with the same block model as SWDSMNODES, so they are
// read with that layout (Model "SWDS"). Texts, conditions, expressions,
// bindings and container rows come from SWD_TEXT, SWD_CONDEF, SWD_EXPR,
// SWD_BINDEF, SWD_CNTIDX, SWD_CONT and SWD_WFCTXT in both cases.
func loadSWD(ctx context.Context, q adt.QueryClient, wfdID, version, exetyp, language string, withContainer bool) (*rawModel, error) {
	if err := validateKeys(wfdID, version, language); err != nil {
		return nil, err
	}
	if exetyp != "E" && exetyp != "S" {
		return nil, fmt.Errorf("invalid execution type %q", exetyp)
	}
	l := newLoader(ctx, q, "swd_", exetyp, wfdID, version)
	m := &rawModel{TablePrefix: "SWD_", WfdID: wfdID, Version: version, Language: language}
	var err error
	if exetyp == "E" {
		m.Model = "SWD"
		err = l.load(m, withContainer, l.runtimeNodes, l.runtimeLines)
	} else {
		m.Model = "SWDS"
		err = l.load(m, withContainer, l.nodes, l.lines)
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// runtimeNodes reads SWD_NODES and fills each step's task from SWD_STEPS.
func (l swdsLoader) runtimeNodes() ([]rawNode, error) {
	rows, err := l.rows("SELECT wfd_id, version, exetyp, nodeid, nodetype, blockid, par_blckid, case_conte, cont_cmd," +
		" min_tokens, exp_tokens, parameter1, parameter2, parameter3, evt_type, evt_otype FROM swd_nodes")
	if err != nil {
		return nil, err
	}
	steps, err := l.rows("SELECT wfd_id, version, exetyp, nodeid, swd_steps~task FROM swd_steps")
	if err != nil {
		return nil, err
	}
	taskOfNode := make(map[string]string, len(steps))
	for _, r := range steps {
		taskOfNode[trimID(r["NODEID"])] = r["TASK"]
	}
	nodes := make([]rawNode, 0, len(rows))
	for _, r := range rows {
		nodes = append(nodes, rawNode{
			ID: r["NODEID"], Type: r["NODETYPE"], Task: taskOfNode[trimID(r["NODEID"])],
			BlockID: r["BLOCKID"], ParentBlockID: r["PAR_BLCKID"], ContCmd: r["CONT_CMD"],
			MinTokens: r["MIN_TOKENS"], ExpTokens: r["EXP_TOKENS"], EvtType: r["EVT_TYPE"],
			EvtOType: r["EVT_OTYPE"], CaseConte: r["CASE_CONTE"],
			Param1: r["PARAMETER1"], Param2: r["PARAMETER2"], Param3: r["PARAMETER3"],
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, nil
}

// runtimeLines reads SWD_LINES including LINE_TYPE and RETURNCODE.
func (l swdsLoader) runtimeLines() ([]rawLine, error) {
	rows, err := l.rows("SELECT wfd_id, version, exetyp, pred_node, succ_node, line_type, returncode FROM swd_lines")
	if err != nil {
		return nil, err
	}
	lines := make([]rawLine, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, rawLine{From: r["PRED_NODE"], To: r["SUCC_NODE"], LineType: r["LINE_TYPE"], ReturnCode: r["RETURNCODE"]})
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].From != lines[j].From {
			return lines[i].From < lines[j].From
		}
		return lines[i].To < lines[j].To
	})
	return lines, nil
}
