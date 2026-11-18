package bwf

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

var (
	languageRe = regexp.MustCompile(`^[A-Z0-9]$`)
	taskRefRe  = regexp.MustCompile(`^(TS|WS)\d{8}$`)
)

// excludedExprTypes appear on almost every node and carry nothing the decoder uses.
var excludedExprTypes = map[string]bool{"037": true, "038": true, "039": true, "040": true}

// loadSWDS reads one client-independent definition version from the SWDS* tables.
// Bindings and container rows are only read when withContainer is set.
func loadSWDS(ctx context.Context, q adt.QueryClient, wfdID, version, language string, withContainer bool) (*rawModel, error) {
	if !validName(wfdID) {
		return nil, fmt.Errorf("invalid workflow definition id %q", wfdID)
	}
	if !versionRe.MatchString(version) {
		return nil, fmt.Errorf("invalid version %q", version)
	}
	if !languageRe.MatchString(language) {
		return nil, fmt.Errorf("invalid language %q", language)
	}
	l := swdsLoader{ctx: ctx, q: q, wfdID: wfdID, version: version,
		key: fmt.Sprintf("WHERE wfd_id = '%s' AND version = '%s' AND exetyp = 'S'", wfdID, version)}
	m := &rawModel{Model: "SWDS", WfdID: wfdID, Version: version}

	hdr, err := l.rows("SELECT wfd_id, version, language, activat_on FROM swdsheader")
	if err != nil {
		return nil, err
	}
	if len(hdr) == 0 {
		return nil, fmt.Errorf("%w: %s version %s in SWDSHEADER", ErrNotFound, wfdID, version)
	}
	m.MasterLanguage, m.ActivatedOn = hdr[0]["LANGUAGE"], hdr[0]["ACTIVAT_ON"]

	if m.Nodes, err = l.nodes(); err != nil {
		return nil, err
	}
	if m.Lines, err = l.lines(); err != nil {
		return nil, err
	}
	if m.Texts, err = l.texts(language, m.MasterLanguage); err != nil {
		return nil, err
	}
	if m.Conds, err = l.rows("SELECT wfd_id, version, nodeid, line_id, line_typ, leftexpr, compop, rightexpr, logop, notflag FROM swdscondef"); err != nil {
		return nil, err
	}
	if m.Exprs, err = l.exprs(); err != nil {
		return nil, err
	}
	if withContainer {
		if m.Bindings, err = l.rows("SELECT wfd_id, version, nodeid, bindtype, dataflow, element, expr FROM swdsbindef"); err != nil {
			return nil, err
		}
		if m.CntIdx, err = l.rows("SELECT otype, objid, wfd_id, version, element, typeid, catid, typekind, typename FROM swdscntidx"); err != nil {
			return nil, err
		}
		if m.ContXML, err = l.container(); err != nil {
			return nil, err
		}
	}
	if m.TaskTexts, err = loadTaskTexts(ctx, q, m.Nodes); err != nil {
		return nil, err
	}
	return m, nil
}

type swdsLoader struct {
	ctx            context.Context
	q              adt.QueryClient
	wfdID, version string
	key            string // WHERE clause on the definition key
}

// rows runs "<sel> <key> <extra>" and keeps only rows of this definition version.
func (l swdsLoader) rows(sel string, extra ...string) ([]Row, error) {
	sql := sel + " " + l.key
	if len(extra) > 0 {
		sql += " " + strings.Join(extra, " ")
	}
	rows, err := fetch(l.ctx, l.q, sql)
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for _, r := range rows {
		if r["WFD_ID"] == l.wfdID && r["VERSION"] == l.version {
			out = append(out, r)
		}
	}
	return out, nil
}

func (l swdsLoader) nodes() ([]rawNode, error) {
	rows, err := l.rows("SELECT wfd_id, version, nodeid, nodetype, swdsmnodes~task, blockid, par_blckid," +
		" cont_cmd, return, returncode, exc_clsnam, min_tokens, exp_tokens, evt_name, evt_type, evt_otype, case_conte" +
		" FROM swdsmnodes")
	if err != nil {
		return nil, err
	}
	nodes := make([]rawNode, 0, len(rows))
	for _, r := range rows {
		evt := r["EVT_NAME"]
		if evt == "" {
			evt = r["EVT_TYPE"] // BOR events carry their name here
		}
		nodes = append(nodes, rawNode{
			ID: r["NODEID"], Type: r["NODETYPE"], Task: r["TASK"],
			BlockID: r["BLOCKID"], ParentBlockID: r["PAR_BLCKID"], ContCmd: r["CONT_CMD"],
			Return: r["RETURN"], ReturnCode: r["RETURNCODE"], ExcClass: r["EXC_CLSNAM"],
			MinTokens: r["MIN_TOKENS"], ExpTokens: r["EXP_TOKENS"], EvtName: evt,
			EvtOType: r["EVT_OTYPE"], CaseConte: r["CASE_CONTE"],
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, nil
}

func (l swdsLoader) lines() ([]rawLine, error) {
	rows, err := l.rows("SELECT wfd_id, version, pred_node, succ_node FROM swdsmlines")
	if err != nil {
		return nil, err
	}
	lines := make([]rawLine, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, rawLine{From: r["PRED_NODE"], To: r["SUCC_NODE"]})
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].From != lines[j].From {
			return lines[i].From < lines[j].From
		}
		return lines[i].To < lines[j].To
	})
	return lines, nil
}

// texts reads SWDSTEXT in the requested and master language; if that yields no
// node name (ND) at all, it reads every language.
func (l swdsLoader) texts(language, master string) ([]Row, error) {
	langs := map[string]bool{language: true}
	list := "'" + language + "'"
	if languageRe.MatchString(master) && master != language {
		langs[master] = true
		list += ", '" + master + "'"
	}
	const sel = "SELECT wfd_id, version, nodeid, language, texttyp, textnumber, wfd_text FROM swdstext"
	rows, err := l.rows(sel, "AND language IN ("+list+")")
	if err != nil {
		return nil, err
	}
	filtered := []Row{}
	hasND := false
	for _, r := range rows {
		if langs[r["LANGUAGE"]] {
			filtered = append(filtered, r)
			hasND = hasND || r["TEXTTYP"] == "ND"
		}
	}
	if hasND {
		return filtered, nil
	}
	return l.rows(sel)
}

func (l swdsLoader) exprs() ([]Row, error) {
	rows, err := l.rows("SELECT wfd_id, version, nodeid, exprtype, expression FROM swdsexpr",
		"AND exprtype NOT IN ('037', '038', '039', '040')")
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for _, r := range rows {
		if !excludedExprTypes[r["EXPRTYPE"]] {
			out = append(out, r)
		}
	}
	return out, nil
}

// container reads the workflow container XML (node 0, empty CNT_TYPE) in TABIX order.
func (l swdsLoader) container() ([]Row, error) {
	rows, err := l.rows("SELECT wfd_id, version, nodeid, cnt_type, tabix, length, data FROM swdscont",
		"AND nodeid = '0000000000' AND cnt_type = ' '")
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for _, r := range rows {
		if trimID(r["NODEID"]) == "" && r["CNT_TYPE"] == "" {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["TABIX"] < out[j]["TABIX"] })
	return out, nil
}

// loadTaskTexts reads the short texts of the tasks the nodes reference, from
// HRS1000 and, for tasks not found there, HRP1000.
func loadTaskTexts(ctx context.Context, q adt.QueryClient, nodes []rawNode) ([]Row, error) {
	want := map[string]bool{}
	for _, n := range nodes {
		if t := taskOf(n.Task); taskRefRe.MatchString(t) {
			want[t] = true
		}
	}
	out := []Row{}
	found := map[string]bool{}
	for _, table := range []string{"hrs1000", "hrp1000"} {
		var missing []string
		for t := range want {
			if !found[t] {
				missing = append(missing, "'"+t[2:]+"'")
			}
		}
		if len(missing) == 0 {
			break
		}
		sort.Strings(missing)
		rows, err := fetch(ctx, q, fmt.Sprintf(
			"SELECT otype, objid, langu, stext FROM %s WHERE otype IN ('TS', 'WS') AND objid IN (%s)",
			table, strings.Join(missing, ", ")))
		if err != nil {
			return nil, err
		}
		hit := map[string]bool{}
		for _, r := range rows {
			if t := r["OTYPE"] + r["OBJID"]; want[t] && !found[t] {
				out = append(out, r)
				hit[t] = true
			}
		}
		for t := range hit {
			found[t] = true
		}
	}
	return out, nil
}

// taskOf trims a node's TASK column; "" for none.
func taskOf(nodeTask string) string { return strings.TrimSpace(nodeTask) }

// trimID drops leading zeros from a node or block id ("0000000036" → "36"); an
// all-zero id (no block) becomes "".
func trimID(id string) string { return strings.TrimLeft(strings.TrimSpace(id), "0") }
