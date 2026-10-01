package pdfops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// Moving what is anchored with the text — `PLAN-text-reflow.md` P07.S04.
//
// When a region of a page moves down by dy, everything anchored inside it moves by the same dy: an annotation's `/Rect`
// and every coordinate key its subtype carries (`/QuadPoints`, `/L`, `/Vertices`, `/InkList`, `/CL`) — an appearance
// stream is mapped onto `/Rect` by translation (ISO 32000-1 §12.5.5), so moving the rect moves what is drawn; a
// destination's `top` (and a `/FitR`'s bottom and top), in place, so every link, bookmark and name that shares it follows;
// and a NibFlag's fraction of the page. Something only PARTLY inside — straddling the zone's edge, or lying in the free room
// the text will move into — has no single right answer and refuses.

// annotCoordKeys are the keys holding user-space coordinates as flat x,y pairs, by the subtypes that carry them.
var annotCoordKeys = []string{"QuadPoints", "L", "Vertices", "CL"}

// anchorSet is what is anchored inside a zone of a page, resolved ONCE, from the page as it was before anything moved: the
// notes on its `/Annots` (each with its popup, wherever that sits), the destination arrays naming it, and the NibFlags on
// it by their index in the list. A flow moves exactly these (P07 phase-close review): it had re-selected each page's
// anchors by zone AFTER the in-page shift had run, so what the shift moved into the old leave zone — the anchors of the
// last paragraph that STAYED — was then carried to the next page with the ones that left.
type anchorSet struct {
	annots []types.Object
	dests  []types.Array
	flags  []int
}

// anchorsIn resolves what is anchored wholly inside zone on page pg — the one selection a flow plans with and then moves.
// A popup is never selected for its own window; it goes where its note goes.
func anchorsIn(ctx *model.Context, pg pdfread.Page, zone [4]float64) anchorSet {
	xt := ctx.XRefTable
	var s anchorSet
	for _, o := range derefArray(xt, pg.Dict["Annots"]) {
		d := derefDict(xt, o)
		if d == nil || nameVal(d, "Subtype") == "Popup" {
			continue
		}
		if llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"]); ok && covers(zone, [4]float64{llx, lly, urx, ury}) {
			s.annots = append(s.annots, o)
		}
	}
	if pg.Ref != nil {
		page := pg.Ref.ObjectNumber.Value()
		for _, arr := range destinationArrays(ctx, page) { // each shared array once
			if b, ok := destinationBox(xt, arr, page); ok && covers(zone, b) {
				s.dests = append(s.dests, arr)
			}
		}
	}
	_, flags := nibFlags(ctx)
	box := visibleBoxOf(pg)
	for i, f := range flags {
		if page, onPage, x, y, placed := flagOn(f, box); onPage && placed && page == pg.Nr && covers(zone, [4]float64{x, y, x, y}) {
			s.flags = append(s.flags, i)
		}
	}
	return s
}

// shiftAnchors moves the anchors of set, on page pg, down by dy. The rewrite has already refused anything that touches
// the height that moves without lying inside a zone that moves (`pushDown`), so the set is all there is to move.
func shiftAnchors(ctx *model.Context, pg pdfread.Page, set anchorSet, dy float64) error {
	xt := ctx.XRefTable
	for _, o := range set.annots {
		d := derefDict(xt, o)
		if d == nil {
			continue
		}
		shiftAnnot(ctx, d, dy)
		if pop := derefDict(xt, d["Popup"]); pop != nil {
			shiftAnnot(ctx, pop, dy) // a popup moves with its note — never by where its window happens to be
		}
	}
	for _, arr := range set.dests {
		shiftDestination(xt, arr, dy)
	}
	return moveFlags(ctx, set.flags, pg, pg, dy)
}

// shiftAnnot moves annotation d down by dy: its /Rect and every coordinate key its subtype carries.
func shiftAnnot(ctx *model.Context, d types.Dict, dy float64) {
	xt := ctx.XRefTable
	if llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"]); ok {
		d["Rect"] = types.Array{types.Float(llx), types.Float(lly - dy), types.Float(urx), types.Float(ury - dy)}
	}
	for _, k := range annotCoordKeys {
		if arr := derefArray(xt, d[k]); arr != nil {
			d[k] = shiftPairs(xt, arr, dy)
		}
	}
	if ink := derefArray(xt, d["InkList"]); ink != nil {
		out := make(types.Array, len(ink))
		for i, path := range ink {
			if p := derefArray(xt, path); p != nil {
				out[i] = shiftPairs(xt, p, dy)
			} else {
				out[i] = path
			}
		}
		d["InkList"] = out
	}
}

// carryAnchors moves the anchors of set from page from to page to, dy lower: each annotation — and the popup it opens,
// wherever that sits — leaves from's /Annots for to's, its /P re-pointed; each destination is re-pointed at to; each
// NibFlag takes to's number and its fraction of to's page (P07.S06).
func carryAnchors(ctx *model.Context, from, to pdfread.Page, set anchorSet, dy float64) error {
	xt := ctx.XRefTable
	if from.Ref == nil || to.Ref == nil {
		return fmt.Errorf("pdfops: a page to carry anchors between has no reference")
	}
	carry := map[string]bool{}
	key := func(o types.Object) string { return fmt.Sprint(o) }
	for _, o := range set.annots {
		carry[key(o)] = true
		if pop, ok := derefDict(xt, o)["Popup"]; ok {
			carry[key(pop)] = true
		}
	}
	if len(carry) > 0 {
		annots := derefArray(xt, from.Dict["Annots"])
		var kept, moved types.Array
		for _, o := range annots {
			if carry[key(o)] {
				moved = append(moved, o)
			} else {
				kept = append(kept, o)
			}
		}
		from.Dict["Annots"] = kept
		dst := append(types.Array(nil), derefArray(xt, to.Dict["Annots"])...)
		for _, o := range moved {
			if d := derefDict(xt, o); d != nil {
				shiftAnnot(ctx, d, dy)
				d["P"] = *to.Ref
				repointOBJR(ctx, d, o, *from.Ref, *to.Ref)
			}
			dst = append(dst, o)
		}
		to.Dict["Annots"] = dst
	}
	for _, arr := range set.dests {
		shiftDestination(xt, arr, dy)
		arr[0] = *to.Ref
	}
	return moveFlags(ctx, set.flags, from, to, dy)
}

// shiftPairs returns arr with every second number — each y — moved down by dy.
func shiftPairs(xt *model.XRefTable, arr types.Array, dy float64) types.Array {
	out := make(types.Array, len(arr))
	for i, o := range arr {
		v, ok := pdfNumber(xt, o)
		if !ok || i%2 == 0 {
			out[i] = o
			continue
		}
		out[i] = types.Float(v - dy)
	}
	return out
}

// shiftDestination moves a destination array's vertical position down by dy, in place — the array is the object every
// link, bookmark and name pointing at it shares.
func shiftDestination(xt *model.XRefTable, arr types.Array, dy float64) {
	name, _ := arr[1].(types.Name)
	move := func(i int) {
		if v, ok := pdfNumber(xt, arr[i]); ok {
			arr[i] = types.Float(v - dy)
		}
	}
	switch name {
	case "XYZ":
		move(3)
	case "FitH", "FitBH":
		move(2)
	case "FitR":
		move(3)
		move(5)
	}
}

// moveFlags moves the NibFlags at indices idx — each on page from — to page to, dy lower — the same page when they are one —
// with its fraction taken of to's page, and every other field, and every other flag, as it was.
func moveFlags(ctx *model.Context, idx []int, from, to pdfread.Page, dy float64) error {
	if len(idx) == 0 {
		return nil
	}
	info, flags := nibFlags(ctx)
	if info == nil {
		return nil
	}
	box, tbox := visibleBoxOf(from), visibleBoxOf(to)
	changed := false
	for _, i := range idx {
		if i < 0 || i >= len(flags) {
			continue
		}
		f, _ := flags[i].(map[string]any)
		page, onPage, x, y, placed := flagOn(f, box)
		if !onPage || !placed || page != from.Nr {
			continue
		}
		frac := f["frac"].(map[string]any) // flagOn read it as an object
		frac["y"] = (tbox[3] - (y - dy)) / (tbox[3] - tbox[1])
		if to.Nr != from.Nr {
			frac["x"] = (x - tbox[0]) / (tbox[2] - tbox[0])
			f["page"] = to.Nr
		}
		changed = true
	}
	if !changed {
		return nil
	}
	out, err := json.Marshal(flags)
	if err != nil {
		return fmt.Errorf("pdfops: re-encoding the moved flags: %w", err)
	}
	// The Info dictionary is what pdfcpu writes; `ctx.Properties` mirrors it for anything that reads this context again,
	// as `pdfcpu.PropertiesAdd` keeps the two.
	v := base64.StdEncoding.EncodeToString(out)
	info[flagsKey] = types.StringLiteral(v)
	if ctx.Properties != nil {
		ctx.Properties[flagsKey] = v
	}
	return nil
}

// repointOBJR re-points the structure tree's reference to annotation annot — object o — at page to: the element its
// `/StructParent` names, and the OBJR kid of it whose `/Obj` is o, gains `/Pg` to — and the element follows it when nothing
// of it is left on from (`followsItsContent`, P07.S07). An OBJR's `/Pg` says which
// page the object is on, and an annotation that changed page and kept its OBJR on the old one is placed in reading order
// on a page it is not on — which no check sees, because that page is still live. An annotation the tree does not
// reference has nothing to re-point.
func repointOBJR(ctx *model.Context, annot types.Dict, o types.Object, from, to types.IndirectRef) {
	xt := ctx.XRefTable
	key, ok := pdfNumber(xt, annot["StructParent"])
	cat, err := xt.Catalog()
	if !ok || err != nil {
		return
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return
	}
	_, elem, found := rowFor(ctx, &structTree{root: root}, int(key))
	if !found || elem == nil {
		return
	}
	d := derefDict(xt, *elem)
	if d == nil {
		return
	}
	kids, _ := kidsArray(ctx, d)
	for _, k := range kids {
		objr := derefDict(xt, k)
		if t := objr.NameEntry("Type"); objr != nil && t != nil && *t == "OBJR" && sameObject(objr["Obj"], o) {
			objr["Pg"] = to
		}
	}
	followsItsContent(ctx, *elem, kids, from, to)
}
