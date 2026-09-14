package uacheck

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The clauses the structure editor exists to satisfy — `PLAN-accessibility.md` P09.S05.

// semanticDoc is a one-page tagged document whose structure tree holds objs (numbered as given), with
// rootKids directly under the root and roleMap as the RoleMap body, or none.
func semanticDoc(objs map[int]string, rootKids []int, roleMap string) []byte {
	m := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 44 >>\nstream\nBT /F1 24 Tf 72 700 Td (Untagged) Tj ET\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var kids []string
	for _, k := range rootKids {
		kids = append(kids, fmt.Sprintf("%d 0 R", k))
	}
	rm := ""
	if roleMap != "" {
		rm = " /RoleMap << " + roleMap + " >>"
	}
	m[7] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s]%s >>", strings.Join(kids, " "), rm)
	for n, o := range objs {
		m[n] = o
	}
	return buildPDF(m)
}

// semanticCase is one hand-built document and nib's expected verdict on one clause. The same table is
// the unit test's and the oracle comparison's, so an expectation is never nib's reading alone.
type semanticCase struct {
	name   string
	clause string
	pdf    []byte
	want   Verdict
}

func figure(extra string) string { return "<< /Type /StructElem /S /Figure /P 7 0 R" + extra + " >>" }

// figureCases are 7.3 t1's branches.
func figureCases() []semanticCase {
	one := func(obj string) []byte { return semanticDoc(map[int]string{10: obj}, []int{10}, "") }
	return []semanticCase{
		{"alt text", "7.3 t1", one(figure(" /Alt (A chart)")), Pass},
		{"alt text written as a hex string", "7.3 t1", one(figure(" /Alt <FEFF0041>")), Pass},
		{"an empty alt", "7.3 t1", one(figure(" /Alt ()")), Fail},
		{"replacement text only", "7.3 t1", one(figure(" /ActualText (42)")), Pass},
		{"neither", "7.3 t1", one(figure("")), Fail},
		{"the second of two figures has neither", "7.3 t1", semanticDoc(map[int]string{10: figure(" /Alt (a)"), 11: figure("")}, []int{10, 11}, ""), Fail},
		{"a figure under a section", "7.3 t1", semanticDoc(map[int]string{
			10: "<< /Type /StructElem /S /Sect /P 7 0 R /K [11 0 R] >>",
			11: "<< /Type /StructElem /S /Figure /P 10 0 R >>",
		}, []int{10}, ""), Fail},
		{"a role-mapped figure", "7.3 t1", semanticDoc(map[int]string{10: "<< /Type /StructElem /S /Img /P 7 0 R >>"}, []int{10}, "/Img /Figure"), Fail},
		{"no figure at all", "7.3 t1", one("<< /Type /StructElem /S /P /P 7 0 R >>"), NotApplicable},
	}
}

// tableDoc is a document holding one table whose rows are cell specs: a type (TH, TD, or a custom name
// the role map resolves), and after a colon a Scope (Column, Row, Both), Headers, ColSpan, or
// LayoutScope (a Scope under a Layout attribute object). grouped puts the first row under THead and
// the rest under TBody.
func tableDoc(rows [][]string, grouped bool, roleMap string) []byte {
	objs := map[int]string{}
	next := 11
	alloc := func() int { n := next; next++; return n }
	row := func(parent int, spec []string) int {
		tr := alloc()
		var cells []string
		for _, c := range spec {
			n := alloc()
			kind, attr, _ := strings.Cut(c, ":")
			a := ""
			switch attr {
			case "Column", "Row", "Both":
				a = " /A << /O /Table /Scope /" + attr + " >>"
			case "Headers":
				a = " /A << /O /Table /Headers [(h1)] >>"
			case "ColSpan":
				a = " /A << /O /Table /ColSpan 2 >>"
			case "LayoutScope":
				a = " /A << /O /Layout /Scope /Column >>"
			case "BadScope":
				a = " /A << /O /Table /Scope /Diagonal >>"
			case "SpanHeaders":
				a = " /A << /O /Table /ColSpan 2 /Headers [(h1)] >>"
			}
			objs[n] = fmt.Sprintf("<< /Type /StructElem /S /%s /P %d 0 R%s >>", kind, tr, a)
			cells = append(cells, fmt.Sprintf("%d 0 R", n))
		}
		objs[tr] = fmt.Sprintf("<< /Type /StructElem /S /TR /P %d 0 R /K [%s] >>", parent, strings.Join(cells, " "))
		return tr
	}
	var top []string
	if grouped {
		head, body := alloc(), alloc()
		h := row(head, rows[0])
		var bodyRows []string
		for _, spec := range rows[1:] {
			bodyRows = append(bodyRows, fmt.Sprintf("%d 0 R", row(body, spec)))
		}
		objs[head] = fmt.Sprintf("<< /Type /StructElem /S /THead /P 10 0 R /K [%d 0 R] >>", h)
		objs[body] = fmt.Sprintf("<< /Type /StructElem /S /TBody /P 10 0 R /K [%s] >>", strings.Join(bodyRows, " "))
		top = []string{fmt.Sprintf("%d 0 R", head), fmt.Sprintf("%d 0 R", body)}
	} else {
		for _, spec := range rows {
			top = append(top, fmt.Sprintf("%d 0 R", row(10, spec)))
		}
	}
	objs[10] = fmt.Sprintf("<< /Type /StructElem /S /Table /P 7 0 R /K [%s] >>", strings.Join(top, " "))
	return semanticDoc(objs, []int{10}, roleMap)
}

// tableCases are the table shapes veraPDF was measured on (P09.S05's build block names them all), with
// nib's answer: every header scoped, or none present, passes; an unscoped header over a data cell that
// names no headers cannot be checked.
func tableCases() []semanticCase {
	tc := func(name string, rows [][]string, want Verdict) semanticCase {
		return semanticCase{name, "7.5 t1", tableDoc(rows, false, ""), want}
	}
	return []semanticCase{
		// Every TH scoped — passes wherever the headers sit.
		tc("column headers", [][]string{{"TH:Column", "TH:Column"}, {"TD", "TD"}}, Pass),
		tc("row headers", [][]string{{"TH:Row", "TD"}, {"TH:Row", "TD"}}, Pass),
		tc("row-scoped headers above", [][]string{{"TH:Row", "TH:Row"}, {"TD", "TD"}}, Pass),
		tc("a column-scoped header before the cell", [][]string{{"TH:Column", "TD"}}, Pass),
		tc("headers below the cells", [][]string{{"TD", "TD"}, {"TH:Column", "TH:Column"}}, Pass),
		tc("a row header after the cell", [][]string{{"TD", "TH:Row"}}, Pass),
		tc("Both in a corner with an unheaded cell", [][]string{{"TH:Both", "TD"}, {"TD", "TD"}}, Pass),
		tc("Both, with the rest headed", [][]string{{"TH:Both", "TH:Column"}, {"TH:Row", "TD"}}, Pass),
		tc("an extra column with no header", [][]string{{"TH:Column", "TH:Column"}, {"TD", "TD", "TD"}}, Pass),
		tc("one row header over three rows", [][]string{{"TH:Row", "TD"}, {"TD", "TD"}, {"TD", "TD"}}, Pass),
		{"THead and TBody", "7.5 t1", tableDoc([][]string{{"TH:Column", "TH:Column"}, {"TD", "TD"}, {"TD", "TD"}}, true, ""), Pass},
		// No TH at all — passes.
		tc("no header cells", [][]string{{"TD", "TD"}, {"TD", "TD"}}, Pass),
		tc("a Scope on a data cell only", [][]string{{"TD:Column", "TD"}, {"TD", "TD"}}, Pass),
		tc("Headers attributes", [][]string{{"TD:Headers", "TD:Headers"}}, Pass),
		// An unscoped TH over a data cell that names no headers — veraPDF's verdict turns on its algorithm.
		tc("header cells with no scope", [][]string{{"TH", "TH"}, {"TD", "TD"}}, CannotCheck),
		tc("one scoped, one not", [][]string{{"TH:Column", "TH"}, {"TD", "TD"}}, CannotCheck),
		tc("one not, one scoped", [][]string{{"TH", "TH:Column"}, {"TD", "TD"}}, CannotCheck),
		tc("a Scope under a Layout attribute object", [][]string{{"TH:LayoutScope", "TH:LayoutScope"}, {"TD", "TD"}}, CannotCheck),
		tc("Headers beside an unheaded cell", [][]string{{"TH", "TH"}, {"TD:Headers", "TD"}}, CannotCheck),
		tc("a header grid with no scopes", [][]string{{"TH", "TH", "TH"}, {"TH", "TD", "TD"}, {"TH", "TD", "TD"}}, CannotCheck),
		tc("a lone unscoped header mid-table", [][]string{{"TD", "TD", "TD"}, {"TD", "TH", "TD"}, {"TD", "TD", "TD"}}, CannotCheck),
		{"a role-mapped data cell under unscoped headers", "7.5 t1", tableDoc([][]string{{"TH", "TH"}, {"Cell", "Cell"}}, false, "/Cell /TD"), CannotCheck},
		// Unscoped headers, but every data cell names its headers — nothing is left to infer.
		tc("unscoped headers, every cell naming its headers", [][]string{{"TH", "TH"}, {"TD:Headers", "TD:Headers"}}, Pass),
		tc("a Scope that names no direction", [][]string{{"TH:BadScope", "TH:BadScope"}, {"TD", "TD"}}, CannotCheck),
		tc("a column span", [][]string{{"TH:Column", "TH:Column"}, {"TD:ColSpan"}}, CannotCheck),
		tc("a column span on a cell naming its headers", [][]string{{"TH:Column", "TH:Column"}, {"TD:SpanHeaders"}}, Pass),
		{"no data cells", "7.5 t1", semanticDoc(map[int]string{10: "<< /Type /StructElem /S /P /P 7 0 R >>"}, []int{10}, ""), NotApplicable},
		{"a TD outside any table", "7.5 t1", semanticDoc(map[int]string{10: "<< /Type /StructElem /S /TD /P 7 0 R >>"}, []int{10}, ""), CannotCheck},
	}
}

// TestAFigureNeedsAltTextOrReplacementText — 7.3 t1, each branch.
func TestAFigureNeedsAltTextOrReplacementText(t *testing.T) {
	for _, c := range figureCases() {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != c.want {
			t.Errorf("%s: %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
		if got.Verdict == Fail && (got.Why == "" || !strings.Contains(got.Where, "Figure") && !strings.Contains(got.Where, "Img")) {
			t.Errorf("%s: a failure must say which element: why %q where %q", c.name, got.Why, got.Where)
		}
	}
}

// TestATableIsCheckedOnlyWhereVeraPDFsAnswerIsKnown — 7.5 t1, each shape.
func TestATableIsCheckedOnlyWhereVeraPDFsAnswerIsKnown(t *testing.T) {
	for _, c := range tableCases() {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != c.want {
			t.Errorf("%s: %v (%s at %s), want %v", c.name, got.Verdict, got.Why, got.Where, c.want)
		}
		if got.Verdict == CannotCheck && (got.Why == "" || got.Where == "") {
			t.Errorf("%s: an unsettled table must say why and where: %q at %q", c.name, got.Why, got.Where)
		}
		if strings.Contains(c.name, "no scope") && got.Verdict == CannotCheck && !strings.Contains(got.Where, "header at row 1, cell 1") {
			t.Errorf("%s: the report must name the header cell with no Scope, and says %q", c.name, got.Where)
		}
	}
}

// TestTheFigureAndTableCasesAgreeWithVeraPDF — law 5 over the hand-built cases above, on their own
// clause. The oracle corpus holds LibreOffice's table; these are the shapes LibreOffice does not write,
// so without this every shape past its own would be nib's reading with nothing checking it.
func TestTheFigureAndTableCasesAgreeWithVeraPDF(t *testing.T) {
	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the hand-built 7.3 t1 and 7.5 t1 cases are UNCHECKED against the oracle in this run.")
	}
	cases := append(figureCases(), tableCases()...)
	dir := t.TempDir()
	files := make([]string, len(cases))
	for i, c := range cases {
		files[i] = filepath.Join(dir, fmt.Sprintf("case%02d.pdf", i))
		if err := os.WriteFile(files[i], c.pdf, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, files...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v\n%.500s", err, out)
	}
	vera := veraStates(rep, files)
	settled := 0
	for i, c := range cases {
		if vera[i] == nil {
			t.Errorf("%s: veraPDF returned no job for it", c.name)
			continue
		}
		state, listed := vera[i][c.clause]
		if !listed {
			t.Errorf("%s: veraPDF's report does not list %s", c.name, c.clause)
			continue
		}
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != CannotCheck {
			settled++
		}
		if !state.agrees(got.Verdict) {
			t.Errorf("%s: %s — veraPDF %s, nib %v (%s)", c.name, c.clause, state, got.Verdict, got.Why)
		}
	}
	// CannotCheck agrees with anything, so a rule that answered it everywhere would pass the loop above.
	if settled < 20 {
		t.Errorf("only %d case(s) settled either way — the agreement above is mostly CannotCheck agreeing with everything", settled)
	}
}
