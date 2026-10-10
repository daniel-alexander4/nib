package uacheck

import (
	"fmt"
	"testing"
)

// patternFontDocs is `/pending 678`'s population: a tagged page whose non-embedded Helvetica is drawn inside a tiling
// pattern the page selects, or inside the glyph procedure of a Type 3 font the page shows a glyph in — visibly,
// invisibly, not at all, and beside the same font on the page itself.
func patternFontDocs() ([]string, [][]byte) {
	st := func(body string) string { return fmt.Sprintf("/Length %d >>\nstream\n%s\nendstream", len(body), body) }
	helv := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	tiling := "/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 100 100] /XStep 100 /YStep 100 "
	doc := func(res, content string, extra map[int]string) []byte {
		objs := map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << " + res + " >> /Contents 4 0 R >>",
			4:  "<< " + st(content),
			7:  "<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
			8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
			9:  "<< /Nums [0 [8 0 R]] >>",
			30: helv,
		}
		for k, v := range extra {
			objs[k] = v
		}
		return buildPDF(objs)
	}
	drawPattern := "/P << /MCID 0 >> BDC /Pattern cs /P0 scn 0 0 100 100 re f EMC"
	text := func(mode string) string { return "BT /F1 12 Tf " + mode + " 10 10 Td (abc) Tj ET" }
	pat := func(body string) map[int]string {
		return map[int]string{20: "<< " + tiling + "/Resources << /Font << /F1 30 0 R >> >> " + st(body)}
	}
	t3 := func(body string) map[int]string {
		return map[int]string{
			21: "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /a 22 0 R >> " +
				"/Encoding << /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [1000] /Resources << /Font << /F1 30 0 R >> >> >>",
			22: "<< " + st("1000 0 d0 "+body),
		}
	}
	drawGlyph := "/P << /MCID 0 >> BDC BT /F3 12 Tf 72 700 Td (a) Tj ET EMC"
	names := []string{"pattern: visible Helvetica", "pattern: invisible Helvetica", "pattern: no text", "type3 proc: visible Helvetica", "type3 proc: invisible Helvetica", "type3 proc: no text",
		"page text + pattern text", "page invisible, pattern visible"}
	docs := [][]byte{
		doc("/Pattern << /P0 20 0 R >>", drawPattern, pat(text("0 Tr"))),
		doc("/Pattern << /P0 20 0 R >>", drawPattern, pat(text("3 Tr"))),
		doc("/Pattern << /P0 20 0 R >>", drawPattern, pat("0 0 5 5 re f")),
		doc("/Font << /F3 21 0 R >>", drawGlyph, t3(text("0 Tr"))),
		doc("/Font << /F3 21 0 R >>", drawGlyph, t3(text("3 Tr"))),
		doc("/Font << /F3 21 0 R >>", drawGlyph, t3("0 0 5 5 re f")),
		doc("/Pattern << /P0 20 0 R >> /Font << /F1 30 0 R >>", "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET /Pattern cs /P0 scn 0 0 100 100 re f EMC", pat(text("0 Tr"))),
		doc("/Pattern << /P0 20 0 R >> /Font << /F1 30 0 R >>", "/P << /MCID 0 >> BDC BT /F1 12 Tf 3 Tr 72 700 Td (x) Tj ET /Pattern cs /P0 scn 0 0 100 100 re f EMC", pat(text("0 Tr"))),
	}
	return names, docs
}

// TestAFontUsedOnlyInAPatternOrAGlyphProcedureIsAFontTheDocumentUses — `/pending 678`.
//
// The content walk reads a tiling pattern and a Type 3 glyph procedure for `/Lang` and glyphs and records no content
// event there — rightly, nothing they draw is a content item. But the FONT rules took their population from those
// events, so a font used only in such a stream was not a font at all: a non-embedded Helvetica drawn in a glyph
// procedure PASSED 7.21.4.1 t1 (the Type 3 font beside it was the only subject) where veraPDF fails, and drawn in a
// pattern it had no subject where veraPDF fails. Measured on 1.30.2; with veraPDF present every document is compared
// with it on every clause nib checks.
func TestAFontUsedOnlyInAPatternOrAGlyphProcedureIsAFontTheDocumentUses(t *testing.T) {
	names, docs := patternFontDocs()
	want := map[string]Verdict{
		"pattern: visible Helvetica":      Fail,
		"pattern: invisible Helvetica":    Pass,
		"type3 proc: visible Helvetica":   Fail,
		"type3 proc: invisible Helvetica": Pass,
		"page text + pattern text":        Fail,
		"page invisible, pattern visible": Pass, // the first use decides, and the page's comes first (`firstUseOrder`)
	}
	for i, pdf := range docs {
		w, ok := want[names[i]]
		if !ok {
			continue
		}
		if got := verdictOf(t, pdf, "7.21.4.1 t1"); got.Verdict != w {
			t.Errorf("%s: 7.21.4.1 t1 reports %v (%s), want %v", names[i], got.Verdict, got.Why, w)
		}
	}
	// What the walk must NOT do with those streams: their text is no content item, so 7.1 t3 answers as it does for
	// the same pattern with no text in it.
	if with, without := verdictOf(t, docs[0], "7.1 t3"), verdictOf(t, docs[2], "7.1 t3"); with.Verdict != without.Verdict {
		t.Errorf("text inside a pattern moved 7.1 t3 from %v to %v — a pattern's content is not page content", without.Verdict, with.Verdict)
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
