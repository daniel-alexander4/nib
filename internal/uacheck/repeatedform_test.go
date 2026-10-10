package uacheck

import (
	"fmt"
	"testing"
)

// repeatedFormDocs is `/pending 659`'s population: one tagged page that draws ONE form XObject twice, in two different
// marked-content or text-state contexts, in both orders, beside the same form drawn once and twice in one context.
func repeatedFormDocs() ([]string, [][]byte) {
	st := func(body string) string { return fmt.Sprintf("/Length %d >>\nstream\n%s\nendstream", len(body), body) }
	doc := func(content, form string) []byte {
		return buildPDF(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /XObject << /X0 20 0 R >> /Font << /F1 30 0 R >> >> /Contents 4 0 R >>",
			4:  "<< " + st(content),
			7:  "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
			9:  "<< /Nums [0 [8 0 R]] >>",
			20: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 30 0 R >> >> " + st(form),
			30: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		})
	}
	tagged := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC "
	art := "/Artifact BMC /X0 Do EMC "
	bare := "/X0 Do "
	text := "BT /F1 12 Tf 72 600 Td (form) Tj ET"
	span := "/Span << /Lang (en_US) >> BDC 0 0 5 5 re f EMC"
	inP := "/P << /MCID 0 >> BDC /X0 Do EMC "
	artForm := "/Artifact BMC 0 0 5 5 re f EMC"
	extraNames := []string{"artifact-in-form: bare THEN inside P", "artifact-in-form: inside P THEN bare", "form text: invisible ctx THEN visible ctx", "form text: visible ctx THEN invisible ctx"}
	extraDocs := [][]byte{
		doc(art+inP, artForm), doc(inP+art, artForm),
		doc(tagged+"/Artifact BMC BT 3 Tr ET /X0 Do EMC /Artifact BMC BT 0 Tr ET /X0 Do EMC ", "BT /F1 12 Tf 72 600 Td (form) Tj ET"),
		doc(tagged+"/Artifact BMC BT 0 Tr ET /X0 Do EMC /Artifact BMC BT 3 Tr ET /X0 Do EMC ", "BT /F1 12 Tf 72 600 Td (form) Tj ET"),
	}
	names := []string{"text form: artifact THEN bare", "text form: bare THEN artifact", "text form: artifact twice", "text form: bare twice", "text form: artifact once", "text form: bare once",
		"badlang form: artifact THEN bare", "badlang form: bare THEN artifact"}
	docs := [][]byte{
		doc(tagged+art+bare, text), doc(tagged+bare+art, text), doc(tagged+art+art, text), doc(tagged+bare+bare, text), doc(tagged+art, text), doc(tagged+bare, text),
		doc(tagged+art+bare, span), doc(tagged+bare+art, span),
	}
	return append(names, extraNames...), append(docs, extraDocs...)
}

// TestAFormDrawnTwiceIsJudgedWhereItIsDrawnFirst — `/pending 659`.
//
// veraPDF's validator visits an object that has an id once, and a form's content stream has one. So a form drawn
// twice is judged in the context of its FIRST drawing: untagged text in a form drawn inside `/Artifact` and then
// again bare PASSES 7.1 t3, and bare first FAILS. nib walked the form at every `Do` and so failed the first — a
// false fail. Measured on 1.30.2; every document is compared with veraPDF on every clause whenever it is present.
func TestAFormDrawnTwiceIsJudgedWhereItIsDrawnFirst(t *testing.T) {
	names, docs := repeatedFormDocs()
	want := map[string]Verdict{
		"text form: artifact THEN bare":    Pass, // the row nib failed
		"text form: bare THEN artifact":    Fail, // …and its mirror, or "a repeated form is never graded" would pass
		"text form: artifact twice":        Pass,
		"text form: bare twice":            Fail,
		"text form: artifact once":         Pass,
		"text form: bare once":             Fail,
		"badlang form: artifact THEN bare": Pass,
		"badlang form: bare THEN artifact": Fail,
	}
	seen := 0
	for i, pdf := range docs {
		w, ok := want[names[i]]
		if !ok {
			continue
		}
		seen++
		if got := verdictOf(t, pdf, "7.1 t3"); got.Verdict != w {
			t.Errorf("%s: 7.1 t3 reports %v (%s), want %v", names[i], got.Verdict, got.Why, w)
		}
	}
	if seen != len(want) {
		t.Fatalf("setup: %d of the %d named documents exist", seen, len(want))
	}
	vera := veraAsk(t, docs)
	if vera == nil {
		return
	}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	for i, pdf := range docs {
		for _, clause := range Clauses() {
			if got := verdictOf(t, pdf, clause); words[got.Verdict] != vera[i][clause] {
				t.Errorf("%s: %s — nib reports %v (%s), veraPDF %q", names[i], clause, got.Verdict, got.Why, vera[i][clause])
			}
		}
	}
}
