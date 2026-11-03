package pdfops

import (
	"math"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// PageLinks is the third missing input of the document model (PLAN-import-export D18, W1, W24, Y7): where a page's
// links are and what they point at, as boxes in the page map's own fractions of the DISPLAYED page (ADR-088), so a
// caller never converts. A span takes the link whose box holds its run's CENTRE (`At`, ADR-090's rule — never
// overlap, because a neighbouring run's box reaches into a link's).
//
// What is carried: an external URI whose scheme is http, https or mailto. Everything else is DROPPED AND COUNTED in
// `Dropped`, by cause, and its text stays plain text: an internal destination (`/Dest`, `/GoTo`), `/GoToR`,
// `/Launch`, `/JavaScript`, a URI of any other scheme (`javascript:`, `file:`, `data:`), a URI with a control
// character or over `maxLinkURI`. A link the viewer would not show (Hidden, NoView) is not a link; the Print flag is not read — most producers leave it off on a link.
// Declared: a link hidden by an optional-content group (`/OC`) is read as a link — layers are resolved at the
// model's own read, P02.S03c.

// PageLink is one carried link.
type PageLink struct {
	Rect MapRect `json:"rect"`
	URI  string  `json:"uri"`
}

// LinkDropCause names why a link annotation was not carried.
const (
	DroppedInternal  = "internal"  // a destination inside the document
	DroppedRemote    = "remote"    // /GoToR, /Launch, /JavaScript, /Named and every other action
	DroppedScheme    = "scheme"    // a URI whose scheme is not http, https or mailto
	DroppedMalformed = "malformed" // no usable URI or rectangle, a control character, or past maxLinkURI
	DroppedCapped    = "capped"    // past maxLinksPerPage
)

const (
	maxLinkURI      = 2048
	maxLinksPerPage = 10000
)

// PageLinkSet is a page's carried links and the count of those dropped, by cause.
type PageLinkSet struct {
	Links   []PageLink     `json:"links"`
	Dropped map[string]int `json:"dropped,omitempty"`
}

// PageLinks reads the links of page of pdf.
func PageLinks(pdf []byte, page int) (PageLinkSet, error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return PageLinkSet{}, err
	}
	if err := pageInDocument(page, ctx.PageCount); err != nil {
		return PageLinkSet{}, err
	}
	return pageLinksIn(ctx, pageAt(ctx, nil, page)), nil
}

// At is the link whose box holds (x, y), a point in the page map's fractions.
func (s PageLinkSet) At(x, y float64) (PageLink, bool) {
	for _, l := range s.Links {
		if x >= l.Rect[0] && x <= l.Rect[2] && y >= l.Rect[1] && y <= l.Rect[3] {
			return l, true
		}
	}
	return PageLink{}, false
}

// pageLinksIn reads pg of the validated ctx, as `mapPageIn` does.
func pageLinksIn(ctx *model.Context, pg pdfread.Page) PageLinkSet {
	out := PageLinkSet{Links: []PageLink{}}
	drop := func(cause string) {
		if out.Dropped == nil {
			out.Dropped = map[string]int{}
		}
		out.Dropped[cause]++
	}
	annots, err := ctx.DereferenceArray(pg.Dict["Annots"])
	if err != nil {
		return out
	}
	sp := newDisplaySpace(pg)
	for _, a := range annots {
		d := derefDict(ctx.XRefTable, a)
		if d == nil || nameVal(d, "Subtype") != "Link" {
			continue
		}
		if f, ok := d["F"].(types.Integer); ok {
			// The Print flag is NOT read: a link with /F absent or 0 (hyperref writes none) is shown and is a link; the
			// plan's "print-off" skip would have dropped most real ones (remedy run, P02.S03a).
			const hidden, noView = 1 << 1, 1 << 5
			if v := f.Value(); v&hidden != 0 || v&noView != 0 {
				continue
			}
		}
		if len(out.Links) >= maxLinksPerPage {
			drop(DroppedCapped)
			continue
		}
		uri, cause := linkTarget(ctx.XRefTable, d)
		if cause != "" {
			drop(cause)
			continue
		}
		boxes := linkBoxes(ctx, d, sp)
		if len(boxes) == 0 {
			drop(DroppedMalformed)
			continue
		}
		for _, b := range boxes {
			out.Links = append(out.Links, PageLink{Rect: b, URI: uri})
		}
	}
	return out
}

// linkTarget is a link's URI, or the cause it is not carried. A link with both a /Dest and an /A is read by its /A,
// as a viewer does.
func linkTarget(xt *model.XRefTable, d types.Dict) (string, string) {
	act := derefDict(xt, d["A"])
	if act == nil {
		if d["Dest"] != nil {
			return "", DroppedInternal
		}
		return "", DroppedMalformed
	}
	switch nameVal(act, "S") {
	case "URI":
		s, ok := stringVal(xt, act["URI"])
		if !ok || s == "" || len(s) > maxLinkURI {
			return "", DroppedMalformed
		}
		for _, r := range s {
			if unicode.IsControl(r) || unicode.IsSpace(r) {
				return "", DroppedMalformed
			}
		}
		l := strings.ToLower(s)
		if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:") {
			return s, ""
		}
		return "", DroppedScheme
	case "GoTo":
		return "", DroppedInternal
	}
	return "", DroppedRemote
}

// linkBoxes is where the link is: each /QuadPoints quadrilateral's box if there are any (a link wrapped over two lines
// is two quads, and its /Rect is the union that also holds the gap), else /Rect.
func linkBoxes(ctx *model.Context, d types.Dict, sp displaySpace) []MapRect {
	var out []MapRect
	if qp, err := ctx.DereferenceArray(d["QuadPoints"]); err == nil && len(qp) >= 8 {
		for i := 0; i+8 <= len(qp) && i < 8*64; i += 8 {
			x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			good := true
			for j := 0; j < 8; j += 2 {
				x, ok1 := pdfNumber(ctx.XRefTable, qp[i+j])
				y, ok2 := pdfNumber(ctx.XRefTable, qp[i+j+1])
				if !ok1 || !ok2 {
					good = false
					break
				}
				x0, x1 = math.Min(x0, x), math.Max(x1, x)
				y0, y1 = math.Min(y0, y), math.Max(y1, y)
			}
			if good {
				if r := placeOnPage(sp, [4]float64{x0, y0, x1, y1}); r != ([4]float64{}) && MapRect(r).onPage() {
					out = append(out, MapRect(r))
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"]); ok {
		if r := placeOnPage(sp, [4]float64{llx, lly, urx, ury}); r != ([4]float64{}) && MapRect(r).onPage() {
			out = append(out, MapRect(r))
		}
	}
	return out
}
