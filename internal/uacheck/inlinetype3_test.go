package uacheck

import "testing"

// TestAType3FontWrittenInlineAnswersAsItsIndirectTwinDoes — `/pending 857`, ADR-116.
//
// pdfcpu's validator removes a Type 3 font dictionary written directly in a `/Font` dictionary. The checker refused
// every clause that reads page content over such a document (23 of them) rather than answer about a font it could
// not see. It now carries the font across the validator, and the rules read it.
//
// **The invariant needs no oracle:** the same font written as an object of its own is a document nib always read,
// so each inline document must answer every clause exactly as its indirect twin does. veraPDF is asked as well
// whenever it is present — measured on 1.30.2, it agrees with nib on every clause of all six.
func TestAType3FontWrittenInlineAnswersAsItsIndirectTwinDoes(t *testing.T) {
	t3 := t3Font("97 /a 98 /b", "/FirstChar 97 /LastChar 98 /Widths [500 500]", "/CharProcs << /a 20 0 R /b 20 0 R >>", "")
	helv := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	form := func(font string) string {
		return spStream("/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F1 "+font+" >> >>", "BT /F1 12 Tf 10 10 Td (ab) Tj ET")
	}
	pairs := []struct {
		name             string
		inline, indirect []byte
	}{
		{"drawn on the page",
			glyphPage("BT /F1 12 Tf 10 10 Td (ab) Tj ET", "/F1 "+t3, "", helv, map[int]string{20: proc("")}),
			glyphPage("BT /F1 12 Tf 10 10 Td (ab) Tj ET", "/F1 21 0 R", "", helv, map[int]string{20: proc(""), 21: t3})},
		{"drawn beside another font",
			glyphPage("BT /F0 12 Tf 10 10 Td (x) Tj /F1 12 Tf (ab) Tj ET", "/F1 "+t3, "", helv, map[int]string{20: proc("")}),
			glyphPage("BT /F0 12 Tf 10 10 Td (x) Tj /F1 12 Tf (ab) Tj ET", "/F1 21 0 R", "", helv, map[int]string{20: proc(""), 21: t3})},
		{"in a form's own resources",
			glyphPage("/X0 Do", "", "/XObject << /X0 22 0 R >>", helv, map[int]string{20: proc(""), 22: form(t3)}),
			glyphPage("/X0 Do", "", "/XObject << /X0 22 0 R >>", helv, map[int]string{20: proc(""), 21: t3, 22: form("21 0 R")})},
	}
	var docs [][]byte
	for _, p := range pairs {
		docs = append(docs, p.inline, p.indirect)
	}
	vera := veraAsk(t, docs)
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	for i, p := range pairs {
		settled := 0
		for _, clause := range Clauses() {
			in, out := verdictOf(t, p.inline, clause), verdictOf(t, p.indirect, clause)
			if in.Verdict != out.Verdict {
				t.Errorf("%s: %s reports %v (%s) with the font inline and %v with it as its own object", p.name, clause, in.Verdict, in.Why, out.Verdict)
			}
			if in.Verdict == CannotCheck {
				t.Errorf("%s: %s refuses over the inline font: %s", p.name, clause, in.Why)
			}
			if in.Verdict == Pass || in.Verdict == Fail {
				settled++
			}
			if vera != nil && vera[2*i] != nil && words[in.Verdict] != vera[2*i][clause] {
				t.Errorf("%s: %s — nib reports %v (%s), veraPDF %q", p.name, clause, in.Verdict, in.Why, vera[2*i][clause])
			}
		}
		// Stimulus: the twin is a document the rules actually judge, or "the same as the twin" says nothing.
		if settled < 20 {
			t.Errorf("%s: only %d clauses pass or fail, so the comparison is over almost nothing", p.name, settled)
		}
		// The font really is drawn from: the glyph clauses have a subject.
		if got := verdictOf(t, p.inline, "7.21.4.1 t1"); got.Verdict == NotApplicable {
			t.Errorf("%s: 7.21.4.1 t1 has no subject — the inline font's text was never reached", p.name)
		}
	}
}
