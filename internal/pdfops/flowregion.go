package pdfops

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// What lies below a paragraph — `PLAN-text-reflow.md` P07.S01.
//
// A paragraph that grows pushes the paragraphs below it down, into whatever free space lies under them. Before anything
// moves, reflow has to know three things: which paragraphs move with it (the REGION), how far they can go (the ROOM),
// and what else is drawn where they will be (the BAND's marks). Text is not the only thing on a page — a rule under a
// heading, a logo, a filled box behind a table, a cover nib's own edit baked — and none of those move with the text, so
// each is boxed and reported rather than drawn over.

// markKind names what drew a non-text mark.
type markKind string

const (
	markPath        markKind = "path"         // a filled or stroked path
	markImage       markKind = "image"        // an image XObject drawn with `Do`
	markInlineImage markKind = "inline-image" // `BI … EI`
	markShading     markKind = "shading"      // `sh`: paints its clip, which this reader does not track — unbounded
	markText        markKind = "text"         // text the region does not own: an artifact, another column, a blank run
	markForm        markKind = "form"         // a form the walk did not enter, boxed by its /BBox
)

// pageMark is one mark a page draws, boxed in user space [llx lly urx ury].
type pageMark struct {
	kind markKind
	box  [4]float64
}

// markBox accumulates the box of the points added to it, in user space.
type markBox struct {
	set bool
	box [4]float64
}

// add adds points (x0, y0, x1, y1, …) given in the space m maps to user space.
func (b *markBox) add(m runMatrix, xy ...float64) {
	for i := 0; i+1 < len(xy); i += 2 {
		x, y := m.apply(xy[i], xy[i+1])
		if !b.set {
			b.box, b.set = [4]float64{x, y, x, y}, true
			continue
		}
		b.box = [4]float64{math.Min(b.box[0], x), math.Min(b.box[1], y), math.Max(b.box[2], x), math.Max(b.box[3], y)}
	}
}

// markPath records a painted path. A stroke reaches half its line width past the path, scaled by the CTM's larger axis.
// **Declared gap**: a miter join can reach `miterlimit × w/2` past its corner; only thick strokes with sharp joins reach
// past this box, and a stroke in the band already refuses.
func (w *runWalker) markPath(p markBox, gs runGState, stroked bool) {
	if !w.keepMarks || !p.set {
		return
	}
	b := p.box
	if stroked {
		m := gs.ctm
		half := math.Abs(gs.lw) / 2 * math.Sqrt(math.Max(m[0]*m[0]+m[1]*m[1], m[2]*m[2]+m[3]*m[3]))
		if gs.lw == 0 {
			half = 0.5 // a zero-width line is the thinnest the device can draw, a pixel or so
		}
		b = [4]float64{b[0] - half, b[1] - half, b[2] + half, b[3] + half}
	}
	w.marks = append(w.marks, pageMark{kind: markPath, box: b})
}

// markImage records an image: it fills the unit square under the CTM (ISO 32000-1 §8.3.4).
func (w *runWalker) markImage(kind markKind, gs runGState) {
	if !w.keepMarks {
		return
	}
	var b markBox
	b.add(gs.ctm, 0, 0, 1, 0, 0, 1, 1, 1)
	w.marks = append(w.marks, pageMark{kind: kind, box: b.box})
}

// markShading records `sh`, which paints the whole of the current clip. The clip is not tracked, so its box is every
// point there is: it meets every band, and a flow over it refuses rather than guessing where it is.
func (w *runWalker) markShading() {
	if w.keepMarks {
		inf := math.Inf(1)
		w.marks = append(w.marks, pageMark{kind: markShading, box: [4]float64{-inf, -inf, inf, inf}})
	}
}

// markForm records a form the walk did not enter — undecodable, nested past `maxFormDepth`, or drawing itself — as the
// box its /BBox covers under its /Matrix and the CTM: nothing it draws was read, and none of it reaches outside that box.
// Without one there is no bound on where it draws, so the box is every point there is.
func (w *runWalker) markForm(sd *types.StreamDict, gs runGState) {
	if !w.keepMarks {
		return
	}
	nums := func(key string, n int) ([]float64, bool) {
		arr, err := w.xt.DereferenceArray(sd.Dict[key])
		if err != nil || len(arr) != n {
			return nil, false
		}
		out := make([]float64, n)
		for i, o := range arr {
			v, ok := pdfNumber(w.xt, o)
			if !ok {
				return nil, false
			}
			out[i] = v
		}
		return out, true
	}
	bb, ok := nums("BBox", 4)
	if !ok {
		inf := math.Inf(1)
		w.marks = append(w.marks, pageMark{kind: markForm, box: [4]float64{-inf, -inf, inf, inf}})
		return
	}
	m := runIdentity
	if v, ok := nums("Matrix", 6); ok {
		m = runMatrix{v[0], v[1], v[2], v[3], v[4], v[5]}
	}
	var b markBox
	b.add(m.mul(gs.ctm), bb[0], bb[1], bb[2], bb[1], bb[0], bb[3], bb[2], bb[3])
	w.marks = append(w.marks, pageMark{kind: markForm, box: b.box})
}

// drawsImage says whether name, in res, is an image XObject.
func (w *runWalker) drawsImage(res types.Dict, name string) bool {
	if !w.keepMarks || res == nil {
		return false
	}
	xobjs, err := w.xt.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return false
	}
	obj, ok := xobjs[name]
	if !ok {
		return false
	}
	sd, _, err := w.xt.DereferenceStreamDict(obj)
	if err != nil || sd == nil {
		return false
	}
	st := sd.Dict.NameEntry("Subtype")
	return st != nil && *st == "Image"
}

// flowRegion is what lies below paragraph pi of a page.
type flowRegion struct {
	// paragraphs are the indices, into the layout, of the paragraphs that move when pi grows — those below it in its
	// column, top to bottom, up to the first gap clearly wider than a paragraph break.
	paragraphs []int
	// x0, x1 is the column's horizontal extent: what the band covers across the page.
	x0, x1 float64
	// bottom is the lowest point the region draws — pi's own when the region is empty — and floor the lowest it may
	// reach: room is bottom − floor, never negative.
	bottom, floor, room float64
	// bound says what set the floor: `roomBelowContent` or `roomBelowMargin`.
	bound string
	// step is the column's paragraph step — last baseline to the next paragraph's first — its median where it has one,
	// else the step from pi to the first paragraph below it, else 0: how far a block pushed onto the next page stands from
	// what it pushes (P07.S06).
	step float64
	// band is the column's extent from pi's bottom down to the floor: where the region's text is, and will be.
	band [4]float64
	// marks are the marks meeting the band — the column's extent from pi's bottom down to the floor — that the region
	// does not own. None of them moves with the text.
	marks []pageMark
}

const (
	roomBelowContent = "content-below" // something is drawn below the region, in its column
	roomBelowMargin  = "page-margin"   // nothing is: the page's bottom margin, mirroring its top one
)

// regionGapEm is the least gap between two paragraphs, in ems, that still ends a region however the column spaces its
// paragraphs — below it, a gap is a paragraph break.
const regionGapEm = 2.5

// regionGapRatio is how much wider than the column's usual paragraph step a step must be to end the region.
const regionGapRatio = 1.5

// descentEm and ascentEm bound a line's glyphs about its baseline, in ems of its size: a letter's descender reaches a
// quarter em below it, a capital or ascender at most one em above.
const (
	descentEm = 0.25
	ascentEm  = 1.0
)

func (p textParagraph) top() float64    { return paragraphBox(p)[3] }
func (p textParagraph) bottom() float64 { return paragraphBox(p)[1] }

// runBox is a run's box: its advance across, a descender below its baseline to an ascender above.
func runBox(r textRun) [4]float64 {
	x0, x1 := r.x, r.x+r.width
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	return [4]float64{x0, r.y - descentEm*r.size, x1, r.y + ascentEm*r.size}
}

// paragraphBox is the box a paragraph's lines occupy: their extent across, and from the lowest descender to the highest
// ascender, taken as the size below and above each baseline — the glyph boxes are not read, so this is the lines' reach,
// not their ink. `TestTheReflowDoor` holds a rewrite inside it too.
func paragraphBox(p textParagraph) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, l := range p.lines {
		b[0], b[2] = math.Min(b[0], l.x0), math.Max(b[2], l.x1)
		b[1], b[3] = math.Min(b[1], l.y-descentEm*l.size), math.Max(b[3], l.y+ascentEm*l.size)
	}
	return b
}

// meets says whether boxes a and b overlap in both axes. Touching edges do not meet.
func meets(a, b [4]float64) bool {
	return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]
}

// touches says whether boxes a and b share any point, edges included — the test for an anchor, which may be a point (a
// destination's top, a flag) and has no area to overlap with.
func touches(a, b [4]float64) bool {
	return a[0] <= b[2] && b[0] <= a[2] && a[1] <= b[3] && b[1] <= a[3]
}

// covers says whether box a contains box b.
func covers(a, b [4]float64) bool {
	return a[0] <= b[0] && a[1] <= b[1] && a[2] >= b[2] && a[3] >= b[3]
}

// visibleBoxOf is the page's crop box, else its media box, or zeros.
func visibleBoxOf(pg pdfread.Page) [4]float64 {
	if pg.Err == nil && pg.Attrs != nil && pg.Attrs.CropBox != nil {
		cb := pg.Attrs.CropBox
		return [4]float64{cb.LL.X, cb.LL.Y, cb.UR.X, cb.UR.Y}
	}
	return mediaBoxOf(pg)
}

// regionOf answers what lies below paragraph pi of l, on a page whose visible box is page.
//
// The FLOOR: when nothing is drawn below the region in its column, the region may grow to the page's bottom margin, which
// is taken to mirror its top one — the distance from the top of the page to the highest thing it draws. One page's own
// geometry, read once: a document-wide margin would read every page on a request path. When something IS drawn below — a
// footer, a page number, the next section past a wide gap — the floor is one em above it, the region's last line's em, if
// that is higher than the margin floor; a footer inside the margin leaves the margin binding.
func regionOf(l pageLayout, pi int, page [4]float64) flowRegion {
	p := l.paragraphs[pi]
	var col []int // the column's paragraphs, top to bottom — the layout's order within a column
	for i, q := range l.paragraphs {
		if q.column == p.column {
			col = append(col, i)
		}
	}
	var r flowRegion
	r.x0, r.x1 = math.Inf(1), math.Inf(-1)
	for _, i := range col {
		b := paragraphBox(l.paragraphs[i])
		r.x0, r.x1 = math.Min(r.x0, b[0]), math.Max(r.x1, b[2])
	}
	// A step is from one paragraph's last baseline to the next one's first: the column's usual one is their median.
	var steps []float64
	for k := 1; k < len(col); k++ {
		a, b := l.paragraphs[col[k-1]], l.paragraphs[col[k]]
		steps = append(steps, a.lines[len(a.lines)-1].y-b.lines[0].y)
	}
	usual := 0.0
	if len(steps) >= 2 {
		usual = median(steps)
	}
	r.step = usual
	pos := sort.SearchInts(col, pi)
	owned := map[int]bool{pi: true}
	last := p
	for k := pos + 1; k < len(col); k++ {
		q := l.paragraphs[col[k]]
		em := math.Max(last.lines[len(last.lines)-1].size, q.lines[0].size)
		if step := last.lines[len(last.lines)-1].y - q.lines[0].y; step > math.Max(regionGapRatio*usual, regionGapEm*em) {
			break
		}
		if r.step == 0 {
			r.step = last.lines[len(last.lines)-1].y - q.lines[0].y
		}
		r.paragraphs = append(r.paragraphs, col[k])
		owned[col[k]] = true
		last = q
	}
	r.bottom = last.bottom()
	em := last.lines[len(last.lines)-1].size

	obstacles := obstaclesOf(l, owned)

	// The margin floor mirrors the page's top margin: the gap between the page's top and the highest thing it draws.
	top := math.Inf(-1)
	for i := range l.paragraphs {
		top = math.Max(top, l.paragraphs[i].top())
	}
	for _, o := range obstacles {
		if !math.IsInf(o.box[3], 1) {
			top = math.Max(top, o.box[3])
		}
	}
	r.floor, r.bound = r.bottom, roomBelowMargin // a page with no box to measure gives no room
	margin := math.Inf(-1)
	if page[3] > page[1] {
		r.floor = math.Max(page[1], page[1]+(page[3]-math.Min(top, page[3])))
		margin = r.floor
	}
	// below raises the floor to an em above what is drawn at top. What lies wholly inside the bottom margin — a page number
	// within an em of the margin's edge — keeps its em but leaves the MARGIN binding, as one further down does: it is a
	// footer, and nothing is read after it (/pending 787: a "1" at y 60 made a full page refuse to flow).
	below := func(top float64) {
		if top+em > r.floor {
			r.floor = top + em
			if top > margin {
				r.bound = roomBelowContent
			}
		}
	}
	// The highest obstacle below the region, across its column: an em above its top is the floor when that is higher.
	for _, o := range obstacles {
		if o.box[0] < r.x1 && r.x0 < o.box[2] && o.box[3] <= r.bottom && !math.IsInf(o.box[1], -1) {
			below(o.box[3])
		}
	}
	// A running footer (`runningLinesOf`) bounds the floor wherever it sits across the page: it is not in the column, but
	// it is the page's, and the body growing past it would put the page number among the text — the band below, which is
	// the page's width on a page of one column, would otherwise meet it and refuse a growth it has room for (/pending 787).
	for i, q := range l.paragraphs {
		if b := paragraphBox(q); q.running && !owned[i] && b[3] <= r.bottom {
			below(b[3])
		}
	}
	r.room = math.Max(0, r.bottom-r.floor)
	// The band: the column's extent, from pi's bottom down to the floor. A mark meeting it is drawn where the region
	// will be, and moves with nothing.
	// On a page of one column the band is the page's width: a drawing beside the region — a change bar, a margin rule —
	// is at the height that moves, and nothing moves it (P07.S04).
	bx0, bx1 := r.x0, r.x1
	if l.columns == 1 && page[2] > page[0] {
		bx0, bx1 = page[0], page[2]
	}
	band := [4]float64{bx0, math.Min(r.floor, r.bottom), bx1, p.bottom()}
	colBand := [4]float64{r.x0, band[1], r.x1, band[3]}
	r.band = band
	for _, o := range obstacles {
		// A mark covering the whole band is the text's BACKDROP — a page or cell fill — and the text stays over it
		// wherever in the band it moves, so it blocks nothing. Not a shading: its box is unbounded because its clip is
		// unknown, not because it covers everything. Covering the COLUMN's band is enough: the text never leaves it.
		if meets(o.box, band) && (o.kind == markShading || !covers(o.box, colBand)) {
			r.marks = append(r.marks, o)
		}
	}
	return r
}

// obstaclesOf is everything drawn on the page that the paragraphs in owned do not draw: other paragraphs, text no paragraph
// holds — an artifact, a stray run — and every non-text mark. A run showing only white space draws no ink and is none (a
// producer that sets each space as its own show is the commonest source: measured, 760 of 999 real-producer paragraphs
// would otherwise read as blocked).
func obstaclesOf(l pageLayout, owned map[int]bool) []pageMark {
	var out []pageMark
	for i, q := range l.paragraphs {
		if !owned[i] {
			out = append(out, pageMark{kind: markText, box: paragraphBox(q)})
		}
	}
	for _, t := range l.loose {
		if strings.TrimFunc(t.text, unicode.IsSpace) != "" {
			out = append(out, pageMark{kind: markText, box: runBox(t)})
		}
	}
	return append(out, l.marks...)
}

// deviceComponents is the number of components each device colour space takes (P07.S05).
var deviceComponents = map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}

// deviceColourOps is the number of components each device colour operator takes, and deviceSpaceOf the space that number
// names — package-level, not built per operator (P07 phase-close review: two map literals per colour operator, in the
// walk's inner loop).
var (
	deviceColourOps = map[string]int{"g": 1, "rg": 3, "k": 4, "G": 1, "RG": 3, "K": 4}
	deviceSpaceOf   = map[int]string{1: "DeviceGray", 3: "DeviceRGB", 4: "DeviceCMYK"}
	// spaceOp is the operator selecting a colour space, fill (true) or stroke; colourSetterOf the one setting a colour in it.
	spaceOp        = map[bool]string{true: "cs", false: "CS"}
	colourSetterOf = map[string]string{"cs": "sc", "CS": "SC"}
)

// initialDeviceColour is each device space's initial colour, black, as `cs` sets it.
var initialDeviceColour = map[string]string{"DeviceGray": "0", "DeviceRGB": "0 0 0", "DeviceCMYK": "0 0 0 1"}

// colourOp writes colour components and the operator that sets them.
func colourOp(v []float64, op string) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = num(x)
	}
	return strings.Join(parts, " ") + " " + op
}

// clipTo applies a pending clip: when a `W` marked the path, the clip becomes its intersection with the path's box. A path
// with no points clips everything away.
func clipTo(clip [4]float64, p markBox, pending bool) ([4]float64, bool) {
	if !pending {
		return clip, false
	}
	if !p.set {
		return [4]float64{0, 0, 0, 0}, false
	}
	return [4]float64{math.Max(clip[0], p.box[0]), math.Max(clip[1], p.box[1]), math.Min(clip[2], p.box[2]), math.Min(clip[3], p.box[3])}, false
}
