package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// How far print in a font with NO embedded program reaches up the page (/pending 851, after ADR-098 and ADR-132).
//
// A viewer SUBSTITUTES an unembedded font, so there is no program whose points bound the ink: the glyphs painted
// are whichever font the viewer picked, and they are not Adobe's. What a standard font does have is Adobe's own
// published box around every glyph of it (the Core 14 metrics' `FontBBox`, which pdfcpu carries), and a substitute
// for a standard font is drawn to stand in for it. So a run in one of the twelve standard text fonts, by its exact
// name or a listed alias of it, is boxed to the TOP of that font's box instead of a full size above the baseline.
//
// # Why the font's box and nothing tighter — measured, on the accuracy corpus's 632 such words
//
// Each bound was laid against the pixels of four renderings (`test/accuracy/reach.mjs`, 300 dpi): poppler with this
// machine's substitutes (URW Nimbus), poppler made to substitute DejaVu, poppler made to substitute Liberation, and
// Ghostscript. Words whose box reaches the line above, and words with their own ink left outside the box:
//
//	                               Nimbus      DejaVu          Liberation   Ghostscript
//	a full size (as it was)        74 / 0      69 / 0          73 / 0       79 / 0
//	Adobe's box per GLYPH          5 / 0       4 / 49 (0.9pt)  5 / 0        5 / 0
//	Adobe's Ascender               9 / 0       8 / 45 (0.6pt)  9 / 0        9 / 0
//	the tallest letter or digit    9 / 0       8 / 6 (0.3pt)   9 / 0        9 / 0
//	Adobe's FontBBox (this)        43 / 0      26 / 0          28 / 0       50 / 0
//
// DejaVu Sans Mono's letters stand 0.13 of a size taller than Courier's, so every bound read from Adobe's own
// glyphs leaves ink showing under it. Per-glyph boxes are therefore refused, and no glyph table is carried.
//
// # What it does not touch
//
//   - **The bottom.** It stays the reach's quarter of a size. Measured, the font's box moves no word's bottom (39 of
//     39 still reach the line below), and a substitute's tail is deeper than Adobe's: under DejaVu one word's ink
//     already stands 0.33pt below the quarter.
//   - **A run with any code outside 0x20–0x7E**, and a font whose `/Encoding` is not one of the three named ones (or
//     absent on a standard font): the tall glyphs of a font are its accented capitals, a substitute's are not
//     Adobe's, and under `/Differences` a code is whatever glyph the document says. In those three encodings the
//     codes 0x20–0x7E are the same ninety-five glyphs, so the code itself says which glyph is drawn.
//   - **A font with a program** (`/FontFile`, `/FontFile2`, `/FontFile3`), read or refused: it is not substituted,
//     and its own points are the only bound (fontink.go).
//   - **Symbol and ZapfDingbats**, a subset tag, and any name not listed below, however close.
//
// Declared: a viewer substituting a font whose ASCII glyphs stand taller than Adobe's whole-font box plus the
// reader's one-point pad would leave ink above the box. None of the four renderings does.

// coreFontAlias is every name read as a standard text font, to the Core 14 name whose box it takes: the twelve
// themselves, the alternative names the PDF Reference lists for them (1.7, implementation note 62), and the
// PostScript names of the fonts those alternatives are. Exact, case and punctuation included.
var coreFontAlias = map[string]string{
	"Helvetica": "Helvetica", "Arial": "Helvetica", "ArialMT": "Helvetica",
	"Helvetica-Bold": "Helvetica-Bold", "Arial,Bold": "Helvetica-Bold", "Arial-BoldMT": "Helvetica-Bold",
	"Helvetica-Oblique": "Helvetica-Oblique", "Arial,Italic": "Helvetica-Oblique", "Arial-ItalicMT": "Helvetica-Oblique",
	"Helvetica-BoldOblique": "Helvetica-BoldOblique", "Arial,BoldItalic": "Helvetica-BoldOblique", "Arial-BoldItalicMT": "Helvetica-BoldOblique",

	"Times-Roman": "Times-Roman", "TimesNewRoman": "Times-Roman", "TimesNewRomanPSMT": "Times-Roman",
	"Times-Bold": "Times-Bold", "TimesNewRoman,Bold": "Times-Bold", "TimesNewRomanPS-BoldMT": "Times-Bold",
	"Times-Italic": "Times-Italic", "TimesNewRoman,Italic": "Times-Italic", "TimesNewRomanPS-ItalicMT": "Times-Italic",
	"Times-BoldItalic": "Times-BoldItalic", "TimesNewRoman,BoldItalic": "Times-BoldItalic", "TimesNewRomanPS-BoldItalicMT": "Times-BoldItalic",

	"Courier": "Courier", "CourierNew": "Courier", "CourierNewPSMT": "Courier",
	"Courier-Bold": "Courier-Bold", "CourierNew,Bold": "Courier-Bold", "CourierNewPS-BoldMT": "Courier-Bold",
	"Courier-Oblique": "Courier-Oblique", "CourierNew,Italic": "Courier-Oblique", "CourierNewPS-ItalicMT": "Courier-Oblique",
	"Courier-BoldOblique": "Courier-BoldOblique", "CourierNew,BoldItalic": "Courier-BoldOblique", "CourierNewPS-BoldItalicMT": "Courier-BoldOblique",
}

// coreFontTop is the top of Adobe's box for the standard text font d names, in ems above the baseline — or ok false
// where d is not a simple font, carries a program, or is not named exactly as one of them.
func coreFontTop(xt *model.XRefTable, d types.Dict) (top float64, ok bool) {
	if sub := nameVal(d, "Subtype"); sub != "Type1" && sub != "TrueType" {
		return 0, false
	}
	core, listed := coreFontAlias[nameVal(d, "BaseFont")]
	if !listed {
		return 0, false
	}
	if fd, err := xt.DereferenceDict(d["FontDescriptor"]); err == nil && fd != nil {
		for _, key := range []string{"FontFile", "FontFile2", "FontFile3"} {
			if _, has := fd[key]; has {
				return 0, false
			}
		}
	}
	box := font.BoundingBox(core)
	if box == nil || box.UR.Y <= 0 {
		return 0, false
	}
	return box.UR.Y / 1000, true
}
