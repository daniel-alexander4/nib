package pdfops

import (
	"math"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// The page map (ADR-088): where a page's text, ruled lines, boxes and form fields are, read from what the page
// DRAWS rather than from a picture of it.
//
// Finding the blanks on a form and placing a redaction over a phrase both come down to knowing where things are. Both
// used to guess: field detection scanned a rendered image for dark pixels, and a search hit was placed by measuring
// its characters in a stand-in font. The content stream says exactly where each rule and each glyph is, and the walk
// reflow already makes reads it.
//
// **One coordinate space, converted in one place.** Everything here is a fraction of the page AS DISPLAYED — the crop
// box, turned by `/Rotate`, origin at the top left — which is the space the page's overlays are stored in. A caller
// never sees user space, so it cannot forget a rotation or a crop box whose corner is not the origin.

// MapRect is a box as fractions of the displayed page: [left, top, right, bottom], origin top-left.
type MapRect [4]float64

// MapText is one run of text as the page sets it: a single show operator's worth.
type MapText struct {
	Rect MapRect `json:"rect"`
	Text string  `json:"text"`
	// Cuts are the run's glyph boundaries along its baseline, as fractions of the page in the direction the run
	// advances: len(Chars)+1 of them, Cuts[i]..Cuts[i+1] being Chars[i]. From the font's own widths, kerning and
	// spacing included. Empty for a run that does not read left to right on the displayed page.
	Cuts  []float64 `json:"cuts,omitempty"`
	Chars []string  `json:"chars,omitempty"`
	// Size is the text's height as a fraction of the page's height.
	Size float64 `json:"size"`
	// Hidden is text the page sets without painting it (render mode 3) — an OCR layer.
	Hidden bool `json:"hidden,omitempty"`
	// Ink is a fitted OCR word's own ink up the page, [top, bottom] as fractions of its height: the scanned word's box
	// (ADR-093). Rect stays the line's reach — a full size above the baseline — which is what a reader groups runs
	// into lines by and what an estimated box is matched against; Ink is what a redaction of the word covers. Set
	// only where Cuts are.
	Ink []float64 `json:"ink,omitempty"`
	// Short is hidden text drawn inside a form at one scale both ways: an OCR word as Nib stamped it before ADR-092,
	// set at the height of its word's ink and so narrower than the word. Its box ends before the word does, and the
	// reader carries it out to the next one (ADR-091). A fitted word is scaled differently across and up.
	Short bool `json:"short,omitempty"`
}

// MapShape is a ruled line or a box.
type MapShape struct {
	Rect MapRect `json:"rect"`
	// Kind is "h" (a horizontal rule), "v" (a vertical rule) or "box" (a rectangle that is neither).
	Kind string `json:"kind"`
	// Filled is a box whose inside is painted with something other than white.
	Filled bool `json:"filled,omitempty"`
}

// MapWidget is a form field the document already has on this page.
type MapWidget struct {
	Rect MapRect `json:"rect"`
	// Kind is "text", "check", "radio", "choice", "signature" or "button".
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// PageMap is one page, mapped.
type PageMap struct {
	Page int `json:"page"`
	// Width and Height are the displayed page in points.
	Width   float64     `json:"width"`
	Height  float64     `json:"height"`
	Text    []MapText   `json:"text"`
	Shapes  []MapShape  `json:"shapes"`
	Widgets []MapWidget `json:"widgets"`
	// NoText is a page that sets no text at all: a scan, or text drawn as outlines.
	NoText bool `json:"noText"`
}

// ruleMax is the thickest a painted piece may be, in points, and still be a ruled line and not a box.
const ruleMax = 2.5

// MapPage maps page of pdf.
func MapPage(pdf []byte, page int) (PageMap, error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return PageMap{}, err
	}
	if err := pageInDocument(page, ctx.PageCount); err != nil {
		return PageMap{}, err
	}
	pg := pageAt(ctx, nil, page)
	pr, err := readPageShapes(ctx, pg)
	if err != nil {
		return PageMap{}, err
	}
	sp := newDisplaySpace(pg)
	m := PageMap{Page: page, Width: sp.w, Height: sp.h, NoText: pr.noText, Text: []MapText{}, Shapes: []MapShape{}, Widgets: []MapWidget{}}
	for _, r := range pr.runs {
		if t, ok := mapText(sp, r); ok {
			m.Text = append(m.Text, t)
		}
	}
	for _, s := range pr.shapes {
		if s.white {
			continue // painted white: on a white page it is not there, and under a field it is the field's background
		}
		if ms, ok := mapShape(sp, s); ok {
			m.Shapes = append(m.Shapes, ms)
		}
	}
	m.Shapes = dedupeShapes(m.Shapes)
	m.Widgets = mapWidgets(ctx, pg, sp)
	return m, nil
}

// displaySpace turns user space into fractions of the displayed page.
type displaySpace struct {
	box  [4]float64 // the visible box in user space
	rot  int        // /Rotate, a quarter turn in [0, 360)
	w, h float64    // the displayed page, in points
}

func newDisplaySpace(pg pdfread.Page) displaySpace {
	box, rot := visibleBoxOf(pg), pageRotation(pg)
	mb := mediaBoxOf(pg)
	// A crop box reaching past the media box shows only what the media box holds.
	if mb != ([4]float64{}) {
		box = [4]float64{math.Max(box[0], mb[0]), math.Max(box[1], mb[1]), math.Min(box[2], mb[2]), math.Min(box[3], mb[3])}
	}
	w, h := box[2]-box[0], box[3]-box[1]
	if rot == 90 || rot == 270 {
		w, h = h, w
	}
	return displaySpace{box: box, rot: rot, w: w, h: h}
}

// point is user-space (x, y) as a fraction of the displayed page, origin top-left.
func (sp displaySpace) point(x, y float64) (fx, fy float64) {
	bw, bh := sp.box[2]-sp.box[0], sp.box[3]-sp.box[1]
	if bw <= 0 || bh <= 0 {
		return 0, 0
	}
	u, v := (x-sp.box[0])/bw, (y-sp.box[1])/bh // from the bottom-left
	switch sp.rot {
	case 90: // turned clockwise: the page's bottom edge is the display's left edge
		return v, u
	case 180:
		return 1 - u, v
	case 270:
		return 1 - v, 1 - u
	}
	return u, 1 - v
}

// rect is a user-space box as a display rect.
func (sp displaySpace) rect(b [4]float64) MapRect {
	x0, y0 := sp.point(b[0], b[1])
	x1, y1 := sp.point(b[2], b[3])
	return MapRect{math.Min(x0, x1), math.Min(y0, y1), math.Max(x0, x1), math.Max(y0, y1)}
}

func (r MapRect) onPage() bool {
	return r[2] > 0 && r[0] < 1 && r[3] > 0 && r[1] < 1 && r[2] > r[0] && r[3] > r[1]
}

func mapText(sp displaySpace, r textRun) (MapText, bool) {
	if strings.TrimSpace(r.text) == "" || r.size <= 0 {
		return MapText{}, false
	}
	rect := sp.rect(runBox(r))
	if !rect.onPage() {
		return MapText{}, false
	}
	t := MapText{Rect: rect, Text: r.text, Size: r.size / (sp.box[3] - sp.box[1]), Hidden: r.state.tr == 3}
	if sp.rot == 90 || sp.rot == 270 {
		t.Size = r.size / (sp.box[2] - sp.box[0])
	}
	if c := r.state.ctm; t.Hidden && r.inForm {
		across, up := math.Hypot(c[0], c[1]), math.Hypot(c[2], c[3])
		t.Short = math.Abs(across-up) <= 1e-4*up
	}
	// Glyph boundaries only where the run advances left to right ON THE DISPLAY: an upright run on an unturned page.
	// Anything else keeps its box and is placed whole.
	if !r.rotated && sp.rot == 0 && r.width > 0 && len(r.glyphs) > 0 {
		pos := r.x
		cut := func(x float64) float64 { fx, _ := sp.point(x, r.y); return fx }
		t.Cuts = make([]float64, 0, len(r.glyphs)+1)
		t.Chars = make([]string, 0, len(r.glyphs))
		for _, g := range r.glyphs {
			pos += g.kern
			if len(t.Cuts) == 0 {
				t.Cuts = append(t.Cuts, cut(pos))
			} else {
				t.Cuts[len(t.Cuts)-1] = cut(pos) // one boundary between two glyphs: a kern's gap goes to the glyph before it
			}
			pos += g.advance
			t.Cuts = append(t.Cuts, cut(pos))
			t.Chars = append(t.Chars, g.text)
		}
		if bottom, top, ok := fittedInk(r); ok && t.Hidden && r.inForm && !t.Short {
			_, y0 := sp.point(r.x, top)
			_, y1 := sp.point(r.x, bottom)
			t.Ink = []float64{y0, y1}
		}
	}
	return t, true
}

// fittedInk is how far a fitted OCR word's ink reaches up the page, in user space: the ink of its own glyphs, which
// is the scanned word's box, because that is what the fit put them on (ADR-092, `fitWord`).
//
// `runBox` gives every run a full size above its baseline and a quarter below, which is a line's reach and right for
// reflow. A fitted word is set at its true em, a median 1.32 of its ink's height, so that box stood 1.65 times as tall
// as the word and a redaction drawn to it reached the line above (measured on two real scans: 13 of 40 and 5 of 39
// boxes, /pending 851 part 1) — and it still stopped a quarter em down, short of an Arabic descender (part 2). The
// face is nib's own and the text is the run's, so the ink is measured, not bounded.
//
// Only for a face nib stamps an OCR layer in, on an upright run. The name is checked against the table before the
// face is asked for, because `ocrFace` remembers every name it is asked — and a document chooses its own font names.
func fittedInk(r textRun) (bottom, top float64, ok bool) {
	name := r.baseFont
	if i := strings.IndexByte(name, '+'); i >= 0 {
		name = name[i+1:] // a subset's tag
	}
	if _, known := ocrFontFiles[name]; (!known && name != ocrFont) || r.rotated {
		return 0, 0, false
	}
	_, b, t, ok := wordInk(name, ocrFace(name), r.text)
	if !ok {
		return 0, 0, false
	}
	return r.y + b*r.size, r.y + t*r.size, true
}

func mapShape(sp displaySpace, s pageShape) (MapShape, bool) {
	b := s.box
	// A stroked line is as thick as its width: grow a segment's box to what is painted.
	if !s.rect && s.stroked {
		half := math.Max(s.lw, 0.25) / 2
		b = [4]float64{b[0] - half, b[1] - half, b[2] + half, b[3] + half}
	}
	rect := sp.rect(b)
	if !rect.onPage() {
		return MapShape{}, false
	}
	wPt, hPt := (rect[2]-rect[0])*sp.w, (rect[3]-rect[1])*sp.h
	out := MapShape{Rect: rect}
	switch {
	case hPt <= ruleMax && wPt > hPt*3:
		out.Kind = "h"
	case wPt <= ruleMax && hPt > wPt*3:
		out.Kind = "v"
	case s.rect && wPt > ruleMax && hPt > ruleMax:
		out.Kind, out.Filled = "box", s.filled
	default:
		return MapShape{}, false // a dot
	}
	return out, true
}

// dedupeShapes drops a shape drawn again in the same place — a rule stroked and then filled, a cell border shared by
// two cells — and sorts top to bottom, left to right.
func dedupeShapes(in []MapShape) []MapShape {
	const eps = 0.0006 // about a third of a point on a letter page
	sort.SliceStable(in, func(i, j int) bool {
		if math.Abs(in[i].Rect[1]-in[j].Rect[1]) > eps {
			return in[i].Rect[1] < in[j].Rect[1]
		}
		return in[i].Rect[0] < in[j].Rect[0]
	})
	out := in[:0]
	for _, s := range in {
		dup := false
		for k := len(out) - 1; k >= 0 && s.Rect[1]-out[k].Rect[1] <= eps; k-- {
			o := out[k]
			if o.Kind == s.Kind && math.Abs(o.Rect[0]-s.Rect[0]) <= eps && math.Abs(o.Rect[2]-s.Rect[2]) <= eps && math.Abs(o.Rect[3]-s.Rect[3]) <= eps {
				dup = true
				out[k].Filled = out[k].Filled || s.Filled
				break
			}
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

func mapWidgets(ctx *model.Context, pg pdfread.Page, sp displaySpace) []MapWidget {
	out := []MapWidget{}
	annots, err := ctx.DereferenceArray(pg.Dict["Annots"])
	if err != nil {
		return out
	}
	for _, a := range annots {
		d := derefDict(ctx.XRefTable, a)
		if d == nil || nameVal(d, "Subtype") != "Widget" {
			continue
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok {
			continue
		}
		rect := sp.rect([4]float64{llx, lly, urx, ury})
		if !rect.onPage() {
			continue
		}
		w := MapWidget{Rect: rect, Kind: widgetKind(ctx.XRefTable, d)}
		if t, ok := d["T"].(types.StringLiteral); ok {
			w.Name = t.Value()
		}
		out = append(out, w)
	}
	return out
}

// widgetKind names a widget by its field type and, for a button, the flags that tell a checkbox from a radio button.
func widgetKind(xt *model.XRefTable, d types.Dict) string {
	switch inheritedFieldType(xt, d) {
	case "Tx":
		return "text"
	case "Ch":
		return "choice"
	case "Sig":
		return "signature"
	case "Btn":
		ff := fieldFlags(xt, d)
		switch {
		case ff&(1<<16) != 0: // pushbutton
			return "button"
		case ff&(1<<15) != 0: // radio
			return "radio"
		}
		return "check"
	}
	return "text"
}

// fieldFlags is a field's /Ff, inherited from its ancestors as /FT is.
func fieldFlags(xt *model.XRefTable, d types.Dict) int {
	seen := map[int]bool{}
	for i := 0; d != nil && i <= maxFieldAncestry; i++ {
		if ff, ok := d["Ff"].(types.Integer); ok {
			return ff.Value()
		}
		ref, ok := d["Parent"].(types.IndirectRef)
		if !ok || seen[ref.ObjectNumber.Value()] {
			return 0
		}
		seen[ref.ObjectNumber.Value()] = true
		d = derefDict(xt, ref)
	}
	return 0
}
