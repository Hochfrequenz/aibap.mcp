package bwf

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func containerFixture(t *testing.T) *rawModel {
	t.Helper()
	return loadModel(t, loadFixture(t, "ws48900002"), "WS48900002", "E")
}

func elementByName(t *testing.T, els []ContainerElement, name string) ContainerElement {
	t.Helper()
	for _, e := range els {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("element %q not found in %+v", name, els)
	return ContainerElement{}
}

func TestContainer_Elements(t *testing.T) {
	els, warnings, err := decodeContainer(containerFixture(t))
	if err != nil {
		t.Fatalf("decodeContainer: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	var names []string
	for _, e := range els {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "ABSENCFORM,COUNTER,FLAG,RESULT" {
		t.Errorf("names = %s", got)
	}
	a := elementByName(t, els, "ABSENCFORM")
	if a.Type != "FORMABSENC" || a.Category != "BO" || a.TypeKind != "u" || a.Props != "0C920201" {
		t.Errorf("ABSENCFORM = %+v", a)
	}
	r := elementByName(t, els, "RESULT")
	if r.DDICType != "WFSYST" || r.TypeKind != "C" || r.Props != "0C002201" {
		t.Errorf("RESULT = %+v", r)
	}
}

// The PROPS bits come from type pool SWFCN (swfcn_p_param_import = property 20,
// swfcn_p_param_export = 21, swfcn_p_param_optional = 23; SWFUT numbers bits
// from the most significant bit). None of the WS48900002 elements carries the
// import or export bit — the workflow's task container holds no elements either.
func TestContainer_Flags(t *testing.T) {
	els, _, err := decodeContainer(containerFixture(t))
	if err != nil {
		t.Fatalf("decodeContainer: %v", err)
	}
	for _, e := range els {
		if e.Import || e.Export || e.Mandatory {
			t.Errorf("%s: import=%v export=%v mandatory=%v, want all false", e.Name, e.Import, e.Export, e.Mandatory)
		}
	}
}

func TestParseProps(t *testing.T) {
	tests := []struct {
		props                          string
		import_, export, mandatory, ok bool
	}{
		{"0C920201", false, false, false, true},           // object element, optional, no parameter
		{"0C002201", false, false, false, true},           // like-typed element, optional
		{"0C921001", true, false, true, true},             // mandatory import (BOR reference)
		{"0CC61001C", true, false, true, true},            // odd length: trailing zero nibbles are dropped
		{"0C001A01", true, true, false, true},             // optional import + export
		{"0C000801", false, true, false, true},            // export only
		{"8C0002", false, false, false, true},             // short: right-padded
		{"", false, false, false, true},                   // no props
		{"ZZ", false, false, false, false},                // not hex
		{"00000000000000000", false, false, false, false}, // longer than 64 bits
	}
	for _, tt := range tests {
		imp, exp, man, ok := parseProps(tt.props)
		if imp != tt.import_ || exp != tt.export || man != tt.mandatory || ok != tt.ok {
			t.Errorf("parseProps(%q) = %v,%v,%v,%v want %v,%v,%v,%v",
				tt.props, imp, exp, man, ok, tt.import_, tt.export, tt.mandatory, tt.ok)
		}
	}
}

func TestContainer_NoChangerInOutput(t *testing.T) {
	els, _, err := decodeContainer(containerFixture(t))
	if err != nil {
		t.Fatalf("decodeContainer: %v", err)
	}
	b, err := json.Marshal(els)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SAPUSER") {
		t.Errorf("output contains changer: %s", b)
	}
}

func TestContainer_MissingXML(t *testing.T) {
	m := containerFixture(t)
	m.ContXML = nil
	els, warnings, err := decodeContainer(m)
	if err != nil {
		t.Fatalf("decodeContainer: %v", err)
	}
	if len(els) != 4 {
		t.Errorf("elements = %d, want 4", len(els))
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want 1", warnings)
	}
	if a := elementByName(t, els, "ABSENCFORM"); a.Type != "FORMABSENC" || a.Props != "" {
		t.Errorf("ABSENCFORM = %+v", a)
	}
}

func TestContainer_CorruptHex(t *testing.T) {
	m := containerFixture(t)
	m.ContXML = []Row{{"TABIX": "000001", "LENGTH": "4", "DATA": "XYZ1"}}
	_, _, err := decodeContainer(m)
	if err == nil || !strings.Contains(err.Error(), "SWDSCONT") {
		t.Fatalf("err = %v, want error mentioning SWDSCONT", err)
	}
}

func hexRows(xml string, chunk int) []Row {
	var rows []Row
	for i := 0; i*chunk < len(xml); i++ {
		end := (i + 1) * chunk
		if end > len(xml) {
			end = len(xml)
		}
		part := []byte(xml[i*chunk : end])
		padded := append(append([]byte{}, part...), []byte("    ")...)
		rows = append(rows, Row{
			"TABIX":  fmt.Sprintf("%06d", i+1),
			"LENGTH": strconv.Itoa(len(part)),
			"DATA":   strings.ToUpper(hex.EncodeToString(padded)),
		})
	}
	return rows
}

func TestContainer_XMLOnlyElementAndHeapValue(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">` +
		`<asx:values><CONTAINER href="#o1"/></asx:values><asx:heap xmlns:xsd="http://www.w3.org/2001/XMLSchema" ` +
		`xmlns:cls="http://www.sap.com/abapxml/classes/global"><cls:CL_SWF_CNT_CONTAINER id="o1"><CL_SWF_CNT_CONTAINER>` +
		`<META><LTEXTS/><CHGDTA>x:SAPUSER</CHGDTA></META><ELMT>` +
		`<SWFDELMDEFXML><NAME>COUNTER</NAME><VALUE>7</VALUE><TYPE>::SYST-INDEX:I:0:0</TYPE><PROPS>0C001001</PROPS>` +
		`<LTEXTS>Loop counter</LTEXTS><CHGDTA>620:1:SAPUSER::0:</CHGDTA></SWFDELMDEFXML>` +
		`<SWFDELMDEFXML><NAME>EXTRA</NAME><VALUE href="#d2"/><TYPE>::WFSYST-RESULT:C:255:0</TYPE><PROPS>0C000801</PROPS></SWFDELMDEFXML>` +
		`</ELMT></CL_SWF_CNT_CONTAINER></cls:CL_SWF_CNT_CONTAINER><xsd:string id="d2">  done </xsd:string></asx:heap></asx:abap>`
	m := &rawModel{
		CntIdx:  []Row{{"OTYPE": "WS", "ELEMENT": "COUNTER", "TYPEKIND": "I", "TYPENAME": "SYST"}, {"OTYPE": "TS", "ELEMENT": "OTHER"}},
		ContXML: hexRows(xml, 100),
	}
	els, warnings, err := decodeContainer(m)
	if err != nil {
		t.Fatalf("decodeContainer: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(els) != 2 {
		t.Fatalf("elements = %+v, want COUNTER and EXTRA", els)
	}
	c := elementByName(t, els, "COUNTER")
	if c.InitialValue != "7" || c.Description != "Loop counter" || !c.Import || !c.Mandatory || c.DDICType != "SYST" {
		t.Errorf("COUNTER = %+v", c)
	}
	x := elementByName(t, els, "EXTRA")
	if x.InitialValue != "done" || x.DDICType != "WFSYST-RESULT" || x.TypeKind != "C" || !x.Export || x.Import {
		t.Errorf("EXTRA = %+v", x)
	}
}

func TestContainer_EmptyModel(t *testing.T) {
	els, warnings, err := decodeContainer(&rawModel{})
	if err != nil {
		t.Fatal(err)
	}
	if els == nil || len(els) != 0 {
		t.Errorf("elements = %#v, want empty non-nil", els)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none for an empty container", warnings)
	}
}

func TestContainer_DescriptionFromWFCTXT(t *testing.T) {
	tables := loadFixture(t, "ws48900002")
	for _, tc := range []struct{ lang, want string }{
		{"E", "Form for Absence"},
		{"D", "Formular zur Abwesenheit"},
	} {
		els, _, err := decodeContainer(loadModel(t, tables, "WS48900002", tc.lang))
		if err != nil {
			t.Fatalf("decodeContainer(%s): %v", tc.lang, err)
		}
		if got := elementByName(t, els, "ABSENCFORM").Description; got != tc.want {
			t.Errorf("language %s: description = %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func TestContainer_DescriptionLanguageFallback(t *testing.T) {
	tables := loadFixture(t, "ws48900002")
	// No French texts: the master language (from SWDSHEADER) is used.
	m := loadModel(t, tables, "WS48900002", "F")
	els, _, err := decodeContainer(m)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"D": "Formular zur Abwesenheit", "E": "Form for Absence"}[m.MasterLanguage]
	if got := elementByName(t, els, "ABSENCFORM").Description; got != want {
		t.Errorf("description = %q, want master-language text %q", got, want)
	}

	// Neither requested nor master language present: any language is used.
	var onlyD []Row
	for _, r := range tables["SWDSWFCTXT"] {
		if r["LANGUAGE"] == "D" {
			onlyD = append(onlyD, r)
		}
	}
	tables["SWDSWFCTXT"] = onlyD
	hdr := make([]Row, 0, len(tables["SWDSHEADER"]))
	for _, r := range tables["SWDSHEADER"] {
		c := Row{}
		for k, v := range r {
			c[k] = v
		}
		c["LANGUAGE"] = "E"
		hdr = append(hdr, c)
	}
	tables["SWDSHEADER"] = hdr
	els, _, err = decodeContainer(loadModel(t, tables, "WS48900002", "F"))
	if err != nil {
		t.Fatal(err)
	}
	if got := elementByName(t, els, "COUNTER").Description; got != "Schleifenzähler" {
		t.Errorf("description = %q, want any-language text", got)
	}
}

func TestContainer_DescriptionShortTextFallsBackToDescript(t *testing.T) {
	m := &rawModel{
		Language: "E",
		CntIdx:   []Row{{"OTYPE": "WS", "ELEMENT": "FLAG"}},
		ElemTexts: []Row{
			{"LANGUAGE": "E", "ELEMENT": "FLAG", "DESCRIPT": "Flag", "SHORTTEXT": ""},
		},
	}
	els, _, err := decodeContainer(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := els[0].Description; got != "Flag" {
		t.Errorf("description = %q, want DESCRIPT", got)
	}
}
