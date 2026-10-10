package uacheck

import (
	"testing"

	"nib/internal/pdfops"
)

// A table corrected through the structure editor's cell edits (ADR-119) is judged the same by nib and by
// veraPDF, before and after each edit: a first row one cell wide becomes the table's width with a ColSpan,
// a short second row is filled by a RowSpan above it, and data cells no scoped header reaches are connected
// by the `/Headers` the edit writes — against identifiers nib allocated and entered in the `/IDTree`.
func TestATableCorrectedByCellEditsIsJudgedAsVeraPDFJudgesIt(t *testing.T) {
	doc := func(table map[int]string) []byte {
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
			4: "<< /Length 1 >>\nstream\n \nendstream",
			7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		}
		for n, body := range table {
			objs[n] = body
		}
		return buildPDF(objs)
	}
	edit := func(pdf []byte, edits ...pdfops.StructureEdit) []byte {
		t.Helper()
		out, err := pdfops.EditStructure(pdf, edits)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// A title cell over two columns, a header row, a data row.
	wide := doc(map[int]string{
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [9 0 R 10 0 R 11 0 R] >>",
		9:  "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [13 0 R 14 0 R] >>",
		11: "<< /Type /StructElem /S /TR /P 8 0 R /K [15 0 R 16 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 9 0 R >>",
		13: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		14: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		15: "<< /Type /StructElem /S /TD /P 11 0 R >>",
		16: "<< /Type /StructElem /S /TD /P 11 0 R >>",
	})
	spanned := edit(wide, pdfops.StructureEdit{Kind: "colspan", Element: 12, Value: "2"})
	headed := edit(spanned,
		pdfops.StructureEdit{Kind: "headers", Element: 15, Headers: []int{12, 13}},
		pdfops.StructureEdit{Kind: "headers", Element: 16, Headers: []int{12, 14}})
	half := edit(spanned, pdfops.StructureEdit{Kind: "headers", Element: 15, Headers: []int{13}})
	// A header cell two rows tall beside one data cell per row.
	tall := doc(map[int]string{
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [9 0 R 10 0 R] >>",
		9:  "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R 13 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 9 0 R /A << /O /Table /Scope /Row >> >>",
		13: "<< /Type /StructElem /S /TD /P 9 0 R >>",
		14: "<< /Type /StructElem /S /TD /P 10 0 R >>",
	})
	rowSpanned := edit(tall, pdfops.StructureEdit{Kind: "rowspan", Element: 12, Value: "2"})

	clauses := [6]string{"7.2 t15", "7.2 t41", "7.2 t42", "7.2 t43", "7.5 t1", "7.5 t2"}
	const P, F = Pass, Fail
	cases := []struct {
		name string
		pdf  []byte
		want [6]Verdict
	}{
		{"a first row one cell wide", wide, [6]Verdict{P, P, F, P, P, P}},
		{"its cell spanning two columns", spanned, [6]Verdict{P, P, P, P, F, P}},
		{"every data cell naming its headers", headed, [6]Verdict{P, P, P, P, P, P}},
		{"one data cell naming its header, one not", half, [6]Verdict{P, P, P, P, F, P}},
		{"a second row one cell short", tall, [6]Verdict{P, P, P, F, P, P}},
		{"the header beside it spanning two rows", rowSpanned, [6]Verdict{P, P, P, P, P, P}},
	}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, c.pdf)
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		for k, clause := range clauses {
			if got := verdictOf(t, c.pdf, clause); got.Verdict != c.want[k] {
				t.Errorf("%s: %s reports %v (%s), want %v", c.name, clause, got.Verdict, got.Why, c.want[k])
			}
			if vera != nil && vera[i][clause] != words[c.want[k]] {
				t.Errorf("%s: veraPDF says %q for %s, want %q", c.name, vera[i][clause], clause, words[c.want[k]])
			}
		}
	}
}
