package uacheck

import (
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The font rules — `PLAN-accessibility.md` P07.S04.

// TestTheFontRulesAgreeWithWhatVeraPDFSaid — the slice's first acceptance clause.
//
// Every expected verdict is what veraPDF reported about that exact document shape at this slice's
// step zero. `/pending 479` is the text-only form's 7.21.4.1 failure: when 479 is fixed veraPDF's
// verdict changes and so does this row, which is what makes it a standing reader rather than a
// one-off measurement.
func TestTheFontRulesAgreeWithWhatVeraPDFSaid(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	textForm, err := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	checkForm, err := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 660, 112, 672}, Kind: "check", Name: "agree", Label: "I agree"}})
	if err != nil {
		t.Fatal(err)
	}
	md, err := pdfops.ConvertDocToPDF([]byte("# Heading\n\nSome prose.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		doc, clause string
		pdf         []byte
		want        Verdict
	}{
		{"a Courier page", "7.21.4.1 t1", plain, Fail},
		{"a Courier page", "7.21.7 t1", plain, Pass},
		{"a Courier page", "7.21.4.2 t2", plain, NotApplicable},
		{"a text-only form (/pending 479)", "7.21.4.1 t1", textForm, Fail},
		{"a text-only form", "7.21.7 t1", textForm, Pass},
		{"a checkbox-only form", "7.21.7 t1", checkForm, Fail},
		{"converted Markdown", "7.21.4.1 t1", md, Pass},
		{"converted Markdown", "7.21.7 t1", md, Pass},
		// Pass, not NotApplicable: the subject is the embedded CID font, and one without a /CIDSet
		// satisfies the clause. The S05 guard found nib and veraPDF disagreeing on exactly this.
		{"converted Markdown", "7.21.4.2 t2", md, Pass},
	} {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != c.want {
			t.Errorf("%s: %s reports %v (%s at %s), want %v", c.doc, c.clause, got.Verdict, got.Why, got.Where, c.want)
		}
	}
}

// TestANonEmbeddedFontIsReportedByFaceAndPage — the slice's second acceptance clause, on the case
// veraPDF located inside a widget's APPEARANCE stream rather than in page content.
func TestANonEmbeddedFontIsReportedByFaceAndPage(t *testing.T) {
	plain, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	form, err := pdfops.AuthorForm(plain, []pdfops.FormField{{Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	// The host page's Courier fails first in page order; strip it so the widget's Helvetica is the
	// one reported and the appearance walk is what finds it.
	got := verdictOf(t, pageContentEmptied(t, form), "7.21.4.1 t1")
	if got.Verdict != Fail {
		t.Fatalf("a form whose only visible text is in its widget appearance reports %v (%s)", got.Verdict, got.Why)
	}
	for _, want := range []string{"Helvetica", "page 1", "appearance"} {
		if !strings.Contains(got.Why+" "+got.Where, want) {
			t.Errorf("the failure %q at %q does not name %q — veraPDF located this font at "+
				"`annots[0]/appearance[0]/contentStream[0]/operators[10]/font[0](Helvetica)`", got.Why, got.Where, want)
		}
	}
}

// TestRenderingIsDecidedByTheRenderModeInForce — measured against veraPDF, one row each.
func TestRenderingIsDecidedByTheRenderModeInForce(t *testing.T) {
	for _, c := range []struct {
		name, content string
		want          Verdict
	}{
		{"visible text", "BT /F1 12 Tf 72 700 Td (abc) Tj ET", Fail},
		// Not used for rendering, so the font PASSES (`renderingMode == 3`) — a subject, not an absence: veraPDF
		// reports one passed check (P07.S02; NotApplicable here had never been scored strictly).
		{"invisible text (3 Tr) is not used for rendering", "BT /F1 12 Tf 3 Tr 72 700 Td (abc) Tj ET", Pass},
		{"clipping text (7 Tr) IS used for rendering", "BT /F1 12 Tf 7 Tr 72 700 Td (abc) Tj ET", Fail},
		{"Q restores the render mode", "q BT /F1 12 Tf 3 Tr ET Q BT /F1 12 Tf 72 700 Td (abc) Tj ET", Fail},
		{"ET does NOT reset the render mode", "BT /F1 12 Tf 3 Tr ET BT 72 700 Td (abc) Tj ET", Pass},
		{"Tf with nothing shown", "BT /F1 12 Tf ET", NotApplicable},
	} {
		got := verdictOf(t, pageWithContent(c.content), "7.21.4.1 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.21.4.1 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
	}
}

// TestUnicodeMappingHasNoInvisibleExemption — the asymmetry with 7.21.4.1, measured: ZapfDingbats in
// `3 Tr` still fails 7.21.7.
func TestUnicodeMappingHasNoInvisibleExemption(t *testing.T) {
	for _, c := range []struct {
		name, font, content string
		want                Verdict
	}{
		{"Helvetica, no /Encoding", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", "BT /F1 12 Tf 72 700 Td (abc) Tj ET", Pass},
		{"ZapfDingbats visible", "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>", "BT /F1 12 Tf 72 700 Td (4) Tj ET", Fail},
		{"ZapfDingbats invisible", "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>", "BT /F1 12 Tf 3 Tr 72 700 Td (4) Tj ET", Fail},
		{"Symbol visible", "<< /Type /Font /Subtype /Type1 /BaseFont /Symbol >>", "BT /F1 12 Tf 72 700 Td (a) Tj ET", Fail},
		// No encoding and no program: nothing names the glyph, so its Unicode is null (measured at P07.S02).
		{"a TrueType face with no encoding, no program and no ToUnicode", "<< /Type /Font /Subtype /TrueType /BaseFont /SomeFace >>", "BT /F1 12 Tf 72 700 Td (a) Tj ET", Fail},
	} {
		got := verdictOf(t, pageWithFont(c.font, c.content), "7.21.7 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.21.7 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
	}
}

// TestACIDSetMustCoverEveryGlyphSlotNotEveryUsedGlyph — the population, measured.
func TestACIDSetMustCoverEveryGlyphSlotNotEveryUsedGlyph(t *testing.T) {
	md, err := pdfops.ConvertDocToPDF([]byte("# Heading\n\nSome prose.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if got := verdictOf(t, md, "7.21.4.2 t2"); got.Verdict != Pass {
		t.Fatalf("control: nib's own Markdown embeds CID fonts with no /CIDSet and reports %v, want Pass", got.Verdict)
	}
	if got := verdictOf(t, pageWithContent("BT /F1 12 Tf 72 700 Td (abc) Tj ET"), "7.21.4.2 t2"); got.Verdict != NotApplicable {
		t.Errorf("a page with no embedded CID font reports %v for 7.21.4.2 t2, want NotApplicable", got.Verdict)
	}
	exact := withCIDSet(t, md, cidExact)
	if got := verdictOf(t, exact, "7.21.4.2 t2"); got.Verdict != Pass {
		t.Errorf("a /CIDSet of exactly every maxp glyph slot reports %v (%s), want Pass — veraPDF passed it", got.Verdict, got.Why)
	}
	partial := withCIDSet(t, md, cidPartial)
	if got := verdictOf(t, partial, "7.21.4.2 t2"); got.Verdict != Fail {
		t.Errorf("a /CIDSet covering only the first glyphs reports %v (%s), want Fail — veraPDF failed a set of the used glyphs", got.Verdict, got.Why)
	}
	// **Over-claiming fails too, measured** — and nib passed it until the law 5 check at this slice's
	// close found veraPDF failing it.
	padded := withCIDSet(t, md, cidPadded)
	got := verdictOf(t, padded, "7.21.4.2 t2")
	if got.Verdict != Fail {
		t.Errorf("a /CIDSet claiming CIDs past numGlyphs reports %v (%s), want Fail — veraPDF failed it", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "claims CID") {
		t.Errorf("the over-claim is reported as %q, which does not say the set claims glyphs the program lacks", got.Why)
	}
}

// fontDoorClauses are the clauses `fontDoorFixtures` are measured over, in the order of their verdict strings.
var fontDoorClauses = []string{"7.21.4.1 t1", "7.21.5 t1", "7.21.4.1 t2", "7.21.8 t1", "7.21.7 t1", "7.21.4.2 t2", "7.21.3.2 t1"}

// fontDoorFixture is one shape the P07 phase close measured: `vera` is veraPDF 1.30.2's verdict on each of
// fontDoorClauses (P, F, or - for no subject), `nib` is nib's — the same letter, or C where nib refuses, and then its
// reason holds `refused`. Each fixture carries its own verdicts, so no table can drift out of step with another.
type fontDoorFixture struct {
	name, vera, nib, refused string
	pdf                      []byte
}

// cid2Doc is a Type 0 font over a CIDFontType2 descendant named `base` carrying `cidExtra`, its program `prog` under
// `key` (a /FontFile3 of `ff3Subtype`), and a /CIDSet object when `set` is not empty, drawing CIDs 1 and 2.
func cid2Doc(base, cidExtra string, prog []byte, key, ff3Subtype, set string, extra map[int]string) []byte {
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + base + " /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) " +
			"/Supplement 0 >> /FontDescriptor 12 0 R " + cidExtra + " >>",
	}
	pd := ""
	if key == "FontFile3" {
		pd = "/Subtype /" + ff3Subtype
	}
	for k, v := range ttObjects("/Flags 4", key, prog, pd) {
		objs[k] = v
	}
	if set != "" {
		objs[12] = strings.TrimSuffix(objs[12], " >>") + " /CIDSet 30 0 R >>"
		objs[30] = set
	}
	for k, v := range extra {
		objs[k] = v
	}
	return glyphPage("BT /F0 12 Tf 10 10 Td <00010002> Tj ET", "", "", "<< /Type /Font /Subtype /Type0 /BaseFont /"+base+
		" /Encoding /Identity-H /DescendantFonts [11 0 R] >>", objs)
}

// ttCounts is a TrueType program whose hhea/hmtx pair declares each of `hmetrics` in turn and whose maxp tables declare
// each of `maxps` in turn — a later duplicate replacing an earlier one, as veraPDF's parser reads a directory.
func ttCounts(hmetrics, maxps []int, cmap []byte) []byte {
	ts := []ttTable{{"cmap", cmap}, {"head", ttHead()}}
	for _, n := range hmetrics {
		ts = append(ts, ttTable{"hhea", ttHhea(n)}, ttTable{"hmtx", ttHmtx(n)})
	}
	for _, n := range maxps {
		ts = append(ts, ttTable{"maxp", ttMaxp(n)})
	}
	return sfnt(append(ts, ttTable{"post", ttPost3()})...)
}

func cidRange(a, b int) []int {
	var out []int
	for i := a; i < b; i++ {
		out = append(out, i)
	}
	return out
}

func fontDoorFixtures() []fontDoorFixture {
	sub := "ABCDEF+Probe"
	std := cffSpec{names: []string{"A", "B", "C"}, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build()
	cidStd := cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build()
	w500 := "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding"
	junk := []byte("this is not a font program at all, just some junk bytes for veraPDF")
	cm := ttCmap(ttSub{3, 1, cmapFmt4(0x20, 95)})
	badCmap := beBytes(uint16(0), uint16(1), uint16(3), uint16(1), uint32(0x00FFFFF0)) // one subtable, past the end
	p100 := ttCounts([]int{100}, []int{100}, cm)
	t1 := func(prog []byte, widths string) []byte {
		return t1cDoc(sub, widths, "/CharSet (/A/B/C)", "(ABC) Tj", prog, "OpenType")
	}
	c0 := func(prog []byte) []byte {
		return cid0Doc("CIDFontType0", sub, "/W [1 [500 500 500]]", "", hexCodes(1, 2, 3), prog, "OpenType", "", nil)
	}
	all := func(n int) string { return cidSetBytes(cidRange(0, n)...) }
	id := "/CIDToGIDMap /Identity"
	mapped := map[int]string{31: spStream("", string([]byte{0, 0, 0, 1, 0, 2, 0, 3}))}
	pastProgram := map[int]string{31: spStream("", string([]byte{0, 0, 0, 1, 0, 2, 0, 200}))} // CID 3 → glyph 200 of 100
	stray := make([]byte, cidSetMaxBytes+10)
	for i := 0; i < 13; i++ {
		stray[i] = 0xFF
	}
	stray[cidSetMaxBytes+5] = 0xFF
	const ot = "OpenType"
	return []fontDoorFixture{
		// R2-1: a /FontFile3 /OpenType program is opened for its "CFF " table; with none there is no program.
		// RR1-5: the metric and glyph clauses ask the same door, so with no "CFF " table they answer as veraPDF does.
		{name: "T1 OpenType: junk", vera: "FPPPP--", nib: "FPPPP--", pdf: t1(junk, w500)},
		{name: "T1 OpenType: two bytes", vera: "FPPPP--", nib: "FPPPP--", pdf: t1([]byte{1, 2}, w500)},
		{name: "T1 OpenType: a TrueType program", vera: "FPPPP--", nib: "FPPPP--", pdf: t1(p100, w500)},
		{name: "T1 OpenType: junk, widths off", vera: "FPPPP--", nib: "FPPPP--",
			pdf: t1(junk, "/FirstChar 65 /LastChar 67 /Widths [700 700 700] /Encoding /WinAnsiEncoding")},
		{name: "T1 OpenType: a CFF table", vera: "PPPPP--", nib: "CCCPP--", refused: ot, pdf: t1(sfnt(ttTable{"CFF ", std}), w500)},
		{name: "T1 OpenType: a CFF table, widths off", vera: "PFPPP--", nib: "CCCPP--", refused: ot,
			pdf: t1(sfnt(ttTable{"CFF ", std}), "/FirstChar 65 /LastChar 67 /Widths [700 700 700] /Encoding /WinAnsiEncoding")},
		{name: "CID0 OpenType: junk", vera: "FPPPFPP", nib: "FPPPFPP", pdf: c0(junk)},
		{name: "CID0 OpenType: a TrueType program", vera: "FPPPFPP", nib: "FPPPFPP", pdf: c0(p100)},
		{name: "CID0 OpenType: a CFF table", vera: "PPPPFPP", nib: "CCCCFPP", refused: ot, pdf: c0(sfnt(ttTable{"CFF ", cidStd}))},
		// RR1-2: a name that is not subset-named passes 7.21.4.2 t2 whatever the program, so the program nib cannot
		// read is not asked; subset-named, the set is judged against it, and nib refuses.
		{name: "CID0 OpenType: a CFF table, a short CIDSet, not subset-named", vera: "PPPPFPP", nib: "CCCCFPP", refused: ot,
			pdf: cid0Doc("CIDFontType0", "Probe", "/W [1 [500 500 500]]", "/CIDSet 22 0 R", hexCodes(1, 2, 3),
				sfnt(ttTable{"CFF ", cidStd}), "OpenType", "", map[int]string{22: cidSetBytes(1)})},
		{name: "CID0 OpenType: a CFF table, a short CIDSet, subset-named", vera: "PPPPFFP", nib: "CCCCFCP", refused: ot,
			pdf: cid0Doc("CIDFontType0", sub, "/W [1 [500 500 500]]", "/CIDSet 22 0 R", hexCodes(1, 2, 3),
				sfnt(ttTable{"CFF ", cidStd}), "OpenType", "", map[int]string{22: cidSetBytes(1)})},
		// R1-2 and /pending 684: the CIDFontType2 population is the hhea count listed and the LAST maxp held, CID 0 aside.
		{name: "CIDSet: every slot", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, p100, "FontFile2", "", all(100), nil)},
		{name: "CIDSet: CID 0 unset", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, p100, "FontFile2", "", cidSetBytes(cidRange(1, 100)...), nil)},
		{name: "CIDSet: 100 over maxp 100 then 50", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{100, 50}, cm), "FontFile2", "", all(100), nil)},
		{name: "CIDSet: 50 over maxp 100 then 50", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{100, 50}, cm), "FontFile2", "", all(50), nil)},
		{name: "CIDSet: 50 over maxp 50 then 100", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{50, 100}, cm), "FontFile2", "", all(50), nil)},
		{name: "CIDSet: 50 over hhea 50, maxp 100", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, ttCounts([]int{50}, []int{100}, cm), "FontFile2", "", all(50), nil)},
		{name: "CIDSet: 100 over hhea 50, maxp 100", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, ttCounts([]int{50}, []int{100}, cm), "FontFile2", "", all(100), nil)},
		{name: "CIDSet: 50 over hhea 100, maxp 50", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{50}, cm), "FontFile2", "", all(50), nil)},
		{name: "CIDSet: 100 over hhea 100, maxp 50", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{50}, cm), "FontFile2", "", all(100), nil)},
		{name: "CIDSet: short, not subset-named", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc("Probe", id, p100, "FontFile2", "", all(10), nil)},
		{name: "CIDSet: short, subset-named", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, p100, "FontFile2", "", all(10), nil)},
		{name: "CIDSet: short, a program with no hhea", vera: "FPPPFPP", nib: "FPPPFPP", pdf: cid2Doc(sub, id, ttCounts(nil, []int{100}, cm), "FontFile2", "", all(10), nil)},
		{name: "CIDSet: short, a cmap past the end", vera: "FPPPFPP", nib: "FPPPFPP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{100}, badCmap), "FontFile2", "", all(10), nil)},
		{name: "CIDSet: a stray bit past 16,384 bytes", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, ttCounts([]int{100}, []int{104}, cm), "FontFile2", "", spStream("", string(stray)), nil)},
		{name: "CIDSet: a four-CID map, 1 to 3", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2, 3), mapped)},
		{name: "CIDSet: a four-CID map, 1 to 2", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2), mapped)},
		{name: "CIDSet: a four-CID map, 1 to 4", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2, 3, 4), mapped)},
		// A mapped CID whose glyph the program lacks is neither listed nor holdable: unset passes, set fails.
		{name: "CIDSet: a map past the program, 1 to 2", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2), pastProgram)},
		{name: "CIDSet: a map past the program, 1 to 3", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, "/CIDToGIDMap 31 0 R", p100, "FontFile2", "", cidSetBytes(1, 2, 3), pastProgram)},
		{name: "CIDSet: FontFile3 OpenType, every slot", vera: "PFPPFPP", nib: "PFPPFPP", pdf: cid2Doc(sub, id, p100, "FontFile3", ot, all(100), nil)},
		{name: "CIDSet: FontFile3 OpenType, short", vera: "PFPPFFP", nib: "PFPPFFP", pdf: cid2Doc(sub, id, p100, "FontFile3", ot, all(10), nil)},
		// /pending 680: 7.21.3.2 t1 asks the same door.
		{name: "no map: a cmap past the end", vera: "FPPPFPP", nib: "FPPPFPP", pdf: cid2Doc(sub, "", ttCounts([]int{100}, []int{100}, badCmap), "FontFile2", "", "", nil)},
		{name: "no map: a program veraPDF parses", vera: "PFPPFPF", nib: "PFPPFPF", pdf: cid2Doc(sub, "", p100, "FontFile2", "", "", nil)},
		{name: "no map: a /FontFile3 of another subtype", vera: "FPPPFPP", nib: "FPPPFPP", pdf: cid2Doc(sub, "", p100, "FontFile3", "Foo", "", nil)},
		{name: "no map: FontFile3 OpenType, a cmap past the end", vera: "FPPPFPP", nib: "FPPPFPP", pdf: cid2Doc(sub, "", ttCounts([]int{100}, []int{100}, badCmap), "FontFile3", ot, "", nil)},
	}
}

// TestTheFontDoorsAgreeWithVeraPDF — every shape above against veraPDF's verdicts, measured at the P07 phase close
// before the doors changed. A letter outside the declared alphabet fails the test rather than reading as a verdict.
func TestTheFontDoorsAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail, '-': NotApplicable, 'C': CannotCheck}
	for _, f := range fontDoorFixtures() {
		if len(f.vera) != len(fontDoorClauses) || len(f.nib) != len(fontDoorClauses) {
			t.Fatalf("%s: %d veraPDF and %d nib letters for %d clauses", f.name, len(f.vera), len(f.nib), len(fontDoorClauses))
		}
		for j, clause := range fontDoorClauses {
			v, n := f.vera[j], f.nib[j]
			if _, ok := want[v]; !ok || v == 'C' {
				t.Fatalf("%s: veraPDF letter %q is not one of P, F, -", f.name, v)
			}
			if _, ok := want[n]; !ok {
				t.Fatalf("%s: nib letter %q is not one of P, F, -, C", f.name, n)
			}
			if n != 'C' && n != v {
				t.Fatalf("%s: %s declares nib %c where veraPDF says %c — only a refusal may differ", f.name, clause, n, v)
			}
			got := verdictOf(t, f.pdf, clause)
			if got.Verdict != want[n] {
				t.Errorf("%s: %s reports %v (%s), want %c (veraPDF %c)", f.name, clause, got.Verdict, got.Why, n, v)
			} else if n == 'C' && !strings.Contains(got.Why, f.refused) {
				t.Errorf("%s: %s refuses (%s), not naming %q", f.name, clause, got.Why, f.refused)
			}
		}
	}
}

// TestTheOpenTypeSearchReadsAsVeraPDFDoes — `getCFFTable`'s search, record by record: a count read past the end is
// 0xFFFF, and a record the program cuts short cannot carry the tag.
func TestTheOpenTypeSearchReadsAsVeraPDFDoes(t *testing.T) {
	withCFF := sfnt(ttTable{"head", ttHead()}, ttTable{"CFF ", []byte{1, 0, 4, 1}})
	for _, c := range []struct {
		name string
		prog []byte
		want bool
	}{
		{"a CFF table second", withCFF, true},
		{"the count one short of it", append(append([]byte{}, withCFF[:5]...), append([]byte{1}, withCFF[6:]...)...), false},
		{"no CFF table", ttProgram(), false},
		{"the tag cut at the end", withCFF[:12+16+3], false},
		{"the tag and nothing after it", withCFF[:12+16+4], true},
		{"a count past the end", []byte{0, 1, 0, 0}, false},
	} {
		if got := openTypeHasCFFTable(c.prog); got != c.want {
			t.Errorf("%s: found a CFF table %v, want %v", c.name, got, c.want)
		}
	}
}

// TestAnUnreadSubtypeGetsOneAnswer — R2-9: a font of a subtype veraPDF builds no font for had three answers from three
// clauses. pdfcpu refuses such a document (measured), so the doors are asked directly.
func TestAnUnreadSubtypeGetsOneAnswer(t *testing.T) {
	d, err := open(ttDoc("/Flags 32", "", "(A) Tj", ttProgram(sub31)))
	if err != nil {
		t.Fatal(err)
	}
	for _, font := range []types.Dict{{"Subtype": types.Name("Foo")}, {}} {
		g := glyph{font: &glyphFont{dict: font}, code: 65}
		if st, why := d.fontEmbedded(font); st != ttUnknown || !strings.Contains(why, "not one veraPDF builds") {
			t.Errorf("%v: 7.21.4.1 t1's door answers %v (%s), want a refusal", font, st, why)
		}
		if m := d.metricsOf(g); m.known || !strings.Contains(m.why, "not one veraPDF builds") {
			t.Errorf("%v: the metric door answers known=%v (%s), want a refusal", font, m.known, m.why)
		}
		if _, known, why := d.glyphName(g); known || !strings.Contains(why, "not one veraPDF builds") {
			t.Errorf("%v: 7.21.8 t1's door answers known=%v (%s), want a refusal", font, known, why)
		}
	}
}

// pageWithFont is pageWithContent with the /F1 font dictionary supplied.
func pageWithFont(fontDict, content string) []byte {
	return buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length " + fmtInt(len(content)) + " >>\nstream\n" + content + "\nendstream",
		5: fontDict,
	})
}

// TestAnUnresolvedFontRefusesTheCIDSetClauseBesideOneThatResolved — /pending 722. The refusal for a font that does not
// resolve was reached only when NO CID font had, so once one Type 0 font resolved with an exact /CIDSet, a second font
// nib never read — possibly a CID font pdfcpu dropped, whose /CIDSet veraPDF does judge — was ignored and 7.21.4.2 t2
// answered Pass. Every sibling font door refuses instead.
func TestAnUnresolvedFontRefusesTheCIDSetClauseBesideOneThatResolved(t *testing.T) {
	md, err := pdfops.ConvertDocToPDF([]byte("# Heading\n\nSome prose.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	exact := withCIDSet(t, md, cidExact)
	if got := verdictOf(t, exact, "7.21.4.2 t2"); got.Verdict != Pass {
		t.Fatalf("control: an exact /CIDSet on every resolved CID font reports %v (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, withTextRunInFont(t, exact, "F722"), "7.21.4.2 t2")
	if got.Verdict != CannotCheck {
		t.Errorf("beside a resolved CID font with an exact /CIDSet, text in a font that does not resolve reports %v (%s), "+
			"want CannotCheck — the unread font may be a CID font whose /CIDSet fails", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "F722") {
		t.Errorf("the refusal %q does not name the font that did not resolve", got.Why)
	}
	// A definite failure among the fonts that DID resolve still outranks the refusal, as it does in the siblings.
	partial := withTextRunInFont(t, withCIDSet(t, md, cidPartial), "F722")
	if got := verdictOf(t, partial, "7.21.4.2 t2"); got.Verdict != Fail {
		t.Errorf("an under-claiming /CIDSet beside an unresolved font reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// withTextRunInFont appends a content stream to page 1 that draws text in font resource `name`, which the page's
// resources do not hold — at nib's reading, the same thing as a font pdfcpu's validator dropped.
func withTextRunInFont(t *testing.T, pdf []byte, name string) []byte {
	t.Helper()
	return mutate(t, pdf, func(ctx *model.Context) error {
		page, _, _, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf([]byte("BT /" + name + " 12 Tf 72 72 Td (x) Tj ET"))
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		var kids types.Array
		switch c := page["Contents"].(type) {
		case nil:
		case types.Array:
			kids = append(kids, c...)
		default:
			if arr, derr := ctx.DereferenceArray(c); derr == nil && arr != nil {
				kids = append(kids, arr...)
			} else {
				kids = append(kids, c)
			}
		}
		page["Contents"] = append(kids, *ref)
		return nil
	})
}
