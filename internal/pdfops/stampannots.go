package pdfops

import (
	"fmt"
	"math"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// turnAnnotations carries a turned page's annotations into the user space a pdfcpu stamp leaves it in —
// /pending 829, the half of the rotation fold `stampInPlace` declared and did not build.
//
// # What pdfcpu leaves behind
//
// To stamp a page with a `/Rotate`, pdfcpu deletes the `/Rotate`, swaps the box's sides and turns the page's
// CONTENT with one matrix (see stampInPlace). It moves no annotation (`stamp.go` has no `/Annots` handling but
// the link a watermark may add), so every widget, link, note and markup on the page kept the `/Rect` written
// for the old user space and lost the turn the viewer had given it. Measured on `testpdf.Form()` turned 90°
// and watermarked: its widgets stayed at y=700 on a page now 595 high — off the page — and every save bakes
// through a stamp, so it took one typed field to do it.
//
// # The repair
//
// The matrix that carried the content — pdfcpu's, about the box's corner as turnAboutTheCorner makes it — is
// applied to everything an annotation says in user space:
//
//   - `/Rect`, as the box of its four turned corners;
//   - `/QuadPoints`, `/L`, `/Vertices`, `/CL` and each path of `/InkList`, point by point;
//   - `/RD`, re-measured between the turned rectangle and the turned inner one;
//   - each appearance stream's `/Matrix`, so what was drawn turned with the page is still drawn turned. The
//     viewer fits the appearance's turned box to `/Rect` (ISO 32000-1 12.5.5), so a `/Rect` moved alone would
//     show a wide field's appearance squeezed upright into a tall rectangle. The stream is COPIED, never
//     edited: an appearance may be shared with an annotation on a page that was not turned;
//   - a widget's `/MK /R`, which is measured against the page: the turn the page lost is taken off it, so a
//     viewer that redraws the field draws it the way it was showing.
//
// An annotation flagged NoRotate was never turned with its page: the viewer holds its upper-left corner in
// place and draws it upright (12.5.3). Its rectangle keeps its size and follows that corner, and its
// appearance and `/MK` are left alone.
//
// Only the annotations the page held BEFORE the stamp are moved — pdfcpu writes a watermark's own link in the
// new user space, after them.
//
// # What it does not repair
//
// A destination naming a place on the page (`/XYZ left top`, `/FitR`, in a link, an outline item or
// `/OpenAction`) and a structure element's `/BBox` are still in the old user space. Declared, not built here.
func turnAnnotations(ctx *model.Context, turned []turnedPage) error {
	if len(turned) == 0 {
		return nil
	}
	pages := pdfread.Pages(ctx)
	moved := map[int]bool{}             // an annotation object two pages list is moved once
	copies := map[apCopy]types.Object{} // an appearance stream several annotations share is copied once per turn
	for _, t := range turned {
		if t.annots == 0 || t.nr > len(pages) || pages[t.nr-1].Err != nil {
			continue
		}
		p := pages[t.nr-1]
		if p.Attrs == nil || p.Attrs.Rotate%360 != 0 || p.Attrs.MediaBox == nil {
			continue // this page was not stamped, so pdfcpu turned nothing
		}
		m, ok := pageTurn(t, p.Attrs.MediaBox)
		if !ok {
			continue
		}
		annots, err := ctx.DereferenceArray(p.Dict["Annots"])
		if err != nil {
			continue
		}
		for i := 0; i < len(annots) && i < t.annots; i++ {
			if ir, ok := annots[i].(types.IndirectRef); ok {
				if moved[ir.ObjectNumber.Value()] {
					continue
				}
				moved[ir.ObjectNumber.Value()] = true
			}
			d := derefDict(ctx.XRefTable, annots[i])
			if d == nil {
				continue
			}
			if err := turnAnnotation(ctx, d, m, t.rot, copies); err != nil {
				return err
			}
		}
	}
	return nil
}

// turnMatrix is `a b c d e f`, as a `cm` writes it: (x, y) goes to (ax + cy + e, bx + dy + f).
type turnMatrix [6]float64

func (m turnMatrix) point(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// box is the rectangle around a turned rectangle's corners — itself a rectangle, since the turn is by quarters.
func (m turnMatrix) box(llx, lly, urx, ury float64) (float64, float64, float64, float64) {
	x0, y0 := m.point(llx, lly)
	x1, y1 := m.point(urx, ury)
	return math.Min(x0, x1), math.Min(y0, y1), math.Max(x0, x1), math.Max(y0, y1)
}

// pageTurn is the matrix a stamp turned page t's content with: pdfcpu's own rotation for the box it wrote
// (read back from the bytes it writes, so the two cannot drift), about the box's corner — `T(-ll) · R · T(ll)`,
// which is pdfcpu's R alone where the corner is the origin.
func pageTurn(t turnedPage, box *types.Rectangle) (turnMatrix, bool) {
	var r turnMatrix
	n, err := fmt.Sscanf(string(model.ContentBytesForPageRotation(t.rot, box.Width(), box.Height())),
		"%f %f %f %f %f %f cm", &r[0], &r[1], &r[2], &r[3], &r[4], &r[5])
	if err != nil || n != 6 {
		return turnMatrix{}, false
	}
	r[4] += t.llx - t.llx*r[0] - t.lly*r[2]
	r[5] += t.lly - t.llx*r[1] - t.lly*r[3]
	return r, true
}

// annotNoRotate is the annotation flag that keeps an annotation upright on a turned page (bit 5).
const annotNoRotate = 1 << 4

// apCopy names one appearance stream under one turn.
type apCopy struct {
	obj int
	m   turnMatrix
}

func turnAnnotation(ctx *model.Context, d types.Dict, m turnMatrix, rot int, copies map[apCopy]types.Object) error {
	xt := ctx.XRefTable
	flags, _ := intVal(xt, d["F"])
	upright := flags&annotNoRotate != 0

	llx, lly, urx, ury, hasRect := rectOf(ctx, d["Rect"])
	var nllx, nlly, nurx, nury float64
	if hasRect {
		if upright {
			// Held by its upper-left corner, at its own size.
			nllx, nury = m.point(llx, ury)
			nurx, nlly = nllx+(urx-llx), nury-(ury-lly)
		} else {
			nllx, nlly, nurx, nury = m.box(llx, lly, urx, ury)
		}
		d["Rect"] = numberArray(nllx, nlly, nurx, nury)
	}
	for _, key := range []string{"QuadPoints", "L", "Vertices", "CL"} {
		if pts, ok := turnedPoints(ctx, d[key], m); ok {
			d[key] = pts
		}
	}
	if paths, err := ctx.DereferenceArray(d["InkList"]); err == nil && paths != nil {
		out := make(types.Array, len(paths))
		for i, p := range paths {
			out[i] = p
			if pts, ok := turnedPoints(ctx, p, m); ok {
				out[i] = pts
			}
		}
		d["InkList"] = out
	}
	if upright {
		return nil
	}
	if rd, err := ctx.DereferenceArray(d["RD"]); err == nil && len(rd) == 4 && hasRect {
		var v [4]float64
		ok := true
		for i, e := range rd {
			f, good := pdfNumber(xt, e)
			v[i], ok = f, ok && good
		}
		if ok {
			ix0, iy0, ix1, iy1 := m.box(llx+v[0], lly+v[1], urx-v[2], ury-v[3])
			d["RD"] = numberArray(ix0-nllx, iy0-nlly, nurx-ix1, nury-iy1)
		}
	}
	if ap := derefDict(xt, d["AP"]); ap != nil {
		turnedAP := types.Dict{}
		for key, v := range ap {
			nv, err := turnedAppearance(ctx, v, m, copies)
			if err != nil {
				return err
			}
			turnedAP[key] = nv
		}
		d["AP"] = turnedAP
	}
	if nameVal(d, "Subtype") == "Widget" {
		mk := types.Dict{}
		for k, v := range derefDict(xt, d["MK"]) {
			mk[k] = v
		}
		r, _ := intVal(xt, mk["R"])
		if r = ((r-rot)%360 + 360) % 360; r != 0 {
			mk["R"] = types.Integer(r)
		} else {
			delete(mk, "R")
		}
		if len(mk) > 0 {
			d["MK"] = mk
		} else {
			delete(d, "MK")
		}
	}
	return nil
}

// turnedAppearance is one `/AP` entry under the turn: a stream, or a dictionary of streams by state.
func turnedAppearance(ctx *model.Context, v types.Object, m turnMatrix, copies map[apCopy]types.Object) (types.Object, error) {
	o, err := ctx.Dereference(v)
	if err != nil {
		return v, nil
	}
	if states, ok := o.(types.Dict); ok {
		out := types.Dict{}
		for name, s := range states {
			ns, err := turnedAppearance(ctx, s, m, copies)
			if err != nil {
				return nil, err
			}
			out[name] = ns
		}
		return out, nil
	}
	ir, isRef := v.(types.IndirectRef)
	sd, isStream := o.(types.StreamDict)
	if !isRef || !isStream {
		return v, nil
	}
	key := apCopy{ir.ObjectNumber.Value(), m}
	if c, ok := copies[key]; ok {
		return c, nil
	}
	// The appearance's own matrix first, then the page's turn: [a b c d e f] · m.
	a := turnMatrix{1, 0, 0, 1, 0, 0}
	if arr, err := ctx.DereferenceArray(sd.Dict["Matrix"]); err == nil && len(arr) == 6 {
		for i, e := range arr {
			if f, ok := pdfNumber(ctx.XRefTable, e); ok {
				a[i] = f
			}
		}
	}
	e, f := m.point(a[4], a[5])
	dict := types.Dict{}
	for k, val := range sd.Dict {
		dict[k] = val
	}
	dict["Matrix"] = numberArray(
		a[0]*m[0]+a[1]*m[2], a[0]*m[1]+a[1]*m[3],
		a[2]*m[0]+a[3]*m[2], a[2]*m[1]+a[3]*m[3],
		e, f)
	sd.Dict = dict
	ref, err := ctx.IndRefForNewObject(sd)
	if err != nil {
		return nil, err
	}
	copies[key] = *ref
	return *ref, nil
}

// turnedPoints is a flat array of x y pairs with every point carried through m.
func turnedPoints(ctx *model.Context, o types.Object, m turnMatrix) (types.Array, bool) {
	arr, err := ctx.DereferenceArray(o)
	if err != nil || len(arr) == 0 || len(arr)%2 != 0 {
		return nil, false
	}
	out := make(types.Array, len(arr))
	for i := 0; i < len(arr); i += 2 {
		x, okx := pdfNumber(ctx.XRefTable, arr[i])
		y, oky := pdfNumber(ctx.XRefTable, arr[i+1])
		if !okx || !oky {
			return nil, false
		}
		nx, ny := m.point(x, y)
		out[i], out[i+1] = types.Float(roundTurn(nx)), types.Float(roundTurn(ny))
	}
	return out, true
}

func numberArray(vs ...float64) types.Array {
	out := make(types.Array, len(vs))
	for i, v := range vs {
		out[i] = types.Float(roundTurn(v))
	}
	return out
}

// roundTurn drops the noise pdfcpu's five-decimal matrix leaves in a product (0 written as -0.00000…1).
func roundTurn(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}
