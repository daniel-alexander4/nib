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

// shiftAnchors moves everything anchored inside zone on page pg down by dy. The rewrite has already refused anything that
// touches the zone's height without lying inside it (`reflowParagraphIn`), so what is inside is all there is to move.
func shiftAnchors(ctx *model.Context, pg pdfread.Page, zone [4]float64, dy float64) error {
	xt := ctx.XRefTable
	inside := func(b [4]float64) bool { return covers(zone, b) }
	for _, o := range derefArray(xt, pg.Dict["Annots"]) {
		d := derefDict(xt, o)
		if d == nil || nameVal(d, "Subtype") == "Popup" {
			continue // a popup moves with its note, below — never by where its window happens to be
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok || !inside([4]float64{llx, lly, urx, ury}) {
			continue
		}
		shiftAnnot(ctx, d, dy)
		if pop := derefDict(xt, d["Popup"]); pop != nil {
			shiftAnnot(ctx, pop, dy)
		}
	}
	if pg.Ref != nil {
		page := pg.Ref.ObjectNumber.Value()
		for _, arr := range destinationArrays(ctx, page) { // each shared array once
			if b, ok := destinationBox(xt, arr, page); ok && inside(b) {
				shiftDestination(xt, arr, dy)
			}
		}
	}
	return shiftFlags(ctx, pg, zone, dy)
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

// carryAnchors moves everything anchored inside zone on page from to page to, dy lower: each annotation — and the popup
// it opens, wherever that sits — leaves from's /Annots for to's, its /P re-pointed; each destination is re-pointed at to;
// each NibFlag takes to's number and its fraction of to's page (P07.S06).
func carryAnchors(ctx *model.Context, from, to pdfread.Page, zone [4]float64, dy float64) error {
	xt := ctx.XRefTable
	if from.Ref == nil || to.Ref == nil {
		return fmt.Errorf("pdfops: a page to carry anchors between has no reference")
	}
	carry := map[string]bool{}
	key := func(o types.Object) string { return fmt.Sprint(o) }
	annots := derefArray(xt, from.Dict["Annots"])
	for _, o := range annots {
		d := derefDict(xt, o)
		if d == nil || nameVal(d, "Subtype") == "Popup" {
			continue // carried with its note, below
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok || !covers(zone, [4]float64{llx, lly, urx, ury}) {
			continue
		}
		carry[key(o)] = true
		if pop, ok := d["Popup"]; ok {
			carry[key(pop)] = true
		}
	}
	if len(carry) > 0 {
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
			}
			dst = append(dst, o)
		}
		to.Dict["Annots"] = dst
	}
	page := from.Ref.ObjectNumber.Value()
	for _, arr := range destinationArrays(ctx, page) {
		if b, ok := destinationBox(xt, arr, page); ok && covers(zone, b) {
			shiftDestination(xt, arr, dy)
			arr[0] = *to.Ref
		}
	}
	return carryFlags(ctx, from, to, zone, dy)
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

// shiftFlags moves each NibFlag on page pg inside zone down by dy, re-encoding the set with every other field as it was.
func shiftFlags(ctx *model.Context, pg pdfread.Page, zone [4]float64, dy float64) error {
	return carryFlags(ctx, pg, pg, zone, dy)
}

// carryFlags moves each NibFlag on page from inside zone to page to, dy lower — the same page when they are one — with its
// fraction taken of to's page, and every other field as it was.
func carryFlags(ctx *model.Context, from, to pdfread.Page, zone [4]float64, dy float64) error {
	if ctx.XRefTable.Info == nil {
		return nil
	}
	info := derefDict(ctx.XRefTable, *ctx.XRefTable.Info)
	if info == nil {
		return nil
	}
	enc, ok := stringVal(ctx.XRefTable, info[flagsKey])
	if !ok {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil
	}
	var flags []map[string]any
	if json.Unmarshal(raw, &flags) != nil {
		return nil
	}
	box, tbox := visibleBoxOf(from), visibleBoxOf(to)
	changed := false
	for _, f := range flags {
		page, _ := f["page"].(float64)
		frac, _ := f["frac"].(map[string]any)
		fx, xok := frac["x"].(float64)
		fy, yok := frac["y"].(float64)
		if int(page) != from.Nr || !xok || !yok {
			continue
		}
		x, y := box[0]+fx*(box[2]-box[0]), box[3]-fy*(box[3]-box[1])
		if covers(zone, [4]float64{x, y, x, y}) {
			frac["y"] = (tbox[3] - (y - dy)) / (tbox[3] - tbox[1])
			if to.Nr != from.Nr {
				frac["x"] = (x - tbox[0]) / (tbox[2] - tbox[0])
				f["page"] = to.Nr
			}
			changed = true
		}
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
