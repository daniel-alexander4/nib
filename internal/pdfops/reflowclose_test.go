package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// The P06 phase-close review's findings (`code-reviews/v1.168.0-p06-phase-close-2026-09-28.md`), one fixture each: every
// shape here was accepted and rewritten wrong — or refused for no reason a user could act on — before its fix.

// pageWith is a one-page document with the page's resources and any further objects given, numbered from 6.
func pageWith(content, resources, catalogExtra, pageExtra string, fonts map[string]string, extra ...string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + ">>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var fontRes []string
	n := 5
	for name, dict := range fonts {
		objs[n] = dict
		fontRes = append(fontRes, fmt.Sprintf("/%s %d 0 R", name, n))
		n += 10 // fonts at 5, 15, …: clear of the numbered extras
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << " + strings.Join(fontRes, " ") +
		" >> " + resources + " >> /Contents 4 0 R " + pageExtra + ">>"
	for i, o := range extra {
		objs[6+i] = o
	}
	return assembleFixture(objs)
}

var helvetica = map[string]string{"F1": "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"}

// A subset TrueType font under WinAnsi whose /Widths are 0 for every character its paragraph does not draw — how a
// subset producer marks a glyph it left out — with `with` given a real width all the same.
func subsetFont(baseFont, draws, with string) string {
	var w []string
	for c := 32; c <= 126; c++ {
		switch {
		case strings.ContainsRune(draws, rune(c)), strings.ContainsRune(with, rune(c)):
			w = append(w, "500")
		default:
			w = append(w, "0")
		}
	}
	return "<< /Type /Font /Subtype /TrueType /BaseFont /" + baseFont + " /FirstChar 32 /LastChar 126 /Widths [" +
		strings.Join(w, " ") + "] /Encoding /WinAnsiEncoding >>"
}

const twoLines = "BT /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj ET"

func TestThePhaseClosesFindingsHold(t *testing.T) {
	draws := "The quick brown fox jumps over the lazy dog"
	t.Run("a subset font's zero-width code is a missing glyph, not a glyph with no advance", func(t *testing.T) {
		pdf := pageWith(twoLines, "", "", "", map[string]string{"F1": subsetFont("ABCDEF+Calibri", draws, "")})
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox jumps over the lazy Zog"); cause != causeMissingGlyph {
			t.Errorf("fell back on %q, want %s", cause, causeMissingGlyph)
		}
		// The zero width alone, in a font not tagged a subset: the subset rule cannot be what refuses it.
		plain := pageWith(twoLines, "", "", "", map[string]string{"F1": subsetFont("Calibri", draws, "")})
		if _, cause := reflowed(t, plain, 0, "The quick brown fox jumps over the lazy Zog"); cause != causeMissingGlyph {
			t.Errorf("a zero-width Z in a full font: fell back on %q, want %s", cause, causeMissingGlyph)
		}
	})
	t.Run("a subset font's code it never draws is a missing glyph, whatever its width says", func(t *testing.T) {
		pdf := pageWith(twoLines, "", "", "", map[string]string{"F1": subsetFont("ABCDEF+Calibri", draws, "Z")})
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox jumps over the lazy Zog"); cause != causeMissingGlyph {
			t.Errorf("fell back on %q, want %s", cause, causeMissingGlyph)
		}
		// The stimulus: the same font NOT tagged as a subset carries its whole repertoire, and Z is drawn.
		full := pageWith(twoLines, "", "", "", map[string]string{"F1": subsetFont("Calibri", draws, "Z")})
		if _, cause := reflowed(t, full, 0, "The quick brown fox jumps over the lazy Zog"); cause != "" {
			t.Errorf("setup: a full font with a width for Z fell back on %q, so the refusal above proves nothing", cause)
		}
	})
	t.Run("a subset font's code drawn elsewhere on the page is drawable", func(t *testing.T) {
		content := strings.Replace(twoLines, " ET", " T* T* (Zed) Tj ET", 1)
		pdf := pageWith(content, "", "", "", map[string]string{"F1": subsetFont("ABCDEF+Calibri", draws+"Zed", "")})
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox jumps over the lazy Zog"); cause != "" {
			t.Errorf("fell back on %q; Z is drawn on this page in this very font", cause)
		}
	})
	t.Run("replacement text named in the resources' /Properties is refused", func(t *testing.T) {
		content := "/Span /P0 BDC " + twoLines + " EMC"
		pdf := pageWith(content, "/Properties << /P0 << /ActualText (old words) >> >>", "", "", helvetica)
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != causeReplacementText {
			t.Errorf("fell back on %q, want %s", cause, causeReplacementText)
		}
		plain := pageWith(content, "/Properties << /P0 << /Lang (en) >> >>", "", "", helvetica)
		if _, cause := reflowed(t, plain, 0, "The quick brown fox leaps over the lazy dog"); cause != "" {
			t.Errorf("setup: a named property list with no replacement text fell back on %q", cause)
		}
	})
	t.Run("replacement text on the structure element the paragraph belongs to is refused, and listed so", func(t *testing.T) {
		content := "/P <</MCID 0>> BDC " + twoLines + " EMC"
		tree := func(elem string) []byte {
			return pageWith(content, "", "/StructTreeRoot 6 0 R /MarkInfo << /Marked true >> ", "/StructParents 0 ", helvetica,
				"<< /Type /StructTreeRoot /K [7 0 R] /ParentTree << /Nums [0 [7 0 R]] >> >>",
				"<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 0 >>",
				elem)
		}
		pdf := tree("<< /Type /StructElem /S /Span /P 6 0 R /ActualText (old words) /K [7 0 R] >>")
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != causeReplacementText {
			t.Errorf("an ancestor's /ActualText: fell back on %q, want %s", cause, causeReplacementText)
		}
		if paras, err := Paragraphs(pdf, 1); err != nil || len(paras) != 1 || paras[0].Refusal != causeReplacementText {
			t.Errorf("the editor's list does not refuse it up front: %+v %v", paras, err)
		}
		plain := tree("<< /Type /StructElem /S /Span /P 6 0 R /K [7 0 R] >>")
		if _, cause := reflowed(t, plain, 0, "The quick brown fox leaps over the lazy dog"); cause != "" {
			t.Errorf("setup: the same tree with no replacement text fell back on %q", cause)
		}
	})
	t.Run("a parent tree nested under /Kids is read too", func(t *testing.T) {
		// The fix-pass re-review: `rowFor` read only a root /Nums, and multi-page producers nest the number tree.
		content := "/P <</MCID 0>> BDC " + twoLines + " EMC"
		pdf := pageWith(content, "", "/StructTreeRoot 6 0 R /MarkInfo << /Marked true >> ", "/StructParents 0 ", helvetica,
			"<< /Type /StructTreeRoot /K [7 0 R] /ParentTree << /Kids [9 0 R] >> >>",
			"<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 0 >>",
			"<< /Type /StructElem /S /Span /P 6 0 R /ActualText (old words) /K [7 0 R] >>",
			"<< /Limits [0 0] /Nums [0 [7 0 R]] >>")
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != causeReplacementText {
			t.Errorf("fell back on %q, want %s", cause, causeReplacementText)
		}
	})
	t.Run("a name VALUE in an inline property list is not a key", func(t *testing.T) {
		pdf := helveticaPage("/Span <</Foo /Alt /Lang (en)>> BDC " + twoLines + " EMC")
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != "" {
			t.Errorf("fell back on %q: /Alt is the value of /Foo here, not a key", cause)
		}
	})
	t.Run("a character with two codes is drawn with the one the page already shows", func(t *testing.T) {
		// A subset whose ToUnicode maps both <5A> and <81> to Z; the page draws Z only as <81> (in "Zed"), so <5A> — the
		// font's first — may have no glyph, and the drawn one must be chosen rather than the edit refused.
		cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <00> <FF> endcodespacerange\n" +
			"2 beginbfchar <5A> <005A> <81> <005A> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
		var w []string
		for c := 32; c <= 129; c++ {
			w = append(w, "500")
		}
		font := "<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Calibri /FirstChar 32 /LastChar 129 /Widths [" +
			strings.Join(w, " ") + "] /Encoding /WinAnsiEncoding /ToUnicode 6 0 R >>"
		content := strings.Replace(twoLines, " ET", " T* T* (\x81ed) Tj ET", 1)
		pdf := pageWith(content, "", "", "", map[string]string{"F1": font}, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap))
		out, cause := reflowed(t, pdf, 0, "The quick brown fox jumps over the lazy Zog")
		if cause != "" {
			t.Fatalf("fell back on %q; Z is drawn on this page as <81>", cause)
		}
		if !bytes.Contains(pageContentOf(t, out, 1), []byte("<81>")) {
			t.Errorf("the new Z was not drawn with <81>, the code the page shows")
		}
	})
	t.Run("a no-break space keeps its word whole", func(t *testing.T) {
		// A full TrueType font with a width for every WinAnsi code through 0xA0 — core Helvetica's metrics carry none for it.
		font := "<< /Type /Font /Subtype /TrueType /BaseFont /Arial /FirstChar 32 /LastChar 160 /Widths [" +
			strings.TrimSpace(strings.Repeat("500 ", 129)) + "] /Encoding /WinAnsiEncoding >>"
		pdf := pageWith("BT /F1 12 Tf 14 TL 72 700 Td (Section\xa05 of the quick brown fox) Tj T* (over the lazy dog) Tj ET", "", "", "",
			map[string]string{"F1": font})
		edit := "Section 5 of the quick brown fox leaps over the lazy dog"
		_, after, _ := rewriteReadsBack(t, pdf, edit)
		lines, _, _ := paragraphWords(after)
		if lines[0][0].text() != "Section 5" {
			t.Errorf("the first word reads %q; the no-break space was broken", lines[0][0].text())
		}
	})
	t.Run("the words and the text the user edits are cut at the same gaps", func(t *testing.T) {
		// "foo" at 20pt then "bar" at 10pt, 2pt apart: under 0.15 em of the LINE (3pt), over 0.15 em of the later run
		// (1.5pt). Grouping reads "foobar"; the words must be cut the same way, or the edit re-spells a word it kept.
		pdf := helveticaPage("BT 14 TL /F1 20 Tf 1 0 0 1 72 700 Tm (foo) Tj /F1 10 Tf 1 0 0 1 101.8 700 Tm (bar and more) Tj 1 0 0 1 72 680 Tm (words here) Tj ET")
		l, _ := layoutOf(t, pdf)
		if !strings.HasPrefix(l.paragraphs[0].text(), "foobar") {
			t.Fatalf("setup: grouping reads %q, not one word foobar", l.paragraphs[0].text())
		}
		lines, _, cause := paragraphWords(l.paragraphs[0])
		if cause == "" && lines[0][0].text() != "foobar" {
			t.Errorf("the paragraph reads foobar and its first word is %q", lines[0][0].text())
		}
	})
	t.Run("a space is measured at the size of the word it follows", func(t *testing.T) {
		pdf := helveticaPage("BT 14 TL 72 700 Td /F1 12 Tf (The quick ) Tj /F1 18 Tf (BIG ) Tj /F1 12 Tf (brown fox jumps) Tj T* (over the lazy dog) Tj ET")
		before, after, _ := rewriteReadsBack(t, pdf, "The BIG quick brown fox jumps")
		_, space, _ := paragraphWords(before)
		lines, _, _ := paragraphWords(after)
		for i, l := range lines {
			for wi := 1; wi < len(l); wi++ {
				if gap := l[wi].startX - (l[wi-1].startX + l[wi-1].width); math.Abs(gap-space) > 1e-6 {
					t.Errorf("line %d: %q to %q is %v apart; the paragraph's space is %v", i, l[wi-1].text(), l[wi].text(), gap, space)
				}
			}
		}
	})
	t.Run("an empty edit is refused, not a deleted paragraph", func(t *testing.T) {
		for _, text := range []string{"", "   ", "\n\t"} {
			if _, cause := reflowed(t, helveticaPage(reflowPara+reflowTail), 0, text); cause != causeEmpty {
				t.Errorf("%q fell back on %q, want %s", text, cause, causeEmpty)
			}
		}
	})
	t.Run("a word drawn in two styles may not be reused, because which was meant is a guess", func(t *testing.T) {
		pdf := twoFontPage("F1", "F2", "BT /F1 12 Tf 14 TL 72 700 Td (Read ) Tj /F2 12 Tf (this) Tj /F1 12 Tf ( and this again) Tj T* (over the lazy dog) Tj ET")
		if _, cause := reflowed(t, pdf, 0, "See this and this again over the lazy dog"); cause != causeAmbiguousStyle {
			t.Errorf("fell back on %q, want %s", cause, causeAmbiguousStyle)
		}
		if _, cause := reflowed(t, pdf, 0, "See that and that again over the lazy dog"); cause != "" {
			t.Errorf("setup: an edit using neither \"this\" fell back on %q", cause)
		}
	})
	t.Run("invisible text is refused: it is a search layer, and the page does not show its words", func(t *testing.T) {
		pdf := helveticaPage("BT 3 Tr /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj ET")
		if _, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog"); cause != causeInvisible {
			t.Errorf("fell back on %q, want %s", cause, causeInvisible)
		}
		if _, cause := reflowed(t, helveticaPage(strings.Replace(twoLines, "BT ", "BT 2 Tr ", 1)), 0, "The quick brown fox leaps over the lazy dog"); cause != "" {
			t.Errorf("setup: visible (stroked) text fell back on %q", cause)
		}
	})
	t.Run("vertical writing is refused", func(t *testing.T) {
		cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <0000> <FFFF> endcodespacerange\n" +
			"3 beginbfchar <0001> <0061> <0002> <0062> <0003> <0020> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
		show := "BT /F1 10 Tf 14 TL 72 700 Td <000100030002> Tj T* <00020003 0001> Tj ET"
		font := func(enc string) []byte {
			return assembleFixture(map[int]string{
				1: "<< /Type /Catalog /Pages 2 0 R >>",
				2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
				3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
				4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(show), show),
				5: "<< /Type /Font /Subtype /Type0 /BaseFont /V /Encoding /" + enc + " /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
				6: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /V /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /CIDToGIDMap /Identity /DW 500 >>",
				8: "<< /Type /FontDescriptor /FontName /V /Flags 4 /FontBBox [0 -200 1000 800] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 >>",
				7: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
			})
		}
		if _, cause := reflowed(t, font("Identity-V"), 0, "b a b a"); cause != causeVertical {
			t.Errorf("fell back on %q, want %s", cause, causeVertical)
		}
		if _, cause := reflowed(t, font("Identity-H"), 0, "b a b a"); cause == causeVertical {
			t.Errorf("setup: horizontal Identity-H was refused as vertical")
		}
	})
	t.Run("a coordinate that multiplies to infinity is refused, never written as Inf", func(t *testing.T) {
		big := "1" + strings.Repeat("0", 200)
		// Each factor parses finite; the CTM times the text matrix is 10^400, which is not.
		pdf := helveticaPage(big + " 0 0 " + big + " 0 0 cm BT /F1 12 Tf 14 TL " + big + " 0 0 " + big + " 72 700 Tm (The quick brown fox jumps) Tj T* (over the lazy dog) Tj ET")
		ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Skipf("the validator refuses the operand, so no request can carry it: %v", err)
		}
		out, err := reflowParagraph(ctx, 1, 0, "The quick brown fox leaps over the lazy dog")
		if err != nil {
			t.Fatal(err)
		}
		if s := string(out.content); strings.Contains(s, "Inf") || strings.Contains(s, "NaN") {
			t.Errorf("the rewrite wrote a number with no spelling: %q", s)
		}
		if out.content == nil && out.cause != "degenerate-state" {
			t.Errorf("refused as %q; the stimulus is an infinite scale, which is degenerate-state", out.cause)
		}
	})
	t.Run("a ' show and a Tz around the paragraph leave what follows where it was", func(t *testing.T) {
		for _, c := range []struct{ name, content string }{
			{"'", "BT /F1 12 Tf 14 TL 72 714 Td (The quick brown fox jumps) ' (over the lazy dog) ' T* T* (Signature line) Tj ET"},
			{"Tz", "BT 80 Tz /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj T* T* (Signature line) Tj ET"},
			{"Ts", "BT 2 Ts /F1 12 Tf 14 TL 72 700 Td (The quick brown fox jumps) Tj T* (over the lazy dog) Tj T* T* (Signature line) Tj ET"},
		} {
			t.Run(c.name, func(t *testing.T) {
				pdf := helveticaPage(c.content)
				_, beforeRuns := layoutOf(t, pdf)
				_, _, afterRuns := rewriteReadsBack(t, pdf, "The quick brown fox leaps over the lazy dog")
				sig := func(runs []textRun) textRun {
					for _, r := range runs {
						if r.text == "Signature line" {
							return r
						}
					}
					t.Fatal("no signature line")
					return textRun{}
				}
				if b, a := sig(beforeRuns), sig(afterRuns); math.Abs(a.x-b.x) > 1e-6 || math.Abs(a.y-b.y) > 1e-6 || a.width != b.width {
					t.Errorf("the signature line moved from (%v, %v, w %v) to (%v, %v, w %v)", b.x, b.y, b.width, a.x, a.y, a.width)
				}
			})
		}
	})
}
