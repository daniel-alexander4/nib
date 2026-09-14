package uacheck

import (
	"strings"
	"testing"

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
		{"converted Markdown", "7.21.4.2 t2", md, NotApplicable},
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
		{"invisible text (3 Tr) is not used for rendering", "BT /F1 12 Tf 3 Tr 72 700 Td (abc) Tj ET", NotApplicable},
		{"clipping text (7 Tr) IS used for rendering", "BT /F1 12 Tf 7 Tr 72 700 Td (abc) Tj ET", Fail},
		{"Q restores the render mode", "q BT /F1 12 Tf 3 Tr ET Q BT /F1 12 Tf 72 700 Td (abc) Tj ET", Fail},
		{"ET does NOT reset the render mode", "BT /F1 12 Tf 3 Tr ET BT 72 700 Td (abc) Tj ET", NotApplicable},
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
		{"an unmeasured TrueType with no ToUnicode", "<< /Type /Font /Subtype /TrueType /BaseFont /SomeFace >>", "BT /F1 12 Tf 72 700 Td (a) Tj ET", CannotCheck},
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
	if got := verdictOf(t, md, "7.21.4.2 t2"); got.Verdict != NotApplicable {
		t.Fatalf("control: nib's own Markdown carries no /CIDSet and reports %v", got.Verdict)
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

// TestTheTrueTypeReaderRefusesWhatItCannotVouchFor.
func TestTheTrueTypeReaderRefusesWhatItCannotVouchFor(t *testing.T) {
	// **Each refusal is checked by its REASON — found by probing.** Accepting `OTTO` left this green
	// because a 24-byte fixture has no maxp table, so the next guard refused it anyway, for a
	// different reason. A reader that then met a real CFF program would parse it as TrueType.
	for _, c := range []struct {
		name, reason string
		prog         []byte
	}{
		{"too short", "too short", []byte{0, 1, 0}},
		{"CFF-flavoured OpenType", "CFF", append([]byte("OTTO"), make([]byte, 20)...)},
		{"a collection", "collection", append([]byte("ttcf"), make([]byte, 20)...)},
		{"no maxp table", "no maxp", append([]byte{0, 1, 0, 0, 0, 0}, make([]byte, 10)...)},
	} {
		_, err := trueTypeGlyphCount(c.prog)
		if err == nil {
			t.Errorf("%s: read a glyph count from a program nib cannot vouch for", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%s: refused for %q, not because it is %s", c.name, err, c.reason)
		}
	}
	// PDF bit order: the high bit of byte 0 is CID 0.
	if ok, _, _ := cidSetExact([]byte{0x80}, 1); !ok {
		t.Error("0x80 does not identify exactly CID 0 — the bit order is reversed")
	}
	if ok, missing, _ := cidSetExact([]byte{0x01}, 1); ok || missing != 0 {
		t.Errorf("0x01 identifies CID 0 (missing=%d) — the bit order is reversed", missing)
	}
	if ok, missing, _ := cidSetExact([]byte{0xFF}, 9); ok || missing != 8 {
		t.Errorf("a one-byte set for nine CIDs reports ok=%v missing=%d, want the ninth missing", ok, missing)
	}
	if ok, _, extra := cidSetExact([]byte{0xFF}, 7); ok || extra != 7 {
		t.Errorf("a full byte for seven CIDs reports ok=%v extra=%d, want CID 7 over-claimed", ok, extra)
	}
	if ok, _, _ := cidSetExact([]byte{0xFE, 0x00}, 7); !ok {
		t.Error("an exact set followed by a zero byte is refused — a zero byte claims nothing")
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
