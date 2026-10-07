package pdfops

import (
	"math"
	"strconv"
	"strings"
)

// The pieces of a painted path (ADR-088).
//
// Reflow asks what a page draws so that text can keep clear of it, and one box per painted path answers that
// (`pageMark`). Finding where a form wants to be filled in asks a different question — where each ruled line and each
// box is — and a table ruled by a single path of forty segments is one mark. So the page map's reader keeps the pieces.

// pageShape is one piece of a painted path, boxed in user space [llx lly urx ury].
//
// A rect is an `re`, or a closed subpath of four axis-aligned segments; a segment is one straight `l`. Only
// axis-aligned pieces are kept: a slanted line and a curve are not where a form is filled in, and their box says
// nothing about where they are.
type pageShape struct {
	box     [4]float64
	rect    bool    // a rectangle; false is a single straight segment
	stroked bool    // its outline is drawn
	filled  bool    // its inside is painted
	lw      float64 // the stroke's width in user space; 0 when not stroked
	// white is true when every paint the piece was drawn with is white in a device space: a white fill under a form's
	// fields, a white rule erasing part of another. Unknown colours (a pattern, a separation) are NOT white — a piece
	// is hidden only when it is known to be.
	white bool
	// whiteFill is a rectangle whose INSIDE is painted white, whatever its outline is painted with: the ground a form
	// lays under a blank (ADR-096). A white piece with whiteFill is that ground and nothing else.
	whiteFill bool
}

// shapePath gathers the current path's pieces between the operator that starts it and the one that paints it.
type shapePath struct {
	pieces []pageShape
	// sub is the current subpath's corners in user space, kept while every segment so far is straight.
	sub      [][2]float64
	curved   bool
	hasPoint bool
}

func (p *shapePath) endSub(closed bool) {
	if len(p.sub) >= 2 && !p.curved {
		pts := p.sub
		if closed {
			pts = append(pts, pts[0])
		}
		// Four axis-aligned sides that return to the start are a rectangle, however they were drawn.
		if len(pts) == 5 && pts[0] == pts[4] && axisAligned(pts) {
			p.pieces = append(p.pieces, pageShape{box: boxOfPoints(pts), rect: true})
		} else {
			for i := 0; i+1 < len(pts); i++ {
				a, b := pts[i], pts[i+1]
				if a == b || (a[0] != b[0] && a[1] != b[1]) {
					continue // a point, or a slanted line
				}
				p.pieces = append(p.pieces, pageShape{box: boxOfPoints([][2]float64{a, b})})
			}
		}
	}
	p.sub, p.curved = nil, false
}

// to is `m` (line false) or `l` (line true).
func (p *shapePath) to(m runMatrix, x, y float64, line bool) {
	ux, uy := m.apply(x, y)
	pt := [2]float64{snap(ux), snap(uy)}
	if !line {
		p.endSub(false)
		p.sub, p.hasPoint = [][2]float64{pt}, true
		return
	}
	if !p.hasPoint {
		return // an `l` with no current point draws nothing
	}
	p.sub = append(p.sub, pt)
}

func (p *shapePath) rect(m runMatrix, x, y, w, h float64) {
	p.endSub(false)
	// A rectangle under a rotating or skewing matrix is not axis-aligned in user space; its box would lie.
	if m[1] != 0 || m[2] != 0 {
		if m[0] != 0 || m[3] != 0 {
			return
		}
	}
	x0, y0 := m.apply(x, y)
	x1, y1 := m.apply(x+w, y+h)
	p.pieces = append(p.pieces, pageShape{box: [4]float64{math.Min(x0, x1), math.Min(y0, y1), math.Max(x0, x1), math.Max(y0, y1)}, rect: true})
	p.hasPoint = true
}

// snap rounds away the float noise a matrix leaves, so two ends of one horizontal line compare equal.
func snap(v float64) float64 { return math.Round(v*1000) / 1000 }

func axisAligned(pts [][2]float64) bool {
	for i := 0; i+1 < len(pts); i++ {
		if pts[i][0] != pts[i+1][0] && pts[i][1] != pts[i+1][1] {
			return false
		}
	}
	return true
}

func boxOfPoints(pts [][2]float64) [4]float64 {
	b := [4]float64{pts[0][0], pts[0][1], pts[0][0], pts[0][1]}
	for _, q := range pts[1:] {
		b = [4]float64{math.Min(b[0], q[0]), math.Min(b[1], q[1]), math.Max(b[2], q[0]), math.Max(b[3], q[1])}
	}
	return b
}

// keepPieces records the current path's pieces as painted, and starts a new path. closes is a closing paint (`s`, `b`).
func (w *runWalker) keepPieces(p *shapePath, gs runGState, stroked, filled, closes bool) {
	p.endSub(closes)
	lw := 0.0
	if stroked {
		m := gs.ctm
		lw = math.Abs(gs.lw) * math.Sqrt(math.Max(m[0]*m[0]+m[1]*m[1], m[2]*m[2]+m[3]*m[3]))
	}
	white := (!stroked || isWhitePaint(gs.stroke)) && (!filled || isWhitePaint(gs.fill))
	for _, s := range p.pieces {
		// A lone segment has no inside: a fill paints nothing of it.
		if !s.rect && !stroked {
			continue
		}
		s.stroked, s.filled, s.lw, s.white = stroked, filled && s.rect, lw, white
		s.whiteFill = s.filled && isWhitePaint(gs.fill)
		w.shapes = append(w.shapes, s)
	}
	*p = shapePath{}
}

// isWhitePaint says whether op — a device-space colour operator as the walk keeps it (`1 g`, `1 1 1 rg`, `0 0 0 0 k`)
// — paints white. "" is a colour the walk could not carry, and the initial colour is black: neither is white.
func isWhitePaint(op string) bool {
	switch op {
	case "1 g", "1 G", "1 1 1 rg", "1 1 1 RG", "0 0 0 0 k", "0 0 0 0 K":
		return true
	}
	return false
}

// faintFrom is how light a colour must be for print in it to be FAINT, 0 black to 1 white (ADR-099). The hint a form
// prints where an entry goes — "MM", "DD", "YYYY" in the IRS 1040's date blanks — is drawn at 0.753, where its
// labels are black.
const faintFrom = 0.7

// paintLightness reads op — a device-space colour operator as the walk keeps it (`0.75 g`, `1 0 0 rg`, `0 0 0 0.2 k`,
// `/DeviceRGB cs 0.5 0.5 0.5 sc`) — as how light the colour is, 0 black to 1 white. Not ok for "" (a colour the walk
// could not carry) or for anything but one, three or four numbers before the operator.
func paintLightness(op string) (float64, bool) {
	f := strings.Fields(op)
	var v []float64
	for i := len(f) - 2; i >= 0; i-- { // the numbers just before the operator, last first
		x, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			break
		}
		v = append(v, math.Max(0, math.Min(1, x)))
	}
	switch len(v) {
	case 1:
		return v[0], true
	case 3: // v is b, g, r
		return 0.299*v[2] + 0.587*v[1] + 0.114*v[0], true
	case 4: // v is k, y, m, c
		k := 1 - v[0]
		return (0.299*(1-v[3]) + 0.587*(1-v[2]) + 0.114*(1-v[1])) * k, true
	}
	return 0, false
}
