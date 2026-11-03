package pdfops

import "math"

// What an export may read of a page (PLAN-import-export D30, R2, X6): only text a reader could see. A run is dropped,
// and COUNTED by cause, when it is
//
//   - off the page (its box does not meet the visible box),
//   - in optional content that is OFF in the default configuration (`ocState`),
//   - covered by a LATER opaque mark: a filled rectangle or an upright, unmasked image drawn after it, unclipped and with
//     no graphics-state dictionary applied, whose box holds the run wholly,
//   - hidden text (render mode 3, an OCR layer) that visible text covers most of.
//
// A mark that might cover it but is translucent, clipped or set under a state dictionary keeps the text and counts it
// `MaybeCovered`: the safe error here is to say so, and the cover test's own budget is `CoverUntested`, never
// `Covered` (`paintedOver` fails safe to "covered", which for text is the wrong way round).
//
// **Declared:** one covering mark must hold the run on its own (two rectangles that jointly cover it do not); the run's
// box is its baseline to 0.7 of its size, not its glyphs; annotation text and `/Redact` rectangles are not read (an
// annotation's appearance is not walked by the page reader at all).

// ExportVisibility counts what the export left out of a page, by cause.
type ExportVisibility struct {
	OffPage       int
	Covered       int
	MaybeCovered  int
	HiddenLayer   int
	HiddenText    int
	CoverUntested int
	// UnreadGlyphs counts glyphs of the runs kept that have no text the document lets nib read (`PageMap.Unread`): the
	// page prints more than the export holds.
	UnreadGlyphs int
}

// Add sums o into v.
func (v *ExportVisibility) Add(o ExportVisibility) {
	v.OffPage += o.OffPage
	v.Covered += o.Covered
	v.MaybeCovered += o.MaybeCovered
	v.HiddenLayer += o.HiddenLayer
	v.HiddenText += o.HiddenText
	v.CoverUntested += o.CoverUntested
	v.UnreadGlyphs += o.UnreadGlyphs
}

// coverBudget bounds the comparisons one page's cover test makes. A variable so a test can lower it.
var coverBudget = 200_000_000

// inkBox is the part of a run's box the cover test asks about: its baseline to 0.7 of its size.
func inkBox(r textRun) [4]float64 {
	x0, x1 := r.x, r.x+r.width
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	return [4]float64{x0, r.y, x1, r.y + 0.7*r.size}
}

func boxHolds(outer, in [4]float64) bool {
	const tol = 0.5
	return in[0] >= outer[0]-tol && in[1] >= outer[1]-tol && in[2] <= outer[2]+tol && in[3] <= outer[3]+tol
}

func boxesMeet(a, b [4]float64) bool {
	return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]
}

func boxArea(b [4]float64) float64 {
	return math.Max(0, b[2]-b[0]) * math.Max(0, b[3]-b[1])
}

func overlapArea(a, b [4]float64) float64 {
	return boxArea([4]float64{math.Max(a[0], b[0]), math.Max(a[1], b[1]), math.Min(a[2], b[2]), math.Min(a[3], b[3])})
}

// opaqueImage says an image hides what is below it where it is drawn.
func opaqueImage(im drawnImage) bool {
	switch im.cause {
	case PlaceholderStencil, PlaceholderMasked, PlaceholderSoftMask:
		return false
	}
	return im.upright && !im.translucent && !im.clipped && !im.layerOff
}

// visibleRuns applies the rules above to pr in place: its runs, shapes and images are what a reader would see.
func visibleRuns(pr *pageRuns, vis [4]float64) ExportVisibility {
	var v ExportVisibility
	type cover struct {
		seq int
		box [4]float64
	}
	type maybe struct {
		seq int
		box [4]float64
	}
	var covers []cover
	var maybes []maybe
	var shapes []pageShape
	for _, s := range pr.shapes {
		if s.layerOff {
			continue
		}
		shapes = append(shapes, s)
		if !s.rect || !s.filled {
			continue
		}
		if s.translucent || s.clipped {
			maybes = append(maybes, maybe{s.seq, s.box})
		} else {
			covers = append(covers, cover{s.seq, s.box})
		}
	}
	var images []drawnImage
	for _, im := range pr.images {
		if im.layerOff {
			continue
		}
		images = append(images, im)
		if opaqueImage(im) {
			covers = append(covers, cover{im.seq, im.box})
		} else if im.upright && im.cause != PlaceholderStencil {
			maybes = append(maybes, maybe{im.seq, im.box})
		}
	}
	pr.shapes, pr.images = shapes, images

	work := len(pr.runs) * (len(covers) + len(maybes))
	testCover := work <= coverBudget
	var kept []textRun
	for _, r := range pr.runs {
		switch {
		case r.layerOff:
			v.HiddenLayer++
			continue
		case !boxesMeet(runBox(r), vis):
			v.OffPage++
			continue
		}
		if r.state.tr != 3 && testCover {
			ink, covered := inkBox(r), false
			for _, c := range covers {
				if c.seq > r.seq && boxHolds(c.box, ink) {
					covered = true
					break
				}
			}
			if covered {
				v.Covered++
				continue
			}
			for _, m := range maybes {
				if m.seq > r.seq && boxHolds(m.box, ink) {
					v.MaybeCovered++
					break
				}
			}
		} else if r.state.tr != 3 {
			v.CoverUntested++
		}
		kept = append(kept, r)
	}
	// Hidden (OCR) text is read unless visible text covers most of its box.
	var visible [][4]float64
	for _, r := range kept {
		if r.state.tr != 3 {
			visible = append(visible, runBox(r))
		}
	}
	out := kept[:0:0]
	for _, r := range kept {
		if r.state.tr == 3 && len(visible) > 0 {
			b, got := runBox(r), 0.0
			for _, vb := range visible {
				got += overlapArea(b, vb)
			}
			if a := boxArea(b); a > 0 && got/a > 0.5 {
				v.HiddenText++
				continue
			}
		}
		out = append(out, r)
	}
	pr.runs = out
	for _, r := range out {
		if !r.decoded {
			for _, g := range r.glyphs {
				if !g.decoded {
					v.UnreadGlyphs++
				}
			}
		}
	}
	return v
}
