package pdfops

import (
	"math"
	"sort"
	"strings"
)

// A typed word takes the document's kerning — `PLAN-text-reflow.md` P08.S05. A word the paragraph already draws keeps its
// own kerns (`reflowParagraphIn`'s `known`); a word the user types has none of its own, and borrows, pair by pair, the
// kern the PAGE draws for that pair in that face and size — only where the page draws it one way.
//
// Kerning comes in two populations (measured over the real-producer corpus at P08's open): InDesign, Antenna House and
// Ghostscript draw a pair one way per face and size; Acrobat and Word position glyphs, and what reads as a kern is noise
// that differs at every occurrence — yet repeats by chance often enough that "drawn one way" alone lends it. The four
// thresholds below were chosen by leave-one-run-out prediction over that corpus — each run's pairs predicted from the
// table the page's OTHER runs lend, and compared with what the producer drew — and the census that holds them is
// `TestATypedWordBorrowsKerningOverTheCorpus`. At adoption (lent right / off by more than kernTolerance): InDesign 12,464
// / 19, Antenna House 21,160 / 0, Acrobat 1 / 0, Word 8 / 1. With support 2 and no table minimum, Acrobat lent 79 right
// and 47 wrong (S05's review, C2).

const (
	// kernTolerance is how far apart, in thousandths of an em (a `TJ` operand's unit), two draws of a pair may be and
	// still be one way: 0.002 em, below a device pixel at any reading size. Antenna House draws a pair within ~2 units at
	// one size; at 0.5 it lends a third of what it lends at 2, for no fewer misses.
	kernTolerance = 2.0
	// kernSupport is how many times the page must draw a pair before it lends it: twice is a coincidence on a positioning
	// producer's page often enough (Acrobat 79 / 47 at 2), three times is not (11 / 5, and nothing with the table below).
	kernSupport = 3
	// kernTable is how many pairs a face-and-size must draw one way, each kernSupport times, before it lends any: a
	// producer that kerns by table kerns dozens of pairs on a page, a positioning one repeats a handful by chance.
	kernTable = 5
	// kernNoise is the share of a face-and-size's kerned pairs that may be drawn more than one way before the face lends
	// nothing at all — the line between a producer that kerns by table and one that positions glyphs.
	kernNoise = 0.1
)

// kernKey is one glyph pair in one face at one size.
type kernKey struct {
	face *runFont
	size float64
	a, b string
}

// pageKerns is what a page lends a typed word: for each pair it draws one way, its kern in thousandths of an em.
type pageKerns map[kernKey]float64

// kernSize is the size a pair is keyed by — the run's size in user space, rounded past arithmetic noise.
func kernSize(size float64) float64 { return math.Round(size*1000) / 1000 }

// kernDraw is one kerned-or-not pair the page draws: its key, its kern in thousandths of an em, and which run drew it.
type kernDraw struct {
	key kernKey
	v   float64
	run int
}

// kernDraws reads every pair the page's paragraphs draw: adjacent glyphs inside one run, neither of them blank, with the
// `TJ` adjustment between them taken back to thousandths of an em — so a pair drawn under `Tz 110` or a stretched text
// matrix reads as the number the producer wrote. An adjustment as wide as a word space (`wordGapEm`) is not a kern but a
// producer that positions its words; a run in a form is drawn in another stream's state.
func kernDraws(layout pageLayout) []kernDraw {
	var out []kernDraw
	run := 0
	for _, p := range layout.paragraphs {
		for _, l := range p.lines {
			for _, r := range l.runs {
				run++
				den := r.state.tfSize * r.state.th * r.state.scale
				if r.face == nil || r.inForm || !(den != 0) || !finite(den) {
					continue
				}
				for gi := 1; gi < len(r.glyphs); gi++ {
					a, b := r.glyphs[gi-1], r.glyphs[gi]
					if strings.TrimSpace(a.text) == "" || strings.TrimSpace(b.text) == "" {
						continue
					}
					v := -b.kern / den * 1000
					if !finite(v) || math.Abs(v) >= wordGapEm*1000 {
						continue
					}
					out = append(out, kernDraw{kernKey{r.face, kernSize(r.size), string(a.code), string(b.code)}, v, run})
				}
			}
		}
	}
	return out
}

// kernsOf is what the page lends a typed word.
func kernsOf(layout pageLayout) pageKerns { return lentFrom(kernDraws(layout), -1) }

// lentFrom is what draws lend, leaving out run skip's (-1: none) — the census predicts each run from the others' table.
func lentFrom(draws []kernDraw, skip int) pageKerns {
	type faceSize struct {
		face *runFont
		size float64
	}
	drawn := map[kernKey][]float64{}
	for _, d := range draws {
		if d.run != skip {
			drawn[d.key] = append(drawn[d.key], d.v)
		}
	}
	oneWay := func(vs []float64) bool {
		lo, hi := vs[0], vs[0]
		for _, v := range vs {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return hi-lo <= kernTolerance
	}
	kerned, noisy, table := map[faceSize]int{}, map[faceSize]int{}, map[faceSize]int{}
	for k, vs := range drawn {
		if unkerned(vs) {
			continue
		}
		fs := faceSize{k.face, k.size}
		kerned[fs]++
		switch {
		case !oneWay(vs):
			noisy[fs]++
		case len(vs) >= kernSupport:
			table[fs]++
		}
	}
	out := pageKerns{}
	for k, vs := range drawn {
		fs := faceSize{k.face, k.size}
		if len(vs) < kernSupport || !oneWay(vs) || table[fs] < kernTable || float64(noisy[fs]) > kernNoise*float64(kerned[fs]) {
			continue
		}
		sorted := append([]float64(nil), vs...)
		sort.Float64s(sorted)
		if m := sorted[len(sorted)/2]; math.Abs(m) > 1e-9 {
			out[k] = m
		}
	}
	return out
}

// unkerned reports whether every draw of a pair is unkerned.
func unkerned(vs []float64) bool {
	for _, v := range vs {
		if math.Abs(v) > 1e-9 {
			return false
		}
	}
	return true
}

// lend is the kern before glyph b after glyph a, in face f at size, in thousandths of an em; 0 where the page lends none.
func (k pageKerns) lend(f *runFont, size float64, a, b []byte) float64 {
	return k[kernKey{f, kernSize(size), string(a), string(b)}]
}
