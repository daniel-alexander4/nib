package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// dropCIDSets removes the `/CIDSet` stream from every embedded CID font's descriptor —
// `PLAN-accessibility.md` P04.S02.
//
// # The clause, and why removal is the honest answer rather than the lazy one
//
// PDF/UA rule 7.21.4.2 t2: *"If the FontDescriptor dictionary of an embedded CID font contains a
// CIDSet stream, then it shall identify all CIDs which are present in the font program, regardless
// of whether a CID in the font is referenced or used by the PDF or not."*
//
// pdfcpu writes one that does not. Its own function comment says so — `font/fontDict.go:252`,
// *"CIDSet computes a CIDSet for used glyphs"* — and it is written unconditionally at `:357` with
// no configuration to turn it off. Measured on a one-line authored page: a 162-byte CIDSet with
// **16 bits set**, which is the used-glyph count and not the subsetted program's.
//
// **The clause is conditional on the stream being present at all**, and PDF/UA does not require
// one. So the choice is between computing a correct set — which means parsing the subsetted
// `glyf`/`loca` tables pdfcpu just wrote, to restate something the font program already says — and
// not making the claim. ADR-031's law 1 is the same principle in a different field: a document
// should not assert what it cannot support, and the assertion here buys nothing.
//
// # Why this does not break PDF/A
//
// `/CIDSet` is **required for subset fonts by PDF/A-1 only**; PDF/A-2 dropped it. Nib targets
// **PDF/A-2b** — `pdfa.go:49` writes `<pdfaid:part>2</pdfaid:part>` and the veraPDF tests assert
// the `2b` flavour — and the Ghostscript path rebuilds font descriptors itself. Checked, not
// assumed: `TestDroppingCIDSetsKeepsPDFAConformance`.
//
// # Where it is applied
//
// The two doors where nib EMBEDS a font of its own: the Markdown conversion and the OCR text
// layer. It is deliberately not applied to documents nib merely rewrites — a `/CIDSet` in a user's
// own document is their file's business, and an office conversion's fonts come from LibreOffice,
// which does not produce this defect (measured: a converted document fails neither font clause).
func dropCIDSets(pdf []byte) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		for _, e := range ctx.XRefTable.Table {
			if e == nil || e.Object == nil {
				continue
			}
			d, ok := e.Object.(types.Dict)
			if !ok {
				continue
			}
			// FontDescriptor is the only dictionary that carries the key, and checking /Type
			// rather than the key's presence means a stray "CIDSet" elsewhere is left alone.
			if ty, _ := d["Type"].(types.Name); ty != "FontDescriptor" {
				continue
			}
			delete(d, "CIDSet")
		}
		return nil
	})
}

// embeddedFontsAreHonest is the tail every door that embeds a nib-supplied face runs.
//
// It is a named function rather than a bare `dropCIDSets` call at each site so the rule has ONE
// door (ADR-009) and so the sites read as what they are: *this output carries fonts nib put there,
// and nib does not make claims about them it cannot support.* A failure returns the document
// UNCHANGED rather than an error — the fonts are embedded either way, and failing a conversion over
// a metadata clean-up is the shape P03's phase-close review had to undo at three other sites.
func embeddedFontsAreHonest(pdf []byte) []byte {
	out, err := dropCIDSets(pdf)
	if err != nil {
		return pdf
	}
	return out
}
