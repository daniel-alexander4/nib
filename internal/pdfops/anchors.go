package pdfops

import (
	"encoding/base64"
	"encoding/json"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// What is anchored to a position on a page — `PLAN-text-reflow.md` P07.S03.
//
// Text that moves leaves behind everything that says WHERE on the page something is, rather than what it is beside: an
// annotation's `/Rect` (a link over a word, a form widget, a sticky note), a NibFlag's fraction of the page, and a
// destination's `top` — a bookmark, a link from another page, the document's open action, a named destination a URL can
// reach. Each is read here as a box in user space. The phase's exit criterion is that none is silently orphaned: what lies
// wholly inside what moves moves with it (`anchormove.go`, P07.S04; across pages, P07.S06), and anything else the moving
// text touches — straddling, in the free room, an article bead — refuses `anchored` (P07 phase-close review: this said
// every anchor refused, which stopped being true at S04).

// anchorKind names what is anchored.
type anchorKind string

const (
	anchorAnnotation  anchorKind = "annotation"
	anchorWidget      anchorKind = "widget"
	anchorFlag        anchorKind = "flag"
	anchorDestination anchorKind = "destination"
	// anchorBead is an article thread's bead (`/B` on the page, its `/R` the rectangle it reads). Nothing moves a bead, so
	// one meeting moving text always refuses (P07 phase-close review).
	anchorBead anchorKind = "bead"
)

// pageAnchor is one thing anchored to a position on a page, boxed in user space. A destination's box is the line it
// scrolls to — [left, top, left, top] for `/XYZ`, the width of everything at `top` for `/FitH` and `/FitBH`, the rectangle
// for `/FitR`.
type pageAnchor struct {
	kind anchorKind
	box  [4]float64
}

var everywhere = [4]float64{math.Inf(-1), math.Inf(-1), math.Inf(1), math.Inf(1)}

// pageAnchors reads everything anchored to a position on page pg: its annotations always, and — when withMoving, for an
// edit that moves text off its own lines — its NibFlags and every destination to it, which cost a pass over every page's
// annotations and the outline.
func pageAnchors(ctx *model.Context, pg pdfread.Page, withMoving bool) []pageAnchor {
	xt := ctx.XRefTable
	var out []pageAnchor
	for _, o := range derefArray(xt, pg.Dict["Annots"]) {
		d := derefDict(xt, o)
		if d == nil || nameVal(d, "Subtype") == "Popup" {
			continue // a popup is its note's window, wherever it is shown: it goes where its note goes (carryAnchors)
		}
		kind := anchorAnnotation
		if nameVal(d, "Subtype") == "Widget" {
			kind = anchorWidget
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok {
			out = append(out, pageAnchor{kind, everywhere}) // a rect it cannot read could be anywhere
			continue
		}
		out = append(out, pageAnchor{kind, [4]float64{llx, lly, urx, ury}})
	}
	if !withMoving {
		return out
	}
	out = append(out, flagAnchors(ctx, pg)...)
	if pg.Ref != nil {
		out = append(out, destinationAnchors(ctx, pg.Ref.ObjectNumber.Value())...)
	}
	return append(out, beadAnchors(ctx, pg)...)
}

// maxBeads bounds the beads read off one page: `/B` is an array the file writes.
const maxBeads = 10000

// beadAnchors reads page pg's article beads — each `/B` entry's `/R` — as boxes; a bead whose rectangle cannot be read
// could be anywhere.
func beadAnchors(ctx *model.Context, pg pdfread.Page) []pageAnchor {
	var out []pageAnchor
	for i, o := range derefArray(ctx.XRefTable, pg.Dict["B"]) {
		if i >= maxBeads {
			return append(out, pageAnchor{anchorBead, everywhere})
		}
		llx, lly, urx, ury, ok := rectOf(ctx, derefDict(ctx.XRefTable, o)["R"])
		if !ok {
			out = append(out, pageAnchor{anchorBead, everywhere})
			continue
		}
		out = append(out, pageAnchor{anchorBead, [4]float64{llx, lly, urx, ury}})
	}
	return out
}

// flagAnchors reads the NibFlags on page pg as points. A flag's `frac` is a fraction of the page as the viewer shows it,
// from its top-left corner — the crop box, turned by `/Rotate`. Unturned it maps straight back to user space; on a turned
// page it is placed nowhere this reader will vouch for, so it could be anywhere — and so could a flag on this page whose
// fraction does not read.
func flagAnchors(ctx *model.Context, pg pdfread.Page) []pageAnchor {
	_, flags := nibFlags(ctx)
	box := visibleBoxOf(pg)
	rotated := pageRotation(pg) != 0
	var out []pageAnchor
	for _, f := range flags {
		page, onPage, x, y, placed := flagOn(f, box)
		if !onPage || page != pg.Nr {
			continue
		}
		if !placed || rotated || !(box[2] > box[0] && box[3] > box[1]) {
			out = append(out, pageAnchor{anchorFlag, everywhere})
			continue
		}
		out = append(out, pageAnchor{anchorFlag, [4]float64{x, y, x, y}})
	}
	return out
}

// nibFlags is the NibFlags list as the Info dictionary holds it, and that dictionary — the ONE decoder the refusal check
// (`flagAnchors`) and the mover (`moveFlags`) share (P07 phase-close review; ADR-009). Each element is read on its own:
// the check decoded the whole list into one typed shape and the mover into another, so one malformed flag read as NO
// flags to the check while the mover still moved the valid ones. A blob that does not decode as a list is no flags, as
// `FlagsJSON` has it.
func nibFlags(ctx *model.Context) (types.Dict, []any) {
	if ctx.XRefTable.Info == nil {
		return nil, nil
	}
	info := derefDict(ctx.XRefTable, *ctx.XRefTable.Info)
	if info == nil {
		return nil, nil
	}
	enc, ok := stringVal(ctx.XRefTable, info[flagsKey])
	if !ok {
		return info, nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return info, nil // unreadable reads as no flags, as FlagsJSON has it
	}
	var flags []any
	if json.Unmarshal(raw, &flags) != nil {
		return info, nil
	}
	return info, flags
}

// flagOn reads one flag: its page number (onPage false when it names none), and — placed — its point in user space on a
// page whose visible box is box, from its fraction of that page. A flag on a page whose fraction does not read is on that
// page and placed nowhere.
func flagOn(f any, box [4]float64) (page int, onPage bool, x, y float64, placed bool) {
	m, _ := f.(map[string]any)
	p, ok := m["page"].(float64)
	if !ok || p != math.Trunc(p) {
		return 0, false, 0, 0, false
	}
	frac, _ := m["frac"].(map[string]any)
	fx, xok := frac["x"].(float64)
	fy, yok := frac["y"].(float64)
	if !xok || !yok || !finite(fx, fy) {
		return int(p), true, 0, 0, false
	}
	return int(p), true, box[0] + fx*(box[2]-box[0]), box[3] - fy*(box[3]-box[1]), true
}

// stringVal reads a PDF string or name as Go text.
func stringVal(xt *model.XRefTable, o types.Object) (string, bool) {
	r, err := xt.Dereference(o)
	if err != nil || r == nil {
		return "", false
	}
	switch v := r.(type) {
	case types.StringLiteral:
		s, err := types.StringLiteralToString(v)
		return s, err == nil
	case types.HexLiteral:
		s, err := types.HexLiteralToString(v)
		return s, err == nil
	case types.Name:
		return string(v), true
	}
	return "", false
}

// maxOutlineItems bounds the outline walk: an outline is a linked structure the file writes, and a loop in it is a loop.
const maxOutlineItems = 100000

// destinationAnchors reads every destination that scrolls to a position on the page whose object number is page: the
// outline, the open action, every link annotation on every page, and the named destinations — which a URL's `#name` can
// reach from outside the document, so they count whether or not anything inside it points at them.
func destinationAnchors(ctx *model.Context, page int) []pageAnchor {
	var out []pageAnchor
	for _, arr := range destinationArrays(ctx, page) {
		if box, ok := destinationBox(ctx.XRefTable, arr, page); ok {
			out = append(out, pageAnchor{anchorDestination, box})
		}
	}
	return out
}

// destinationArrays is every destination array that names page, each ONCE: one array shared by a link, a bookmark and a
// name is one destination, and a mover that visited it three times would move it three times.
func destinationArrays(ctx *model.Context, page int) []types.Array {
	var out []types.Array
	seen := map[*types.Object]bool{}
	for _, d := range destinationObjects(ctx) {
		arr, ok := destinationArray(ctx.XRefTable, d)
		if !ok || len(arr) < 2 {
			continue
		}
		if ref, isRef := arr[0].(types.IndirectRef); !isRef || ref.ObjectNumber.Value() != page {
			continue
		}
		if seen[&arr[0]] {
			continue
		}
		seen[&arr[0]] = true
		out = append(out, arr)
	}
	return out
}

// maxActions bounds one walk of an action and its `/Next` chain: a chain is a linked structure the file writes, and a loop in
// it is a loop.
const maxActions = 10000

// destinationObjects collects every destination the document holds, unresolved: the open action, the outline, the old
// catalog /Dests, the /Dests name tree, and every annotation on every page — a link's `/Dest`, and a GoTo in any
// annotation's `/A` or `/AA` (a widget's button is as much a link as a link is) — and the GoTo actions in the catalog's and
// every page's `/AA`. An action's `/Next` is followed: a GoTo chained after a JavaScript or a sound is still a GoTo (P07
// phase-close review: only the first action of a link or bookmark was read, and a widget's GoTo stayed where the text left).
func destinationObjects(ctx *model.Context) []types.Object {
	xt := ctx.XRefTable
	var dests []types.Object
	// fromAction reads action a and every action its `/Next` chains, each once and bounded.
	fromAction := func(a types.Object) {
		seen := map[int]bool{}
		queue := []types.Object{a}
		for n := 0; len(queue) > 0 && n < maxActions; n++ {
			o := queue[0]
			queue = queue[1:]
			if ir, ok := o.(types.IndirectRef); ok {
				if seen[ir.ObjectNumber.Value()] {
					continue
				}
				seen[ir.ObjectNumber.Value()] = true
			}
			act := derefDict(xt, o)
			if act == nil {
				continue
			}
			if nameVal(act, "S") == "GoTo" {
				if d, ok := act["D"]; ok {
					dests = append(dests, d)
				}
			}
			next, ok := act["Next"]
			if !ok {
				continue
			}
			if arr := derefArray(xt, next); arr != nil {
				queue = append(queue, arr...)
			} else {
				queue = append(queue, next)
			}
		}
	}
	// fromTriggers reads an additional-actions dictionary: every trigger's action.
	fromTriggers := func(aa types.Object) {
		for _, a := range derefDict(xt, aa) {
			fromAction(a)
		}
	}
	if root, err := xt.Catalog(); err == nil && root != nil {
		if oa, ok := root["OpenAction"]; ok {
			if derefArray(xt, oa) != nil {
				dests = append(dests, oa)
			} else {
				fromAction(oa)
			}
		}
		fromTriggers(root["AA"])
		if ol := derefDict(xt, root["Outlines"]); ol != nil {
			// Every item once: an item reached twice by reference is a loop the file wrote, and the walk is bounded.
			seen := map[int]bool{}
			stack := []types.Object{ol["First"]}
			for n := 0; len(stack) > 0 && n < maxOutlineItems; n++ {
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if ir, ok := o.(types.IndirectRef); ok {
					if seen[ir.ObjectNumber.Value()] {
						continue
					}
					seen[ir.ObjectNumber.Value()] = true
				}
				item := derefDict(xt, o)
				if item == nil {
					continue
				}
				if d, ok := item["Dest"]; ok {
					dests = append(dests, d)
				} else {
					fromAction(item["A"])
				}
				for _, k := range []string{"Next", "First"} {
					if v, ok := item[k]; ok {
						stack = append(stack, v)
					}
				}
			}
		}
		if old := derefDict(xt, root["Dests"]); old != nil {
			for _, v := range old {
				dests = append(dests, v)
			}
		}
	}
	if node := xt.Names["Dests"]; node != nil {
		_ = node.Process(xt, func(_ *model.XRefTable, _ string, v *types.Object) error {
			dests = append(dests, *v)
			return nil
		})
	}
	for _, p := range pdfread.Pages(ctx) {
		if p.Err != nil || p.Dict == nil {
			continue
		}
		fromTriggers(p.Dict["AA"])
		for _, o := range derefArray(xt, p.Dict["Annots"]) {
			d := derefDict(xt, o)
			if d == nil {
				continue
			}
			if dest, ok := d["Dest"]; ok && nameVal(d, "Subtype") == "Link" {
				dests = append(dests, dest)
			} else {
				fromAction(d["A"])
			}
			fromTriggers(d["AA"])
		}
	}
	return dests
}

// destinationArray resolves a destination to its array: the array itself, a dictionary carrying one under /D, or a name
// or string keying the /Dests name tree.
func destinationArray(xt *model.XRefTable, o types.Object) (types.Array, bool) {
	r, err := xt.Dereference(o)
	if err != nil || r == nil {
		return nil, false
	}
	switch v := r.(type) {
	case types.Array:
		return v, true
	case types.Dict:
		arr := derefArray(xt, v["D"])
		return arr, arr != nil
	}
	key, err := xt.DestName(o)
	if err != nil || key == "" || xt.Names["Dests"] == nil {
		return nil, false
	}
	v, ok := xt.Names["Dests"].Value(key)
	if !ok {
		return nil, false
	}
	r, err = xt.Dereference(v)
	if err != nil {
		return nil, false
	}
	switch w := r.(type) {
	case types.Array:
		return w, true
	case types.Dict:
		arr := derefArray(xt, w["D"])
		return arr, arr != nil
	}
	return nil, false
}

// destinationBox resolves a destination — an array, a dictionary carrying one under /D, or a name keying the /Dests name
// tree — and, when it scrolls to a position on page, returns that position. A destination that shows the whole page
// (`/Fit`, `/FitB`) or only fixes its left edge (`/FitV`, `/FitBV`), or an `/XYZ` with a null top, names no line to lose.
func destinationBox(xt *model.XRefTable, o types.Object, page int) ([4]float64, bool) {
	arr, ok := destinationArray(xt, o)
	if !ok {
		return [4]float64{}, false
	}
	if len(arr) < 2 {
		return [4]float64{}, false
	}
	ref, ok := arr[0].(types.IndirectRef)
	if !ok || ref.ObjectNumber.Value() != page {
		return [4]float64{}, false
	}
	n := func(i int) (float64, bool) {
		if i >= len(arr) {
			return 0, false
		}
		return pdfNumber(xt, arr[i])
	}
	name, _ := arr[1].(types.Name)
	switch name {
	case "XYZ":
		top, ok := n(3)
		if !ok {
			return [4]float64{}, false
		}
		if left, lok := n(2); lok {
			return [4]float64{left, top, left, top}, true
		}
		return [4]float64{math.Inf(-1), top, math.Inf(1), top}, true
	case "FitH", "FitBH":
		top, ok := n(2)
		if !ok {
			return [4]float64{}, false
		}
		return [4]float64{math.Inf(-1), top, math.Inf(1), top}, true
	case "FitR":
		l, ok1 := n(2)
		b, ok2 := n(3)
		rr, ok3 := n(4)
		t, ok4 := n(5)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return everywhere, true
		}
		return [4]float64{math.Min(l, rr), math.Min(b, t), math.Max(l, rr), math.Max(b, t)}, true
	}
	return [4]float64{}, false
}

// annotatedOver says whether an annotation or widget lies over paragraph p: a re-wrap moves the paragraph's words inside
// its box, so a link on a word, or a field over a line, would point at other words after any edit. The one door both the
// paragraph list and the rewrite ask.
func annotatedOver(ctx *model.Context, pg pdfread.Page, p textParagraph) bool {
	own := paragraphBox(p)
	for _, a := range pageAnchors(ctx, pg, false) {
		if meets(a.box, own) {
			return true
		}
	}
	return false
}
