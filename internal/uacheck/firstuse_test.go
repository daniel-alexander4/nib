package uacheck

import "testing"

// TestAFontIsJudgedByItsFirstUseInVeraPDFsOrder — `/pending 679`.
//
// veraPDF builds one font object per (font, render mode) and evaluates 7.21.4.1 t1 on ONE of them per font — the
// first it reaches. So a non-embedded font drawn invisibly first and visibly later PASSES there, and nib, which
// judged a font by ANY visible use, failed it: a live false fail on every document that puts an OCR-style hidden
// run ahead of visible text in the same font.
//
// Every row was run on veraPDF 1.30.2 before the rule was changed, and is asked again here whenever veraPDF is
// present. The rows are in pairs — the same two uses in both orders — because one order alone cannot tell "the first
// use decides" from "any invisible use excuses the font". What they establish is the ORDER (`firstUseOrder`): a
// stream in content order, a `/Contents` array in array order, pages in page order, a form where its `Do` is, a
// page before its own annotations, and a page's annotations before the next page.
func TestAFontIsJudgedByItsFirstUseInVeraPDFsOrder(t *testing.T) {
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	st := func(c string) string { return "<< /Length " + fmtInt(len(c)) + " >>\nstream\n" + c + "\nendstream" }
	inv := "BT /F1 12 Tf 3 Tr 72 700 Td (abc) Tj ET"
	vis := "BT /F1 12 Tf 0 Tr 72 600 Td (abc) Tj ET"
	res := "/Resources << /Font << /F1 9 0 R >> >>"
	resX := "/Resources << /Font << /F1 9 0 R >> /XObject << /X0 8 0 R >> >>"
	onePage := func(c string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 4 0 R >>", 4: st(c), 9: font})
	}
	twoPages := func(a, b string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 4 0 R >>", 4: st(a),
			5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 6 0 R >>", 6: st(b), 9: font})
	}
	twoStreams := func(a, b string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents [4 0 R 6 0 R] >>", 4: st(a), 6: st(b), 9: font})
	}
	withForm := func(page, form string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + resX + " /Contents 4 0 R >>", 4: st(page),
			8: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Length " + fmtInt(len(form)) + " >>\nstream\n" + form + "\nendstream", 9: font})
	}
	withAnnot := func(page, ap string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 4 0 R /Annots [7 0 R] >>", 4: st(page),
			7: "<< /Type /Annot /Subtype /Stamp /Rect [10 10 300 300] /F 4 /Contents (s) /AP << /N 8 0 R >> >>",
			8: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Length " + fmtInt(len(ap)) + " >>\nstream\n" + ap + "\nendstream", 9: font})
	}
	annotThenPage := func(ap, page2 string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 4 0 R /Annots [7 0 R] >>", 4: st("0 0 1 1 re f"),
			5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " + res + " /Contents 6 0 R >>", 6: st(page2),
			7: "<< /Type /Annot /Subtype /Stamp /Rect [10 10 300 300] /F 4 /Contents (s) /AP << /N 8 0 R >> >>",
			8: "<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Length " + fmtInt(len(ap)) + " >>\nstream\n" + ap + "\nendstream", 9: font})
	}
	cases := []struct {
		name string
		pdf  []byte
		want Verdict
		vera string
	}{
		{"one stream: invisible, then visible", onePage(inv + "\n" + vis), Pass, "passed"},
		{"one stream: visible, then invisible", onePage(vis + "\n" + inv), Fail, "failed"},
		{"two pages: invisible, then visible", twoPages(inv, vis), Pass, "passed"},
		{"two pages: visible, then invisible", twoPages(vis, inv), Fail, "failed"},
		{"a /Contents array: invisible, then visible", twoStreams(inv, vis), Pass, "passed"},
		{"a /Contents array: visible, then invisible", twoStreams(vis, inv), Fail, "failed"},
		{"the page invisible, then a form drawn visible", withForm(inv+"\n/X0 Do", vis), Pass, "passed"},
		{"a form drawn visible, then the page invisible", withForm("/X0 Do\n"+inv, vis), Fail, "failed"},
		{"the page visible, then a form drawn invisible", withForm(vis+"\n/X0 Do", inv), Fail, "failed"},
		{"a form drawn invisible, then the page visible", withForm("/X0 Do\n"+vis, inv), Pass, "passed"},
		{"the page invisible, its annotation visible", withAnnot(inv, vis), Pass, "passed"},
		{"the page visible, its annotation invisible", withAnnot(vis, inv), Fail, "failed"},
		// The pair nib's own walk order gets wrong: it emits every page and then every appearance.
		{"page 1's annotation invisible, page 2 visible", annotThenPage(inv, vis), Pass, "passed"},
		{"page 1's annotation visible, page 2 invisible", annotThenPage(vis, inv), Fail, "failed"},
		{"only invisible", onePage(inv), Pass, "passed"},
		{"only visible", onePage(vis), Fail, "failed"},
	}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, c.pdf)
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		if got := verdictOf(t, c.pdf, "7.21.4.1 t1"); got.Verdict != c.want {
			t.Errorf("%s: 7.21.4.1 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
		if vera != nil && vera[i]["7.21.4.1 t1"] != c.vera {
			t.Errorf("%s: veraPDF now says %q where %q was measured — the row's expectation is stale", c.name, vera[i]["7.21.4.1 t1"], c.vera)
		}
	}
}
