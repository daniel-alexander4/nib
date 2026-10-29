package pdfops

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/font"
)

// coreInkPage is one page showing text once, at 20pt on a baseline 700 up a 612×792 page, in the font dictionary
// given (object 5; extra holds any further objects it names).
func coreInkPage(fontDict, show string, extra map[int]string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: streamObj("BT /F1 20 Tf 72 700 Td " + show + " Tj ET\n"),
		5: fontDict,
	}
	for k, v := range extra {
		objs[k] = v
	}
	return assembleFixture(objs)
}

// widths95 gives a font that is not one pdfcpu has metrics for an advance for every ASCII code, as a real document
// does: a run with no width is not on the map at all.
var widths95 = " /FirstChar 32 /LastChar 126 /Widths [" + strings.Repeat("600 ", 95) + "]"

// asciiToUnicode is a /ToUnicode CMap reading each ASCII code as its own character.
const asciiToUnicode = "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /X def /CMapType 2 def " +
	"1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfrange <20> <7E> <0020> endbfrange endcmap end end"

// inkTopEm is how far above its baseline the page's one run is covered, in sizes, and whether it has an ink at all.
func inkTopEm(t *testing.T, pdf []byte) (top, bottom float64, has bool) {
	t.Helper()
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) != 1 {
		t.Fatalf("the page maps %d runs, want 1", len(m.Text))
	}
	run := m.Text[0]
	if run.Ink == nil {
		return 0, 0, false
	}
	// Fractions of the page from its top; the baseline is 700 up, the size 20.
	return ((1-run.Ink[0])*792 - 700) / 20, (700 - (1-run.Ink[1])*792) / 20, true
}

// TestPrintInAnUnembeddedStandardFontIsBoxedToTheTopOfAdobesBoxForIt — `/pending 851`. Such a run had no ink and was
// covered a full size above its baseline; it is covered to the top of the Core 14 font's own box, and its bottom is
// the reach's, unchanged.
func TestPrintInAnUnembeddedStandardFontIsBoxedToTheTopOfAdobesBoxForIt(t *testing.T) {
	for _, c := range []struct {
		name, dict string
		extra      map[int]string
		core       string
	}{
		{"a standard font by its own name", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>", nil, "Courier"},
		{"a standard font with no /Encoding", "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold >>", nil, "Times-Bold"},
		{"the Reference's alternative name", "<< /Type /Font /Subtype /TrueType /BaseFont /Arial,Bold /Encoding /WinAnsiEncoding" + widths95 + " >>", nil, "Helvetica-Bold"},
		{"the family name with its spaces, as Word writes it", "<< /Type /Font /Subtype /TrueType /BaseFont /Times#20New#20Roman /Encoding /WinAnsiEncoding" + widths95 + " >>", nil, "Times-Roman"},
		{"the same, bold italic", "<< /Type /Font /Subtype /TrueType /BaseFont /Times#20New#20Roman,BoldItalic /Encoding /WinAnsiEncoding" + widths95 + " >>", nil, "Times-BoldItalic"},
		{"a PostScript name, with a descriptor and no program",
			"<< /Type /Font /Subtype /TrueType /BaseFont /ArialMT /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R" + widths95 + " >>",
			map[int]string{6: "<< /Type /FontDescriptor /FontName /ArialMT /Flags 32 /FontBBox [-665 -325 2000 1006] /ItalicAngle 0 /Ascent 905 /Descent -212 /CapHeight 716 /StemV 80 >>"},
			"Helvetica"},
	} {
		bb, berr := font.BoundingBox(context.Background(), c.core)
		if berr != nil {
			t.Fatalf("%s: %v", c.core, berr)
		}
		want := bb.UR.Y / 1000
		if !(want > 0.7 && want < ascentEm) {
			t.Fatalf("%s: pdfcpu's box for %s tops out at %v — not a bound that tightens anything", c.name, c.core, want)
		}
		top, bottom, has := inkTopEm(t, coreInkPage(c.dict, "(Sample 123, done.)", c.extra))
		if !has {
			t.Errorf("%s: the run has no ink, so its box still reaches a full size up", c.name)
			continue
		}
		if math.Abs(top-want) > 1e-6 {
			t.Errorf("%s: covered %.4f of a size above the baseline, want the top of %s's box, %.4f", c.name, top, c.core, want)
		}
		if math.Abs(bottom-descentEm) > 1e-6 {
			t.Errorf("%s: covered %.4f of a size below the baseline, want the reach's own %.2f — a substitute's tail is not Adobe's", c.name, bottom, descentEm)
		}
	}
}

// TestOnlyAnExactStandardFontDrawingItsASCIIGlyphsIsBoxedToAdobesBox — the refusals, each of which leaves the run
// with no ink and so covered to its reach exactly as before. A viewer substitutes an unembedded font, and what it
// draws is bounded by Adobe's box only for the glyphs a substitute is drawn to stand in for.
func TestOnlyAnExactStandardFontDrawingItsASCIIGlyphsIsBoxedToAdobesBox(t *testing.T) {
	const winAnsi = "/Encoding /WinAnsiEncoding"
	for _, c := range []struct {
		name, dict, show string
		extra            map[int]string
	}{
		{"a subset tag", "<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Courier " + winAnsi + widths95 + " >>", "(Sample)", nil},
		{"a name that is close", "<< /Type /Font /Subtype /TrueType /BaseFont /Times-New-Roman " + winAnsi + widths95 + " >>", "(Sample)", nil},
		{"a family name with spaces that is not listed", "<< /Type /Font /Subtype /TrueType /BaseFont /Courier#20New " + winAnsi + widths95 + " >>", "(Sample)", nil},
		{"a pi font: Symbol", "<< /Type /Font /Subtype /Type1 /BaseFont /Symbol /ToUnicode 8 0 R" + widths95 + " >>", "(abc)", map[int]string{8: streamObj(asciiToUnicode)}},
		{"a pi font: ZapfDingbats", "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats /ToUnicode 8 0 R" + widths95 + " >>", "(abc)", map[int]string{8: streamObj(asciiToUnicode)}},
		{"another case", "<< /Type /Font /Subtype /Type1 /BaseFont /courier " + winAnsi + widths95 + " >>", "(Sample)", nil},
		{"a font with a program, even one that cannot be read",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Courier " + winAnsi + " /FontDescriptor 6 0 R >>", "(Sample)",
			map[int]string{
				6: "<< /Type /FontDescriptor /FontName /Courier /Flags 32 /FontBBox [0 0 600 800] /ItalicAngle 0 /Ascent 629 /Descent -157 /CapHeight 562 /StemV 50 /FontFile 7 0 R >>",
				7: "<< /Length 18 /Length1 18 /Length2 0 /Length3 0 >>\nstream\nnot a font program\nendstream",
			}},
		{"glyphs renamed by /Differences",
			"<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [65 /Aring] >> /ToUnicode 8 0 R" + widths95 + " >>", "(Sample)",
			map[int]string{8: streamObj(asciiToUnicode)}},
		{"a code outside ASCII: an accented capital", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier " + winAnsi + " >>", "(Sampl\\311)", nil},
	} {
		if top, _, has := inkTopEm(t, coreInkPage(c.dict, c.show, c.extra)); has {
			t.Errorf("%s: the run is covered only %.3f of a size above its baseline; it must keep its full reach", c.name, top)
		}
	}
}

// TestEveryStandardFontAliasNamesAFontPdfcpuHasABoxFor — the table's own integrity: a name mapped to a font with no
// box would silently give nothing, and one mapped to a box above the reach would claim a bound that binds nothing.
func TestEveryStandardFontAliasNamesAFontPdfcpuHasABoxFor(t *testing.T) {
	if len(coreFontAlias) != 40 {
		t.Errorf("the alias table has %d names, want 40: twelve fonts by three names each, and Times New Roman's four with spaces", len(coreFontAlias))
	}
	cores := map[string]int{}
	for name, core := range coreFontAlias {
		cores[core]++
		if !font.IsCoreFont(core) || core == "Symbol" || core == "ZapfDingbats" {
			t.Errorf("%s maps to %q, which is not a standard text font", name, core)
			continue
		}
		if box, _ := font.BoundingBox(context.Background(), core); box == nil || !(box.UR.Y > 700 && box.UR.Y < 1000) {
			t.Errorf("%s: pdfcpu's box for %s is %v", name, core, box)
		}
	}
	if len(cores) != 12 {
		t.Errorf("the table reaches %d standard fonts, want 12: %s", len(cores), fmt.Sprint(cores))
	}
}
