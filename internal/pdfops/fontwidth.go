package pdfops

import (
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/mdpdf"
)

// The width reader — `PLAN-accessibility.md` P08.S01, which is `PLAN-text-reflow.md` P02.
//
// Given a font dictionary and a character code, the advance the document itself declares, and WHERE
// that number came from. Reflow D2: the dictionary is authoritative and the font program is never
// parsed. Reflow D3: a Base-14 core font is measured through `mdpdf.CoreWidth` and nowhere else.
//
// # Every answer carries its source, and one of the sources is `none`
//
// Law 2 is that no lookup returns a silent zero. A zero IS a legitimate width — a combining mark, a
// zero-width space — so the rule cannot be "never return 0"; it is "never return a number the
// document did not supply without saying so". `none` is the reader saying it does not know.
//
// # Two ways the core-font path invents a number, both refused
//
// pdfcpu's core-font metrics return **1000** for a code whose glyph its WinAnsi map lacks
// (`internal/corefont/metrics.CoreFontCharWidth`), with nothing marking it as a guess. That map is in
// pdfcpu's `internal/`, so it cannot be consulted directly; but Courier is monospaced and every glyph it
// has is 600 wide, so a code Courier measures at 1000 is a code the map does not carry, for every
// WinAnsi core font at once. Symbol and ZapfDingbats use their own maps, which no such discriminator
// covers, so they report `none`.
//
// And those metrics are WinAnsi's. A simple font with no `/Encoding` uses StandardEncoding, which
// disagrees with WinAnsi on some codes, and `/Differences` renames glyphs outright — so the std14
// source is used only when the font says `/WinAnsiEncoding`.

// widthSource says which part of a font dictionary supplied an advance.
type widthSource string

const (
	widthFromWidths  widthSource = "Widths"       // a simple font's /Widths, indexed from /FirstChar
	widthFromMissing widthSource = "MissingWidth" // the descriptor's width for a code outside /Widths
	widthFromW       widthSource = "W"            // a CID font's /W
	widthFromDW      widthSource = "DW"           // a CID font's /DW, for a CID /W does not list
	// widthFromDefault is a CID font with no /DW at all: the specification's default of 1000, which is
	// what a viewer uses. Named apart from DW so a census can tell a declared default from an assumed one.
	widthFromDefault widthSource = "DW default"
	widthFromStd14   widthSource = "std14" // a WinAnsi-encoded Base-14 font, through mdpdf.CoreWidth
	widthNone        widthSource = "none"
)

// maxCID bounds a `/W` range. CIDs are 16-bit; a `cfirst clast w` entry claiming more is malformed,
// and expanding it as written would let one number in a document allocate without limit.
const maxCID = 0xFFFF

// fontWidths is one font's advance table, read once from its dictionary.
type fontWidths struct {
	cid bool
	// descendant is whether a Type0 font resolved to a descendant CIDFont. Without one nothing in the
	// dictionary describes any CID, and every lookup is `none`.
	descendant bool
	w          map[int]float64
	dw         float64
	dwSource   widthSource

	firstChar  int
	widths     []float64 // NaN where the entry is not a number
	missing    float64
	hasMissing bool
	core       string // the Base-14 name when the std14 source applies, else ""
}

// readFontWidths reads a font dictionary's advance table.
func readFontWidths(xt *model.XRefTable, fontObj types.Object) fontWidths {
	var f fontWidths
	d, err := xt.DereferenceDict(fontObj)
	if err != nil || d == nil {
		return f
	}
	if st := d.NameEntry("Subtype"); st != nil && *st == "Type0" {
		f.cid = true
		kids, kerr := xt.DereferenceArray(d["DescendantFonts"])
		if kerr != nil || len(kids) == 0 {
			return f
		}
		desc, derr := xt.DereferenceDict(kids[0])
		if derr != nil || desc == nil {
			return f
		}
		f.descendant = true
		f.w = readW(xt, desc["W"])
		if v, ok := pdfNumber(xt, desc["DW"]); ok {
			f.dw, f.dwSource = v, widthFromDW
		} else {
			f.dw, f.dwSource = 1000, widthFromDefault
		}
		return f
	}

	if arr, aerr := xt.DereferenceArray(d["Widths"]); aerr == nil && arr != nil {
		if fc, ok := pdfNumber(xt, d["FirstChar"]); ok {
			f.firstChar = int(fc)
			f.widths = make([]float64, len(arr))
			for i, o := range arr {
				if v, vok := pdfNumber(xt, o); vok {
					f.widths[i] = v
				} else {
					f.widths[i] = math.NaN()
				}
			}
		}
	}
	if fd, ferr := xt.DereferenceDict(d["FontDescriptor"]); ferr == nil && fd != nil {
		if v, ok := pdfNumber(xt, fd["MissingWidth"]); ok {
			f.missing, f.hasMissing = v, true
		}
	}
	if bf := d.NameEntry("BaseFont"); bf != nil && font.IsCoreFont(*bf) &&
		*bf != "Symbol" && *bf != "ZapfDingbats" {
		if enc := d.NameEntry("Encoding"); enc != nil && *enc == "WinAnsiEncoding" {
			f.core = *bf
		}
	}
	return f
}

// advance returns a code's width in glyph space (thousandths of the font size) and its source.
func (f fontWidths) advance(code int) (float64, widthSource) {
	if f.cid {
		if !f.descendant {
			return 0, widthNone
		}
		if v, ok := f.w[code]; ok {
			return v, widthFromW
		}
		return f.dw, f.dwSource
	}
	if i := code - f.firstChar; i >= 0 && i < len(f.widths) && !math.IsNaN(f.widths[i]) {
		return f.widths[i], widthFromWidths
	}
	if f.hasMissing {
		return f.missing, widthFromMissing
	}
	if f.core != "" && code >= 0 && code <= 0xFF && coreCodeMapped(code) {
		// One code per call: CoreWidth normalises UTF-8, and two code bytes can happen to spell a valid
		// UTF-8 sequence, which it would fold into one. A single byte never can. Measured at size 1000,
		// where pdfcpu's user-space width IS the glyph-space width.
		return mdpdf.CoreWidth(string([]byte{byte(code)}), f.core, 1000), widthFromStd14
	}
	return 0, widthNone
}

// coreCodeMapped reports whether pdfcpu's WinAnsi core metrics carry a glyph for code, rather than
// substituting 1000 — see the file comment for why Courier can tell.
func coreCodeMapped(code int) bool {
	return mdpdf.CoreWidth(string([]byte{byte(code)}), "Courier", 1000) != 1000
}

// advanceAt converts a glyph-space advance to user space at a font size that need not be whole —
// `mdpdf.CoreWidth` takes an int size, and a reader holding 9.5 must not truncate it.
func advanceAt(glyph, size float64) float64 {
	return glyph / 1000 * size
}

// readW parses a CID font's `/W`: `c [w1 w2 …]` and `cfirst clast w`, in any mix. A malformed entry
// stops the parse; CIDs after it fall to `/DW`, whose source says so.
func readW(xt *model.XRefTable, o types.Object) map[int]float64 {
	out := map[int]float64{}
	arr, err := xt.DereferenceArray(o)
	if err != nil || arr == nil {
		return out
	}
	for i := 0; i+1 < len(arr); {
		first, ok := pdfNumber(xt, arr[i])
		if !ok || first < 0 || first > maxCID {
			return out
		}
		next, derr := xt.Dereference(arr[i+1])
		if derr != nil {
			return out
		}
		if ws, isArr := next.(types.Array); isArr {
			for j, wo := range ws {
				cid := int(first) + j
				if cid > maxCID {
					break
				}
				if v, vok := pdfNumber(xt, wo); vok {
					out[cid] = v
				}
			}
			i += 2
			continue
		}
		last, lok := pdfNumber(xt, next)
		if !lok || i+2 >= len(arr) {
			return out
		}
		w, wok := pdfNumber(xt, arr[i+2])
		if !wok || last < first {
			return out
		}
		for cid := int(first); cid <= int(math.Min(last, maxCID)); cid++ {
			out[cid] = w
		}
		i += 3
	}
	return out
}

// pdfNumber resolves an integer or real, direct or indirect.
func pdfNumber(xt *model.XRefTable, o types.Object) (float64, bool) {
	v, err := xt.Dereference(o)
	if err != nil || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case types.Integer:
		return float64(n.Value()), true
	case types.Float:
		return n.Value(), true
	}
	return 0, false
}
