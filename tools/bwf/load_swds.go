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
	if err := validateKeys(wfdID, version, language); err != nil {
		return nil, err
	}
	l := newLoader(ctx, q, "swds", "S", wfdID, version)
	m := &rawModel{Model: "SWDS", TablePrefix: "SWDS", WfdID: wfdID, Version: version, Language: language}
	if err := l.load(m, withContainer, l.nodes, l.lines); err != nil {
		return nil, err
	}
	return m, nil
}

// validateKeys checks the values that are embedded as SQL literals.
func validateKeys(wfdID, version, language string) error {
	if !validName(wfdID) {
		return fmt.Errorf("invalid workflow definition id %q", wfdID)
	}
	if !versionRe.MatchString(version) {
		return fmt.Errorf("invalid version %q", version)
	}
	if !languageRe.MatchString(language) {
		return fmt.Errorf("invalid language %q", language)
	}
	return nil
}

// swdsLoader reads the tables of one definition version. The same table
// layout exists client-independently (prefix "swds") and per client (prefix
// "swd_"); exetyp selects the execution type within the key.
type swdsLoader struct {
	ctx                    context.Context
	q                      adt.QueryClient
	prefix                 string // "swds" | "swd_"
	wfdID, version, exetyp string
	key                    string // WHERE clause on the definition key
}

func newLoader(ctx context.Context, q adt.QueryClient, prefix, exetyp, wfdID, version string) swdsLoader {
	return swdsLoader{ctx: ctx, q: q, prefix: prefix, wfdID: wfdID, version: version, exetyp: exetyp,
		key: fmt.Sprintf("WHERE wfd_id = '%s' AND version = '%s' AND exetyp = '%s'", wfdID, version, exetyp)}
}

// table returns the table name for a suffix: "text" → "swdstext" or "swd_text".
func (l swdsLoader) table(suffix string) string { return l.prefix + suffix }

// load reads the header, nodes, lines, texts, conditions, expressions and,
// with withContainer, bindings and container rows into m. nodes and lines
// read the node model of the definition.
func (l swdsLoader) load(m *rawModel, withContainer bool, nodes func() ([]rawNode, error), lines func() ([]rawLine, error)) error {
	language := m.Language
	hdr, err := l.rows("SELECT wfd_id, version, language, activat_on FROM " + l.table("header"))
	if err != nil {
		return err
	}
	if len(hdr) == 0 {
		return fmt.Errorf("%w: %s version %s in %s", ErrNotFound, l.wfdID, l.version, strings.ToUpper(l.table("header")))
	}
	m.MasterLanguage, m.ActivatedOn = hdr[0]["LANGUAGE"], hdr[0]["ACTIVAT_ON"]

	if m.Nodes, err = nodes(); err != nil {
		return err
	}
	if m.Lines, err = lines(); err != nil {
		return err
	}
	if m.Texts, err = l.texts(language, m.MasterLanguage, m.Nodes); err != nil {
		return err
	}
	if m.Conds, err = l.rows("SELECT wfd_id, version, nodeid, line_id, line_typ, leftexpr, compop, rightexpr, logop, notflag FROM " + l.table("condef")); err != nil {
		return err
	}
	if m.Exprs, err = l.exprs(); err != nil {
		return err
	}
	if withContainer {
		if m.Bindings, err = l.rows("SELECT wfd_id, version, nodeid, bindtype, dataflow, element, expr FROM " + l.table("bindef")); err != nil {
			return err
		}
		if m.CntIdx, err = l.rows("SELECT otype, objid, wfd_id, version, element, typeid, catid, typekind, typename FROM " + l.table("cntidx")); err != nil {
			return err
		}
		if m.ContXML, err = l.container(); err != nil {
			return err
		}
		if m.ElemTexts, err = l.elementTexts(language, m.MasterLanguage); err != nil {
			return err
		}
	}
	m.TaskTexts, err = loadTaskTexts(l.ctx, l.q, m.Nodes)
	return err
}

// rows runs "<sel> <key> <extra>" and keeps only rows of this definition
// version and execution type.
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
		if e, ok := r["EXETYP"]; ok && e != l.exetyp {
			continue
		}
		if r["WFD_ID"] == l.wfdID && r["VERSION"] == l.version {
			out = append(out, r)
		}
	}
	return out, nil
}

func (l swdsLoader) nodes() ([]rawNode, error) {
	t := l.table("mnodes")
	rows, err := l.rows("SELECT wfd_id, version, nodeid, nodetype, " + t + "~task, blockid, par_blckid," +
		" cont_cmd, return, returncode, exc_clsnam, min_tokens, exp_tokens, evt_name, evt_type, evt_otype, case_conte" +
		" FROM " + t)
	if err != nil {
		return nil, err
	}
	nodes := make([]rawNode, 0, len(rows))
	for _, r := range rows {
		nodes = append(nodes, rawNode{
			ID: r["NODEID"], Type: r["NODETYPE"], Task: r["TASK"],
			BlockID: r["BLOCKID"], ParentBlockID: r["PAR_BLCKID"], ContCmd: r["CONT_CMD"],
			Return: r["RETURN"], ReturnCode: r["RETURNCODE"], ExcClass: r["EXC_CLSNAM"],
			MinTokens: r["MIN_TOKENS"], ExpTokens: r["EXP_TOKENS"], EvtName: r["EVT_NAME"],
			EvtType: r["EVT_TYPE"], EvtOType: r["EVT_OTYPE"], CaseConte: r["CASE_CONTE"],
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, nil
}

func (l swdsLoader) lines() ([]rawLine, error) {
	rows, err := l.rows("SELECT wfd_id, version, pred_node, succ_node FROM " + l.table("mlines"))
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

// texts reads the text table (SWDSTEXT / SWD_TEXT) in the requested and master language; if that yields no
// node name (ND) for any of the loaded nodes, it reads every language.
func (l swdsLoader) texts(language, master string, nodes []rawNode) ([]Row, error) {
	known := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		known[trimID(n.ID)] = true
	}
	langs := map[string]bool{language: true}
	list := "'" + language + "'"
	if languageRe.MatchString(master) && master != language {
		langs[master] = true
		list += ", '" + master + "'"
	}
	sel := "SELECT wfd_id, version, nodeid, language, texttyp, textnumber, wfd_text FROM " + l.table("text")
	rows, err := l.rows(sel, "AND language IN ("+list+")")
	if err != nil {
		return nil, err
	}
	filtered := []Row{}
	hasND := false
	for _, r := range rows {
		if langs[r["LANGUAGE"]] {
			filtered = append(filtered, r)
			hasND = hasND || (r["TEXTTYP"] == "ND" && r["WFD_TEXT"] != "" && known[trimID(r["NODEID"])])
		}
	}
	if hasND {
		return filtered, nil
	}
	return l.rows(sel)
}

func (l swdsLoader) exprs() ([]Row, error) {
	rows, err := l.rows("SELECT wfd_id, version, nodeid, exprtype, expression FROM "+l.table("expr"),
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
	rows, err := l.rows("SELECT wfd_id, version, nodeid, cnt_type, tabix, length, data FROM "+l.table("cont"),
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

// elementTexts reads the workflow container element texts (SWDSWFCTXT / SWD_WFCTXT, node 0,
// empty CNT_TYPE) in the requested and master language; if there are none, it
// reads every language.
func (l swdsLoader) elementTexts(language, master string) ([]Row, error) {
	langs := map[string]bool{language: true}
	list := "'" + language + "'"
	if languageRe.MatchString(master) && master != language {
		langs[master] = true
		list += ", '" + master + "'"
	}
	sel := "SELECT wfd_id, version, nodeid, cnt_type, language, element, descript, shorttext FROM " + l.table("wfctxt")
	const where = "AND nodeid = '0000000000' AND cnt_type = ' '"
	keep := func(rows []Row, all bool) []Row {
		out := []Row{}
		for _, r := range rows {
			if trimID(r["NODEID"]) == "" && r["CNT_TYPE"] == "" && (all || langs[r["LANGUAGE"]]) {
				out = append(out, r)
			}
		}
		return out
	}
	rows, err := l.rows(sel, where, "AND language IN ("+list+")")
	if err != nil {
		return nil, err
	}
	if out := keep(rows, false); len(out) > 0 {
		return out, nil
	}
	if rows, err = l.rows(sel, where); err != nil {
		return nil, err
	}
	return keep(rows, true), nil
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
