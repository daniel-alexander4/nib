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
		if d == nil {
			continue
		}
		llx, lly, urx, ury, ok := rectOf(ctx, d["Rect"])
		if !ok || !inside([4]float64{llx, lly, urx, ury}) {
			continue
		}
		d["Rect"] = types.Array{types.Float(llx), types.Float(lly - dy), types.Float(urx), types.Float(ury - dy)}
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
	box := visibleBoxOf(pg)
	h := box[3] - box[1]
	changed := false
	for _, f := range flags {
		page, _ := f["page"].(float64)
		frac, _ := f["frac"].(map[string]any)
		fx, xok := frac["x"].(float64)
		fy, yok := frac["y"].(float64)
		if int(page) != pg.Nr || !xok || !yok {
			continue
		}
		x, y := box[0]+fx*(box[2]-box[0]), box[3]-fy*h
		if covers(zone, [4]float64{x, y, x, y}) {
			frac["y"] = fy + dy/h
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
