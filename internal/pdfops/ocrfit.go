package pdfops

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"nib/internal/contentstream"
	"nib/internal/pdfread"

	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Fitting a stamped OCR word to the scanned word it stands for — ADR-092.
//
// # What was wrong
//
// `StampTextLayer` set each word at a font size equal to its box's HEIGHT. The box is the word's ink, so that size is
// an x-height or a cap height and not an em, and the word came out narrow: a median 0.72 of the scanned width
// (ADR-091), read the same by nib's own map and by poppler. Anything that boxes text by its glyphs — a search-redaction
// in another program, a selection, a Find highlight — stopped short of the end of the word.
//
// # Why it is a matrix rewritten after the stamp
//
// pdfcpu's watermark cannot do it. Its placement matrix is unit scale by construction (`CalcTransformMatrix(1, 1, …)`)
// and its font size is an int at every step, so a uniform scale is quantised to whole points and there is no
// horizontal one. What it writes for each word is
//
//	/Artifact <</Subtype /Watermark /Type /Pagination >>BDC q a b c d e f cm /GS0 gs /Fm0 Do Q EMC
//
// and those six numbers are the one continuous lever: the fit is multiplied in ahead of them, in the form's own
// space, so whatever pdfcpu and `stampInPlace` decided about where the word goes — a turned page included — stands.
//
// # The fit
//
// Across, the word's ADVANCE spans the box: that is the extent every reader computes a glyph run from. (Fitting the
// ink's left and right edges instead leaves a script whose marks are drawn outside their advance short — measured,
// Gurmukhi, 10pt.) Up and down, the glyphs' own ink — as this face draws this text — is put on the box's top and
// bottom, which is what gives the word its true size and its baseline.

// wordFit is A in A·M: what is multiplied into the matrix pdfcpu placed a word's form with.
type wordFit struct {
	sx, sy, oy float64
}

var asStamped = wordFit{sx: 1, sy: 1}

// stampedWord is what is done to one word after pdfcpu stamps it: its fit, and whether it was set in the reverse of
// reading order and so is to be marked as that (ADR-097, ocrorder.go).
type stampedWord struct {
	fit      wordFit
	reversed bool
}

var ocrFaces sync.Map // font name -> *sfnt.Font, or nil for a face that would not read

// ocrFace is the face a word is stamped in, read for its glyphs' boxes. pdfcpu holds the same file but keeps only
// advance widths.
func ocrFace(name string) *sfnt.Font {
	if f, ok := ocrFaces.Load(name); ok {
		return f.(*sfnt.Font)
	}
	path, ok := ocrFontFiles[name]
	if !ok {
		path = "fonts/" + name + ".ttf"
	}
	var face *sfnt.Font
	if b, err := ocrFontFS.ReadFile(path); err == nil {
		face, _ = sfnt.Parse(b)
	}
	ocrFaces.Store(name, face)
	return face
}

// wordInk measures text as face draws it, glyph after glyph and unshaped, in ems: how far it advances, and the
// lowest and highest its ink reaches about the baseline. A rune the face has no glyph for is passed over, as pdfcpu
// passes over it when it writes the run.
func wordInk(fontName string, face *sfnt.Font, text string) (advance, bottom, top float64, ok bool) {
	if face == nil {
		return 0, 0, 0, false
	}
	var buf sfnt.Buffer
	upem := float64(face.UnitsPerEm())
	ppem := fixed.I(int(face.UnitsPerEm()))
	bottom, top = math.Inf(1), math.Inf(-1)
	for _, r := range text {
		gi, err := face.GlyphIndex(&buf, r)
		if err != nil || gi == 0 {
			continue
		}
		bounds, _, err := face.GlyphBounds(&buf, gi, ppem, font.HintingNone)
		if err != nil {
			continue
		}
		// The advance is pdfcpu's own figure for the glyph — the whole number of thousandths it writes into the
		// embedded font's widths — because that is what a reader will add up. The face's exact advance differs by
		// the rounding, which a wide box multiplies.
		advance += float64(pdffont.CharWidth(fontName, r)) / 1000
		if bounds.Max.Y > bounds.Min.Y { // sfnt's y runs down
			bottom, top = math.Min(bottom, -float64(bounds.Max.Y)/64), math.Max(top, -float64(bounds.Min.Y)/64)
		}
	}
	return advance, bottom / upem, top / upem, advance > 0 && top > bottom
}

// fitWord is the size to stamp a word at and the fit that puts it on rect. A word that cannot be fitted — no ink in
// its face, an empty box — comes back at its box's height and unfitted, which is how every word was stamped before.
func fitWord(fontName, text string, rect [4]float64) (pts int, fit wordFit, fitted bool) {
	w, h := rect[2]-rect[0], rect[3]-rect[1]
	pts = max(int(h), 4)
	advance, bottom, top, ok := wordInk(fontName, ocrFace(fontName), text)
	if !ok || !(w > 0) || !(h > 0) {
		return pts, asStamped, false
	}
	em := h / (top - bottom)
	if !(em > 0) || math.IsInf(em, 0) || em > 14400 {
		return pts, asStamped, false
	}
	pts = max(int(math.Round(em)), 4)
	// pdfcpu sets the baseline this far above the form's origin (`model.CalcBoundingBox`), and the run at x = 0.
	base := math.Ceil(pdffont.Descent(fontName, pts))
	fit.sy = em / float64(pts)
	fit.sx = w / (advance * float64(pts))
	fit.oy = -fit.sy * (base + bottom*float64(pts))
	return pts, fit, true
}

// fitStampedWords multiplies each word's fit into the matrix pdfcpu placed it with, and marks the form of each word
// set in reverse. pre is how many watermark markers each page drew before the stamp; fits is each page's stamped
// words, in the order they were stamped.
//
// **The pairing is by position, checked by count — the rule `tagOCRPage` states, for the same reason.** pdfcpu stamps
// a page's words in slice order and appends each after the page's content, so the page's own markers are the prefix
// and the words the suffix. pdfcpu hands back nothing that names a word's form (`addPageWatermark` takes the
// watermark by value), so there is no identity to pair on. A page whose counts disagree — pdfcpu skips a content
// stream it cannot decode, silently — is left as stamped, and its words are returned as unpaired. (A reversed word
// on such a page is then unmarked, and read as any print right-to-left word is: in the order it is set.)
func fitStampedWords(ctx *model.Context, pre map[int]int, fits map[int][]stampedWord) (unpaired int, err error) {
	for _, pg := range pdfread.Pages(ctx) {
		fs := fits[pg.Nr]
		if len(fs) == 0 {
			continue
		}
		if pg.Err != nil || pg.Dict == nil {
			unpaired += len(fs)
			continue
		}
		src, cerr := pdfread.PageContent(ctx, pg.Dict, pg.Nr)
		if cerr != nil {
			unpaired += len(fs)
			continue
		}
		markers := watermarkArtifactSpans(src)
		if n, ok := pre[pg.Nr]; !ok || n > len(markers) || len(markers)-n != len(fs) {
			unpaired += len(fs)
			continue
		}
		markers = markers[pre[pg.Nr]:]
		toks := contentstream.Tokenize(src)
		edit := contentstream.NewEdit(src)
		ti, changed := 0, false
		for i, m := range markers {
			if fs[i].fit == asStamped && !fs[i].reversed {
				continue
			}
			for ti < len(toks) && toks[ti].Start < m.end {
				ti++
			}
			nums, after, ok := placementAfter(src, toks[ti:])
			if !ok {
				return 0, fmt.Errorf("pdfops: page %d: the text layer's word %d is not placed as pdfcpu places a stamp", pg.Nr, i+1)
			}
			var mx [6]float64
			for k, n := range nums {
				if mx[k], err = strconv.ParseFloat(string(n.Bytes(src)), 64); err != nil {
					return 0, err
				}
			}
			if fs[i].reversed {
				name, ok := formDrawnAfter(src, toks[ti+after:])
				if !ok || pg.Attrs == nil {
					return 0, fmt.Errorf("pdfops: page %d: the text layer's word %d draws no form to mark", pg.Nr, i+1)
				}
				if err := markReversedChars(ctx, pg.Attrs.Resources, name); err != nil {
					return 0, err
				}
			}
			f := fs[i].fit
			if f == asStamped {
				continue
			}
			edit.Replace(nums[0].Start, nums[5].End, []byte(fmt.Sprintf("%.5f %.5f %.5f %.5f %.5f %.5f",
				f.sx*mx[0], f.sx*mx[1], f.sy*mx[2], f.sy*mx[3], f.oy*mx[2]+mx[4], f.oy*mx[3]+mx[5])))
			changed = true
		}
		if !changed {
			continue
		}
		edited, eerr := edit.Apply()
		if eerr != nil {
			return 0, eerr
		}
		if err := setPageContent(ctx, pg.Dict, edited); err != nil {
			return 0, err
		}
	}
	return unpaired, nil
}

// placementAfter reads `q a b c d e f cm` — what pdfcpu writes straight after a stamp's marker — and returns the six
// operands, and how many of toks that took.
func placementAfter(src []byte, toks []contentstream.Token) ([]contentstream.Token, int, bool) {
	var nums []contentstream.Token
	opened := false
	for n, tk := range toks {
		if tk.Kind == contentstream.Whitespace {
			continue
		}
		s := string(tk.Bytes(src))
		switch {
		case !opened:
			if tk.Kind != contentstream.Operator || s != "q" {
				return nil, 0, false
			}
			opened = true
		case tk.Kind == contentstream.Operand && len(nums) < 6:
			nums = append(nums, tk)
		default:
			return nums, n + 1, tk.Kind == contentstream.Operator && s == "cm" && len(nums) == 6
		}
	}
	return nil, 0, false
}

// markersBeforeStamp counts the watermark markers each page draws, in the context about to be stamped.
func markersBeforeStamp(ctx *model.Context, fits map[int][]stampedWord) map[int]int {
	pre := map[int]int{}
	for _, pg := range pdfread.Pages(ctx) {
		if len(fits[pg.Nr]) == 0 || pg.Err != nil || pg.Dict == nil {
			continue
		}
		src, err := pdfread.PageContent(ctx, pg.Dict, pg.Nr)
		switch err {
		case nil:
			pre[pg.Nr] = len(watermarkArtifactSpans(src))
		case model.ErrNoContent:
			pre[pg.Nr] = 0
		}
	}
	return pre
}
