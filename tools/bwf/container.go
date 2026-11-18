package bwf

import (
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Container element PROPS bits (type pool SWFCN, bit numbers from SWFUT,
// counted from the most significant bit of the 64-bit SWFDBITS64 word).
const (
	propParamImport   uint64 = 1 << (64 - 20) // swfcn_p_param_import
	propParamExport   uint64 = 1 << (64 - 21) // swfcn_p_param_export
	propParamOptional uint64 = 1 << (64 - 23) // swfcn_p_param_optional
)

// elemTag is the asXML tag of one container element definition.
const elemTag = "SWFDELMDEFXML"

// xmlElement is one SWFDELMDEFXML entry of the container asXML. CHGDTA is
// deliberately not captured: it carries the last changer's logon ID.
type xmlElement struct {
	Name, Value, ValueHref, Type, Props, LTexts string
}

// decodeContainer builds the workflow container element list from the
// container index rows (SWDSCNTIDX / SWD_CNTIDX, OTYPE 'WS'), enriched from the
// container XML (SWDSCONT / SWD_CONT). Elements only present in the XML are
// added unless their NAME is empty. Missing XML yields the index elements and
// a warning; undecodable hex DATA is an error.
func decodeContainer(m *rawModel) ([]ContainerElement, []string, error) {
	cont := contTable(m)
	warnings := []string{}
	byName := map[string]*ContainerElement{}
	for _, r := range m.CntIdx {
		if r["OTYPE"] != "WS" || r["ELEMENT"] == "" {
			continue
		}
		byName[r["ELEMENT"]] = &ContainerElement{
			Name: r["ELEMENT"], Type: r["TYPEID"], Category: r["CATID"],
			TypeKind: r["TYPEKIND"], DDICType: r["TYPENAME"],
		}
	}

	if len(m.ContXML) == 0 {
		if len(byName) > 0 {
			warnings = append(warnings, fmt.Sprintf("container XML (%s) not found: element descriptions, flags and initial values unavailable", cont))
		}
		return sortedElements(byName, m), warnings, nil
	}
	raw, err := concatContainerData(cont, m.ContXML)
	if err != nil {
		return nil, nil, err
	}
	xmlEls, err := parseContainerXML(raw)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("container XML (%s) not parseable: %v", cont, err))
		return sortedElements(byName, m), warnings, nil
	}

	for _, x := range xmlEls {
		if x.Name == "" {
			continue
		}
		e, ok := byName[x.Name]
		if !ok {
			e = elementFromType(x.Name, x.Type)
			byName[x.Name] = e
		}
		e.Description = x.LTexts
		e.InitialValue = x.Value
		e.Props = x.Props
		imp, exp, man, ok := parseProps(x.Props)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("container element %s: flags not decoded (PROPS %q)", x.Name, x.Props))
		}
		e.Import, e.Export, e.Mandatory = imp, exp, man
	}
	return sortedElements(byName, m), warnings, nil
}

// applyElementTexts sets each element's description from SWDSWFCTXT, preferring
// the requested language, then the master language, then any language (lowest
// language key). SHORTTEXT is used, DESCRIPT when it is empty. It overrides LTEXTS.
func applyElementTexts(byName map[string]*ContainerElement, m *rawModel) {
	rank := func(lang string) int {
		switch lang {
		case m.Language:
			return 0
		case m.MasterLanguage:
			return 1
		}
		return 2
	}
	best := map[string]Row{}
	for _, r := range m.ElemTexts {
		text := r["SHORTTEXT"]
		if text == "" {
			text = r["DESCRIPT"]
		}
		if text == "" || byName[r["ELEMENT"]] == nil {
			continue
		}
		b, ok := best[r["ELEMENT"]]
		if !ok || rank(r["LANGUAGE"]) < rank(b["LANGUAGE"]) ||
			(rank(r["LANGUAGE"]) == rank(b["LANGUAGE"]) && r["LANGUAGE"] < b["LANGUAGE"]) {
			best[r["ELEMENT"]] = Row{"LANGUAGE": r["LANGUAGE"], "TEXT": text}
		}
	}
	for name, b := range best {
		byName[name].Description = b["TEXT"]
	}
}

// sortedElements applies the element texts and returns the elements by name.
func sortedElements(byName map[string]*ContainerElement, m *rawModel) []ContainerElement {
	applyElementTexts(byName, m)
	out := make([]ContainerElement, 0, len(byName))
	for _, e := range byName {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// contTable names the container XML table the model was read from.
func contTable(m *rawModel) string {
	if m.TablePrefix == "" {
		return "SWDSCONT"
	}
	return m.TablePrefix + "CONT"
}

// concatContainerData hex-decodes each row's DATA, keeps its first LENGTH
// bytes, concatenates the rows in TABIX order and trims trailing padding.
// table names the source table in errors.
func concatContainerData(table string, rows []Row) ([]byte, error) {
	sorted := append([]Row(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i]["TABIX"] < sorted[j]["TABIX"] })
	var buf bytes.Buffer
	for _, r := range sorted {
		b, err := hex.DecodeString(r["DATA"])
		if err != nil {
			return nil, fmt.Errorf("%s TABIX %s: invalid hex DATA: %w", table, r["TABIX"], err)
		}
		n, err := strconv.Atoi(strings.TrimSpace(r["LENGTH"]))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s TABIX %s: invalid LENGTH %q", table, r["TABIX"], r["LENGTH"])
		}
		if n > len(b) {
			n = len(b)
		}
		buf.Write(b[:n])
	}
	return bytes.TrimRight(buf.Bytes(), " \x00"), nil
}

// frame is one open XML element during parseContainerXML.
type frame struct {
	name, id, href string
	text           strings.Builder
	hasChild       bool
}

// parseContainerXML walks the asXML namespace-agnostically. It collects the
// SWFDELMDEFXML entries and resolves VALUE href="#id" references to the text
// of the referenced heap leaf element.
func parseContainerXML(data []byte) ([]xmlElement, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*frame
	var els []xmlElement
	var cur *xmlElement
	heap := map[string]string{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 0 {
				stack[len(stack)-1].hasChild = true
			}
			f := &frame{name: t.Name.Local}
			for _, a := range t.Attr {
				switch a.Name.Local {
				case "id":
					f.id = a.Value
				case "href":
					f.href = a.Value
				}
			}
			stack = append(stack, f)
			if f.name == elemTag {
				cur = &xmlElement{}
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unbalanced element %s", t.Name.Local)
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.id != "" && !f.hasChild {
				heap[f.id] = strings.TrimSpace(f.text.String())
			}
			if f.name == elemTag && cur != nil {
				els = append(els, *cur)
				cur = nil
				continue
			}
			if cur != nil && len(stack) > 0 && stack[len(stack)-1].name == elemTag {
				setXMLField(cur, f)
			}
		}
	}
	for i := range els {
		if els[i].Value == "" && strings.HasPrefix(els[i].ValueHref, "#") {
			els[i].Value = heap[strings.TrimPrefix(els[i].ValueHref, "#")]
		}
	}
	return els, nil
}

func setXMLField(e *xmlElement, f *frame) {
	text := strings.TrimSpace(f.text.String())
	switch f.name {
	case "NAME":
		e.Name = text
	case "VALUE":
		e.Value, e.ValueHref = text, f.href
	case "TYPE":
		e.Type = text
	case "PROPS":
		e.Props = text
	case "LTEXTS":
		e.LTexts = text
	}
}

// elementFromType builds an element known only from the XML, splitting TYPE
// "<objtype>:<catid>:<ddic>:<kind>:<len>:<dec>".
func elementFromType(name, typ string) *ContainerElement {
	parts := strings.Split(typ, ":")
	get := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	return &ContainerElement{Name: name, Type: get(0), Category: get(1), DDICType: get(2), TypeKind: get(3)}
}

// parseProps decodes the PROPS hex word. SAP serialises the 64-bit word with
// trailing zero nibbles dropped, so the string is right-padded to 16 digits.
// Mandatory is an import parameter without the optional bit. ok is false
// when PROPS is not a hex word of at most 64 bits.
func parseProps(props string) (imp, exp, mandatory, ok bool) {
	if props == "" {
		return false, false, false, true
	}
	if len(props) > 16 {
		return false, false, false, false
	}
	v, err := strconv.ParseUint(props+strings.Repeat("0", 16-len(props)), 16, 64)
	if err != nil {
		return false, false, false, false
	}
	imp = v&propParamImport != 0
	exp = v&propParamExport != 0
	mandatory = imp && v&propParamOptional == 0
	return imp, exp, mandatory, true
}
