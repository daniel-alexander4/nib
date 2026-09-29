package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// onePageFixture is a one-page document drawing content with font /F1 = fontDict.
func onePageFixture(content, fontDict string) []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: fontDict,
	})
}

func glyphRunsOf(t *testing.T, pdf []byte, page int) []textRun {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageGlyphRuns(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	return pr.runs
}

// TestARunsGlyphsSumToItsWidth — `PLAN-text-reflow.md` P06.S01: the glyphs a run keeps are the run, not a second opinion
// about it. Over the generated and real-producer corpora, every run read with its glyphs has one glyph per code, and its
// glyphs' kerns and advances plus the kern after the last glyph sum to the width the run already reports; reading with
// glyphs changes nothing a reader without them sees.
//
// **The stimulus is asserted**: the corpus must carry `TJ` kerns between glyphs, or "kerns sit between glyphs" was never
// exercised.
func TestARunsGlyphsSumToItsWidth(t *testing.T) {
	corpora := []lawOneCorpus{
		{name: "generated", docs: runCorpus(t)},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	runs, glyphs, kerned, undecoded := 0, 0, 0, 0
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				continue
			}
			for p := 1; p <= ctx.PageCount; p++ {
				plain, perr := readPageRuns(ctx, p)
				kept, kerr := readPageGlyphRuns(ctx, p)
				if (perr == nil) != (kerr == nil) {
					t.Errorf("%s / %s p%d: with glyphs the read is %v, without %v", corp.name, doc.name, p, kerr, perr)
					continue
				}
				if perr != nil {
					continue
				}
				if len(plain.runs) != len(kept.runs) {
					t.Errorf("%s / %s p%d: %d runs without glyphs, %d with", corp.name, doc.name, p, len(plain.runs), len(kept.runs))
					continue
				}
				for i, r := range kept.runs {
					runs++
					if r.width != plain.runs[i].width || r.text != plain.runs[i].text || len(plain.runs[i].glyphs) != 0 {
						t.Fatalf("%s / %s p%d run %d: keeping glyphs changed what a reader without them sees", corp.name, doc.name, p, i)
					}
					if len(r.glyphs) != r.codes {
						t.Errorf("%s / %s p%d run %d: %d glyphs for %d codes", corp.name, doc.name, p, i, len(r.glyphs), r.codes)
					}
					sum := r.kernAfter
					joined, allDecoded := "", true
					for gi, g := range r.glyphs {
						glyphs++
						joined += g.text
						allDecoded = allDecoded && g.decoded
						if !g.decoded {
							undecoded++
						}
						sum += g.kern + g.advance
						if gi > 0 && g.kern != 0 {
							kerned++
						}
						if g.widthSrc == widthNone && g.fontWidth != 0 {
							t.Errorf("%s / %s p%d: a glyph with no width source carries width %v", corp.name, doc.name, p, g.fontWidth)
						}
					}
					if joined != r.text || allDecoded != r.decoded {
						t.Errorf("%s / %s p%d run %d: glyph texts %q (all decoded %v), the run %q (decoded %v)",
							corp.name, doc.name, p, i, joined, allDecoded, r.text, r.decoded)
					}
					if math.Abs(sum-r.width) > 1e-6*math.Max(1, math.Abs(r.width)) {
						t.Errorf("%s / %s p%d run %d %q: glyphs sum to %v, the run is %v wide", corp.name, doc.name, p, i, r.text, sum, r.width)
					}
				}
			}
		}
	}
	if kerned == 0 {
		t.Errorf("%d runs and %d glyphs, and not one TJ kern between glyphs — the kern placement was never exercised", runs, glyphs)
	}
	if undecoded == 0 {
		t.Errorf("no glyph in the corpus failed to decode, so a glyph's `decoded` was never checked against its run's")
	}
	t.Logf("%d runs, %d glyphs, %d kerned between glyphs, %d undecoded", runs, glyphs, kerned, undecoded)
}

// TestAKernSitsBetweenGlyphs — a hand-built `TJ` with its answer derived by hand: Helvetica at 10pt, `[(A) -200 (B)] TJ`.
// A is 667/1000 × 10 = 6.67 wide with no kern before it; the -200 moves B right by 200/1000 × 10 = 2.0, which belongs
// to B as the kern before it — not to A's advance and not to the run's tail.
func TestAKernSitsBetweenGlyphs(t *testing.T) {
	pdf := onePageFixture("BT /F1 10 Tf 72 700 Td [(A) -200 (B) 50] TJ ET",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	runs := glyphRunsOf(t, pdf, 1)
	if len(runs) != 1 || len(runs[0].glyphs) != 2 {
		t.Fatalf("want one run of two glyphs, got %+v", runs)
	}
	a, b := runs[0].glyphs[0], runs[0].glyphs[1]
	near := func(x, y float64) bool { return math.Abs(x-y) < 1e-9 }
	if !near(a.kern, 0) || !near(a.advance, 6.67) || !near(b.kern, 2.0) || !near(b.advance, 6.67) || !near(runs[0].kernAfter, -0.5) {
		t.Errorf("A kern %v adv %v; B kern %v adv %v; after %v — want 0, 6.67; 2, 6.67; -0.5",
			a.kern, a.advance, b.kern, b.advance, runs[0].kernAfter)
	}
	if string(a.code) != "A" || a.text != "A" || !a.decoded || a.widthSrc != widthFromStd14 {
		t.Errorf("glyph A is %+v", a)
	}
}

// TestAGlyphWithNoWidthIsCarriedAsNone — law 2: a font with no widths anywhere (a non-core /BaseFont, no /Widths, no
// descriptor) measures its glyphs as `none`, and the glyph says so rather than carrying a 0 read as known.
func TestAGlyphWithNoWidthIsCarriedAsNone(t *testing.T) {
	pdf := onePageFixture("BT /F1 10 Tf 72 700 Td (A) Tj ET",
		"<< /Type /Font /Subtype /Type1 /BaseFont /NotACoreFace >>")
	runs := glyphRunsOf(t, pdf, 1)
	if len(runs) != 1 || len(runs[0].glyphs) != 1 {
		t.Fatalf("want one run of one glyph, got %+v", runs)
	}
	if g := runs[0].glyphs[0]; g.widthSrc != widthNone || g.fontWidth != 0 {
		t.Errorf("a glyph with no width anywhere came back as %v from %q", g.fontWidth, g.widthSrc)
	}
}

// TestEveryDecodedGlyphIsDrawableByItsOwnCode — `PLAN-text-reflow.md` P06.S02: the inverse answers every character the
// reader decoded with, among its codes, the code the glyph was drawn with. Over the generated and real-producer corpora,
// for simple and two-byte fonts both — each asserted present, so the two arms of the inverse were both reached.
func TestEveryDecodedGlyphIsDrawableByItsOwnCode(t *testing.T) {
	corpora := []lawOneCorpus{
		{name: "generated", docs: runCorpus(t)},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	checked := map[bool]int{} // by twoByte
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				continue
			}
			for p := 1; p <= ctx.PageCount; p++ {
				pr, err := readPageGlyphRuns(ctx, p)
				if err != nil {
					continue
				}
				for _, r := range pr.runs {
					if r.face == nil || !r.face.splittable {
						continue
					}
					want := 1
					if r.face.twoByte {
						want = 2
					}
					for _, g := range r.glyphs {
						if !g.decoded || g.text == "" || len(g.code) != want {
							continue
						}
						checked[r.face.twoByte]++
						found, seen := false, map[string]bool{}
						for _, c := range r.face.codesFor(g.text) {
							found = found || bytes.Equal(c, g.code)
							if seen[string(c)] {
								t.Fatalf("%s / %s p%d: the inverse offers %x twice for %q — one code, counted as two",
									corp.name, doc.name, p, c, g.text)
							}
							seen[string(c)] = true
						}
						if !found {
							t.Fatalf("%s / %s p%d: %q decodes from %x, and the inverse offers %x", corp.name, doc.name, p,
								g.text, g.code, r.face.codesFor(g.text))
						}
					}
				}
			}
		}
	}
	if checked[false] == 0 || checked[true] == 0 {
		t.Errorf("simple-font glyphs checked %d, two-byte %d — both arms of the inverse must be reached", checked[false], checked[true])
	}
	t.Logf("glyphs checked: %d simple-font, %d two-byte", checked[false], checked[true])
}

// codesOfOnlyRun reads fixture pdf's single run and answers codesFor for each text.
func codesOfOnlyRun(t *testing.T, pdf []byte) *runFont {
	t.Helper()
	runs := glyphRunsOf(t, pdf, 1)
	if len(runs) != 1 || runs[0].face == nil {
		t.Fatalf("want one run with its font, got %+v", runs)
	}
	return runs[0].face
}

// TestAFontAnswersWhatItCanDraw — the three answers D8 needs, on hand-built fonts: a code, several codes, or none.
func TestAFontAnswersWhatItCanDraw(t *testing.T) {
	t.Run("a core font draws what its encoding names and nothing else", func(t *testing.T) {
		f := codesOfOnlyRun(t, onePageFixture("BT /F1 10 Tf 72 700 Td (A) Tj ET",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"))
		if got := f.codesFor("A"); len(got) != 1 || string(got[0]) != "A" {
			t.Errorf("Helvetica/WinAnsi draws A with %x", got)
		}
		if got := f.codesFor("é"); len(got) != 1 || got[0][0] != 0xE9 {
			t.Errorf("WinAnsi's é is 0xE9; the inverse offers %x", got)
		}
		if got := f.codesFor("Ω"); got != nil {
			t.Errorf("WinAnsi has no Ω, and the inverse offers %x — D8's trigger would never fire", got)
		}
	})
	t.Run("a subset draws only what its ToUnicode names, and two codes for one character are both returned", func(t *testing.T) {
		cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <0000> <FFFF> endcodespacerange\n" +
			"3 beginbfchar <0001> <0061> <0002> <0061> <0003> <0062> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
		show := "BT /F1 10 Tf 72 700 Td <00010003> Tj ET"
		pdf := assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
			4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(show), show),
			5: "<< /Type /Font /Subtype /Type0 /BaseFont /ABCDEF+Sub /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
			6: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /ABCDEF+Sub /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /CIDToGIDMap /Identity /DW 500 >>",
			8: "<< /Type /FontDescriptor /FontName /ABCDEF+Sub /Flags 4 /FontBBox [0 -200 1000 800] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 >>",
			7: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
		})
		f := codesOfOnlyRun(t, pdf)
		if got := f.codesFor("a"); len(got) != 2 || string(got[0]) != "\x00\x01" || string(got[1]) != "\x00\x02" {
			t.Errorf("a is drawn by <0001> and <0002>; the inverse offers %x", got)
		}
		if got := f.codesFor("b"); len(got) != 1 || string(got[0]) != "\x00\x03" {
			t.Errorf("b is <0003>; the inverse offers %x", got)
		}
		if got := f.codesFor("z"); got != nil {
			t.Errorf("the subset has no z, and the inverse offers %x", got)
		}
	})
	t.Run("a ToUnicode key longer than the font's codes is never offered", func(t *testing.T) {
		cmap := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange <00> <FF> endcodespacerange\n" +
			"2 beginbfchar <42> <0042> <0041> <03A9> endbfchar\nendcmap CMapName currentdict /CMap defineresource pop end end"
		show := "BT /F1 10 Tf 72 700 Td (B) Tj ET"
		f := codesOfOnlyRun(t, assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
			4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(show), show),
			5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /ToUnicode 7 0 R >>",
			7: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cmap), cmap),
		}))
		if got := f.codesFor("B"); len(got) != 1 || string(got[0]) != "B" {
			t.Fatalf("the stimulus: the font's one-byte ToUnicode entry must be read — B offers %x", got)
		}
		if got := f.codesFor("Ω"); got != nil {
			t.Errorf("Ω is mapped only from the two-byte key <0041>, which a one-byte font never draws (and WinAnsi has no Ω); the inverse offers %x", got)
		}
	})
	t.Run("a font the reader cannot split draws nothing it can vouch for", func(t *testing.T) {
		if got := (&runFont{splittable: false, toUni: map[string]string{"A": "A"}}).codesFor("A"); got != nil {
			t.Errorf("an unsplittable font offered %x", got)
		}
	})
}
